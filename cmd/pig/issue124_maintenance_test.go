//go:build darwin || linux

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPigMaintenanceParity124(t *testing.T) {
	binary := buildPigBinary(t)
	if os.Getenv("PIG_TEST_RACE") == "1" {
		cmd := exec.Command("go", "build", "-race", "-o", binary, ".")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v", output, err)
		}
	}
	cmd := exec.Command("python3", "../../parity/terminal/maintenance.py", binary, "--pig")
	data, err := cmd.Output()
	if err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			t.Fatalf("%s: %v", e.Stderr, err)
		}
		t.Fatal(err)
	}
	lock, _, err := baseline.Load("../../parity/baseline")
	if err != nil {
		t.Fatal(err)
	}
	locked := parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository}
	fixture, err := parity.LoadFixture("../../parity/oracle/fixtures/maintenance-cli.json", locked)
	if err != nil {
		t.Fatal(err)
	}
	oracle, err := parity.NewFixtureDriver(fixture, locked)
	if err != nil {
		t.Fatal(err)
	}
	result, err := parity.RunCase(context.Background(), fixture.Case, oracle, parity.DriverFunc{SurfaceName: parity.SurfaceCLI, ObserveFunc: func(context.Context, parity.Case) (parity.Observation, error) {
		return parity.Observation{Outcome: json.RawMessage(data), SideEffects: &[]parity.SideEffect{}}, nil
	}})
	if err != nil || !result.Match {
		t.Fatalf("%v\nwant %s\ngot %s", err, fixture.Observation.Outcome, data)
	}
}

func TestPigMaintenanceBusyAndDiagnostics124(t *testing.T) {
	var calls atomic.Int32
	released := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		n := calls.Add(1)
		if n == 2 {
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"BUSY_RESPONSE\"}}]}\n\n")
			w.(http.Flusher).Flush()
			select {
			case <-released:
			case <-r.Context().Done():
				return
			}
		}
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"CONTINUED_%d\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", n)
	}))
	defer server.Close()
	tty, root, done := startQueues116(t, server.URL, `{"quietStartup":true,"compaction":{"enabled":false},"retry":{"enabled":false}}`)
	tty.Send(t, "first question\r")
	tty.Wait(t, "CONTINUED")
	tty.Send(t, "busy question\r")
	tty.Wait(t, "BUSY_RESPONSE")
	tty.Send(t, "/reload\r")
	tty.Wait(t, "Wait for the current response")
	tty.Send(t, "/session\r")
	tty.Wait(t, "Session Info")
	path := filepath.Join(root, "busy.html")
	tty.Send(t, "/export "+path+"\r")
	tty.Wait(t, "Session exported to:")
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	close(released)
	// Wait for a new prompt to reach the Provider before modifying resource files.
	tty.Send(t, "after busy\r")
	deadline := time.Now().Add(5 * time.Second)
	for calls.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if calls.Load() != 3 {
		t.Fatal("did not continue after busy command")
	}
	tty.Wait(t, "CONTINUED_3")
	tty.Send(t, "/session\r")
	tty.Wait(t, "Messages: 6")
	dir := filepath.Join(root, ".pig", "agent", "themes")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.json"), []byte("invalid theme"), 0600); err != nil {
		t.Fatal(err)
	}
	tty.Send(t, "/reload\r")
	tty.Wait(t, "broken.json")
	tty.Wait(t, "Reloaded keybindings")
	exitQueues116(t, tty, done)
}

func TestPigMaintenanceRetryStats124(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprint(w, `{"error":{"message":"overloaded"}}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"RETRY_RECOVERED\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":120,\"completion_tokens\":20,\"total_tokens\":140}}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	tty, _, done := startQueues116(t, server.URL, `{"quietStartup":true,"compaction":{"enabled":false},"retry":{"enabled":true,"maxRetries":2,"baseDelayMs":500,"provider":{"maxRetries":0}}}`)
	tty.Send(t, "retry question\r")
	tty.Wait(t, "Retrying (1/2)")
	tty.Wait(t, "RETRY_RECOVERED")
	tty.Send(t, "/session\r")
	tty.Wait(t, "Session Info")
	tty.Wait(t, "Input: 120")
	tty.Wait(t, "Output: 20")
	tty.Wait(t, "Total: 140")
	if calls.Load() != 2 {
		t.Fatal("unexpected attempts", calls.Load())
	}
	exitQueues116(t, tty, done)
}

func TestPigMaintenanceCompactRunning124(t *testing.T) {
	var calls atomic.Int32
	cancelled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		n := calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		if n == 3 {
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ACTIVE_TO_COMPACT\"}}]}\n\n")
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			close(cancelled)
			return
		}
		content := fmt.Sprintf("ANSWER_%d", n)
		if strings.Contains(string(body), "context summarization assistant") {
			content = "RUNNING_SUMMARY"
		}
		if n > 3 && !strings.Contains(string(body), "context summarization assistant") {
			content = "AFTER_RUNNING_COMPACTION"
			if !strings.Contains(string(body), "RUNNING_SUMMARY") {
				t.Error("next generation missed summary")
			}
		}
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", content)
	}))
	defer server.Close()
	tty, _, done := startQueues116(t, server.URL, `{"quietStartup":true,"compaction":{"enabled":false,"keepRecentTokens":10,"reserveTokens":1000},"retry":{"enabled":false}}`)
	tty.Send(t, "first completed question with enough context\r")
	tty.Wait(t, "ANSWER_1")
	tty.Send(t, "second completed question with enough context\r")
	tty.Wait(t, "ANSWER_2")
	tty.Send(t, "third question\r")
	tty.Wait(t, "ACTIVE_TO_COMPACT")
	tty.Send(t, "/compact\r")
	tty.Wait(t, "Compacted from")
	tty.Wait(t, "RUNNING_SUMMARY")
	select {
	case <-cancelled:
	default:
		t.Fatal("compaction did not cancel active response")
	}
	tty.Send(t, "after running compaction\r")
	tty.Wait(t, "AFTER_RUNNING_COMPACTION")
	exitQueues116(t, tty, done)
}
