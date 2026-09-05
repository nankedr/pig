//go:build !windows

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
)

func TestPigGlobalSettingsStartupParity(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	lock, _, err := baseline.Load(filepath.Join(root, "parity/baseline"))
	if err != nil {
		t.Fatal(err)
	}
	locked := parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository}
	fixture, err := parity.LoadFixture(filepath.Join(root, "parity/oracle/fixtures/settings-startup.json"), locked)
	if err != nil {
		t.Fatal(err)
	}
	oracle, err := parity.NewFixtureDriver(fixture, locked)
	if err != nil {
		t.Fatal(err)
	}
	binary := buildPigBinary(t)
	result, err := parity.RunCase(context.Background(), fixture.Case, oracle, parity.DriverFunc{SurfaceName: parity.SurfaceCLI, ObserveFunc: func(ctx context.Context, _ parity.Case) (parity.Observation, error) {
		var mu sync.Mutex
		var request map[string]any
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			mu.Lock()
			request = body
			mu.Unlock()
			w.Header().Set("Content-Type", "text/event-stream")
			fmt.Fprint(w, "data: {\"id\":\"fixture\",\"choices\":[{\"delta\":{\"content\":\"reply\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
		}))
		defer server.Close()
		dir := t.TempDir()
		cwd := t.TempDir()
		configured := filepath.Join(dir, "configured")
		environment := filepath.Join(dir, "environment")
		explicit := filepath.Join(dir, "explicit")
		data, _ := json.Marshal(map[string]any{"sessionDir": configured})
		if err := os.WriteFile(filepath.Join(dir, "settings.json"), data, 0o600); err != nil {
			t.Fatal(err)
		}
		// Reading this project settings FIFO would block startup until the test deadline.
		if err := os.MkdirAll(filepath.Join(cwd, ".pig"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := syscall.Mkfifo(filepath.Join(cwd, ".pig/settings.json"), 0o600); err != nil {
			t.Fatal(err)
		}
		settings, err := codingagent.NewSettingsManager(cwd, &dir)
		if err != nil {
			return parity.Observation{}, err
		}
		if err = settings.SetDefaultModelAndProvider("deepseek", "deepseek-v4-flash"); err != nil {
			return parity.Observation{}, err
		}
		if err = settings.SetDefaultThinkingLevel("high"); err != nil {
			return parity.Observation{}, err
		}
		if err = settings.Flush(ctx); err != nil {
			return parity.Observation{}, err
		}
		run := func(extra []string, envDir string) {
			t.Helper()
			runCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			defer cancel()
			command := exec.CommandContext(runCtx, binary, append([]string{"-p", "hello"}, extra...)...)
			command.Dir = cwd
			command.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + dir, "PIG_CODING_AGENT_DIR=" + dir, "DEEPSEEK_API_KEY=fixture", "PIG_DEEPSEEK_BASE_URL=" + server.URL, "PIG_CODING_AGENT_SESSION_DIR=" + envDir}
			var stdout, stderr bytes.Buffer
			command.Stdout, command.Stderr = &stdout, &stderr
			if err := command.Run(); err != nil || stderr.Len() != 0 || stdout.String() != "reply\n" {
				t.Fatalf("pig %v: %v, out=%q err=%q", extra, err, stdout.String(), stderr.String())
			}
		}
		cases := []map[string]any{}
		capture := func(name string, args []string, storage, env string) string {
			t.Helper()
			run(args, env)
			files, err := filepath.Glob(filepath.Join(storage, "*.jsonl"))
			if err != nil || len(files) == 0 {
				t.Fatalf("files: %v %v", files, err)
			}
			file := files[len(files)-1]
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			thinking := ""
			for _, entry := range codingagent.ParseSessionEntries(string(data)) {
				if entry.Entry != nil && entry.Entry.Type == "thinking_level_change" {
					thinking = entry.Entry.ThinkingLevel
					break
				}
			}
			mu.Lock()
			model := request["model"]
			mu.Unlock()
			cases = append(cases, map[string]any{"name": name, "model": model, "thinking": thinking, "sessionDir": filepath.Dir(file) == storage})
			return file
		}
		first := capture("settings", nil, configured, "")
		if err = settings.SetDefaultModel("deepseek-v4-pro"); err != nil {
			t.Fatal(err)
		}
		if err = settings.SetDefaultThinkingLevel("low"); err != nil {
			t.Fatal(err)
		}
		if err = settings.Flush(ctx); err != nil {
			t.Fatal(err)
		}
		capture("restart", nil, configured, "")
		run([]string{"--session", first}, "")
		mu.Lock()
		roles := []any{}
		for _, value := range request["messages"].([]any) {
			roles = append(roles, value.(map[string]any)["role"])
		}
		cases = append(cases, map[string]any{"name": "reopen", "model": request["model"], "thinking": request["reasoning_effort"], "history": roles})
		mu.Unlock()
		capture("explicit", []string{"--model", "deepseek/deepseek-v4-flash:low", "--thinking", "off", "--session-dir", explicit}, explicit, environment)
		capture("environment", nil, environment, environment)
		outcome, err := json.Marshal(map[string]any{"cases": cases})
		effects := []parity.SideEffect{}
		return parity.Observation{Outcome: outcome, SideEffects: &effects}, err
	}})
	if err != nil || !result.Match {
		t.Fatalf("startup parity = %+v, %v; Pig=%s Oracle=%s", result, err, result.Pig.Outcome, result.Oracle.Outcome)
	}
}

func TestPigMalformedGlobalSettingsAreObservable(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte("{invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	command := exec.Command(buildPigBinary(t), "-p", "hello")
	command.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + dir, "PIG_CODING_AGENT_DIR=" + dir}
	output, err := command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "global settings:") {
		t.Fatalf("malformed settings: %v %s", err, output)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "{invalid" {
		t.Fatalf("corrupt settings changed: %q %v", data, err)
	}
}
