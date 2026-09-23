//go:build darwin || linux

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
	"github.com/nankedr/pig/internal/terminaltest"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPigModelSelection117(t *testing.T) {
	requests := make(chan map[string]any, 10)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		requests <- body
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"MODEL_DONE\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	tty, _, done := startQueues116(t, server.URL, `{"tuiMode":"regular","compaction":{"enabled":false}}`)
	tty.Send(t, "/model\r")
	tty.Wait(t, "Select Model")
	tty.Send(t, "v4-pro\r")
	tty.Wait(t, "Model: deepseek-v4-pro")
	tty.Send(t, "/settings\r")
	tty.Wait(t, "Thinking level")
	tty.Send(t, "\r")
	tty.Wait(t, "Thinking Level")
	tty.Send(t, "\x1b[A\r")
	tty.Wait(t, "Thinking: off")
	waitScreen116(t, tty, func(screen string) bool { return strings.Contains(screen, "Type to search") })
	tty.Send(t, "\x1b[27u")
	waitScreen116(t, tty, func(screen string) bool { return !strings.Contains(screen, "Type to search") })
	tty.Send(t, "question\r")
	tty.Wait(t, "MODEL_DONE")
	select {
	case r := <-requests:
		if r["model"] != "deepseek-v4-pro" {
			t.Fatal(r)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no request")
	}
	exitQueues116(t, tty, done)
}

func startModels117(t *testing.T, root, url, session string) (*terminaltest.Terminal, <-chan error) {
	t.Helper()
	tty := terminaltest.Open(t)
	binary := buildPigBinary(t)
	if os.Getenv("PIG_TEST_RACE") == "1" {
		cmd := exec.Command("go", "build", "-race", "-o", binary, ".")
		if output, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%s: %v", output, err)
		}
	}
	args := []string{"--no-tools", "--no-skills", "--no-extensions", "--session-dir", filepath.Join(root, "sessions")}
	if session == "" {
		args = append(args, "--provider", "deepseek", "--model", "deepseek-v4-flash")
	} else {
		args = append(args, "--session", session)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = root
	cmd.Env = []string{"HOME=" + root, "PATH=" + os.Getenv("PATH"), "TERM=xterm-256color", "DEEPSEEK_API_KEY=synthetic", "PIG_DEEPSEEK_BASE_URL=" + url}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = tty.Slave, tty.Slave, tty.Slave
	if err := cmd.Start(); err != nil {
		cancel()
		t.Fatal(err)
	}
	done := make(chan error, 1)
	reaped := make(chan struct{})
	go func() { done <- cmd.Wait(); close(reaped) }()
	t.Cleanup(func() { cancel(); <-reaped })
	tty.Wait(t, "> ")
	return tty, done
}
func TestPigModelSelectionParity117(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".pig", "agent")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"tuiMode":"regular","defaultThinkingLevel":"high","compaction":{"enabled":false}}`), 0600); err != nil {
		t.Fatal(err)
	}
	requests := make(chan map[string]any, 10)
	count := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		requests <- body
		count++
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"DONE_%d\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", count)
	}))
	defer server.Close()
	tty, done := startModels117(t, root, server.URL, "")
	tty.Send(t, "before question\r")
	tty.Wait(t, "DONE_1")
	time.Sleep(100 * time.Millisecond)
	tty.Send(t, "/model\r")
	tty.Wait(t, "Select Model")
	tty.Send(t, "v4-pro\r")
	tty.Wait(t, "Model: deepseek-v4-pro")
	tty.Send(t, "\x1b[Z")
	tty.Wait(t, "Thinking: max")
	tty.Send(t, "/scoped-models\r")
	tty.Wait(t, "Model Configuration")
	tty.Send(t, "flash\r")
	tty.Wait(t, "unsaved")
	tty.Send(t, "\x13")
	tty.Wait(t, "Model selection saved to settings")
	tty.Send(t, "\x1b[27u")
	time.Sleep(100 * time.Millisecond)
	tty.Send(t, "\x10")
	tty.Wait(t, "No other available models")
	tty.Send(t, "after question\r")
	tty.Wait(t, "DONE_2")
	time.Sleep(100 * time.Millisecond)
	tty.Send(t, "/model\r")
	time.Sleep(150 * time.Millisecond)
	tty.Send(t, "\x1b[27u")
	time.Sleep(100 * time.Millisecond)
	tty.Send(t, "cancel question\r")
	tty.Wait(t, "DONE_3")
	time.Sleep(100 * time.Millisecond)
	exitQueues116(t, tty, done)
	files, err := filepath.Glob(filepath.Join(root, "sessions", "*.jsonl"))
	if err != nil || len(files) != 1 {
		t.Fatal(files, err)
	}
	second, done := startModels117(t, root, server.URL, files[0])
	second.Send(t, "restored question\r")
	second.Wait(t, "DONE_4")
	time.Sleep(100 * time.Millisecond)
	exitQueues116(t, second, done)
	gotRequests := []map[string]any{}
	for i := 0; i < 4; i++ {
		select {
		case r := <-requests:
			var enabled any
			if thinking, ok := r["thinking"].(map[string]any); ok {
				enabled = thinking["type"]
			}
			gotRequests = append(gotRequests, map[string]any{"model": r["model"], "thinking": r["reasoning_effort"], "enabled": enabled})
		case <-time.After(time.Second):
			t.Fatal("missing request")
		}
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	configs := []map[string]any{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var row map[string]any
		if err = json.Unmarshal([]byte(line), &row); err != nil {
			t.Fatal(err)
		}
		switch row["type"] {
		case "model_change":
			configs = append(configs, map[string]any{"model": row["modelId"]})
		case "thinking_level_change":
			configs = append(configs, map[string]any{"thinking": row["thinkingLevel"]})
		}
	}
	data, err = os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var settings map[string]any
	_ = json.Unmarshal(data, &settings)
	saved := map[string]any{}
	for _, key := range []string{"defaultProvider", "defaultModel", "defaultThinkingLevel", "enabledModels"} {
		saved[key] = settings[key]
	}
	lock, _, err := baseline.Load("../../parity/baseline")
	if err != nil {
		t.Fatal(err)
	}
	locked := parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository}
	fixture, err := parity.LoadFixture("../../parity/oracle/fixtures/model-selection-cli.json", locked)
	if err != nil {
		t.Fatal(err)
	}
	oracle, err := parity.NewFixtureDriver(fixture, locked)
	if err != nil {
		t.Fatal(err)
	}
	outcome, _ := json.Marshal(map[string]any{"requests": gotRequests, "configs": configs, "settings": saved, "terminal_restored": []bool{tty.Restored(t), second.Restored(t)}})
	result, err := parity.RunCase(context.Background(), fixture.Case, oracle, parity.DriverFunc{SurfaceName: parity.SurfaceCLI, ObserveFunc: func(context.Context, parity.Case) (parity.Observation, error) {
		return parity.Observation{Outcome: outcome, SideEffects: &[]parity.SideEffect{}}, nil
	}})
	if err != nil || !result.Match {
		t.Fatalf("%v\nwant %s\ngot %s", err, fixture.Observation.Outcome, outcome)
	}
}
