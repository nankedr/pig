//go:build darwin || linux

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestPigBashParity122(t *testing.T) {
	binary := buildPigBinary(t)
	if os.Getenv("PIG_TEST_RACE") == "1" {
		cmd := exec.Command("go", "build", "-race", "-o", binary, ".")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v", output, err)
		}
	}
	cmd := exec.Command("python3", "../../parity/terminal/bash.py", binary, "--pig")
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
	fixture, err := parity.LoadFixture("../../parity/oracle/fixtures/bash-cli.json", locked)
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

func TestPigBashCancelPriorityAndResume122(t *testing.T) {
	cancelled := make(chan struct{})
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"GENERATING\"}}]}\n\n")
		w.(http.Flusher).Flush()
		if calls.Add(1) == 1 {
			<-r.Context().Done()
			close(cancelled)
			return
		}
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\" RECOVERED\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	tty, root, done := startQueues116(t, server.URL, `{"quietStartup":true,"compaction":{"enabled":false}}`)
	tty.Send(t, "question\r")
	tty.Wait(t, "GENERATING")
	pidPath := filepath.Join(root, "bash.pid")
	tty.Send(t, "!echo $$ > bash.pid; printf 'PARALLEL_%s\\n' BASH; exec sleep 30\r")
	tty.Wait(t, "PARALLEL_BASH")
	tty.Send(t, "\x1b[27u")
	select {
	case <-cancelled:
	case <-time.After(5 * time.Second):
		t.Fatal("Agent was not cancelled first")
	}
	pidText, err := os.ReadFile(pidPath)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidText)))
	if err != nil {
		t.Fatal(err)
	}
	if err = syscall.Kill(pid, 0); err != nil {
		t.Fatal("Bash cancelled before Agent", err)
	}
	tty.Wait(t, "stream: request canceled")
	tty.Send(t, "\x1b[27u")
	tty.Wait(t, "(cancelled)")
	tty.Send(t, "after question\r")
	tty.Wait(t, "RECOVERED")
	tty.Send(t, "/new\r")
	tty.Wait(t, "Started new session")
	files, _ := filepath.Glob(filepath.Join(root, "sessions", "*.jsonl"))
	if len(files) != 1 {
		t.Fatal("old Session missing", files)
	}
	tty.Send(t, "/resume\r")
	tty.Wait(t, "Resume Session")
	tty.Wait(t, "Enter select")
	tty.Send(t, "\r")
	tty.Wait(t, "Resumed session")
	exitQueues116(t, tty, done)
	manager, err := codingagent.OpenSessionManager(files[0], nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, m := range manager.BuildSessionContext().Messages {
		if m.MessageRole() == "bashExecution" {
			count++
			data, _ := agent.MarshalAgentMessage(m)
			var b struct {
				Cancelled bool
				Output    string
			}
			_ = json.Unmarshal(data, &b)
			if !b.Cancelled || b.Output != "PARALLEL_BASH\n" {
				t.Fatal("lost cancelled record", string(data))
			}
		}
	}
	if count != 1 {
		t.Fatal("duplicated or missing Bash record", count)
	}
}
