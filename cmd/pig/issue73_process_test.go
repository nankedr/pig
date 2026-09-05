//go:build !windows

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nankedr/pig/codingagent"
)

func TestPigContinueSelectAndForkSessions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct{ Role string }
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		count := 0
		for _, m := range body.Messages {
			if m.Role == "user" {
				count++
			}
		}
		w.Header().Set("Content-Type", "text/event-stream")
		text := strings.Repeat("x", count)
		_, _ = io.WriteString(w, `data: {"id":"reply","model":"deepseek-v4-flash","choices":[{"delta":{"content":"`+text+`"},"finish_reason":"stop"}]}`+"\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	binary := buildPigBinary(t)
	cwd, dir := t.TempDir(), t.TempDir()
	cwd, _ = filepath.EvalSymlinks(cwd)
	run := func(args ...string) (string, string, error) {
		t.Helper()
		base := []string{"--provider", "deepseek", "--model", "deepseek-v4-flash", "--api-key", "key", "--session-dir", dir}
		cmd := exec.Command(binary, append(base, args...)...)
		cmd.Dir = cwd
		cmd.Env = append(filteredEnvironment(os.Environ(), "PIG_DEEPSEEK_BASE_URL", "PIG_CODING_AGENT_DIR", "PIG_CODING_AGENT_SESSION_DIR"), "PIG_DEEPSEEK_BASE_URL="+server.URL, "PIG_CODING_AGENT_DIR="+t.TempDir())
		var out, stderr bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &stderr
		err := cmd.Run()
		return out.String(), stderr.String(), err
	}
	for i, args := range [][]string{{"--session-id", "original", "--name", "named", "-p", "first"}, {"--continue", "-p", "second"}, {"--session", "orig", "-p", "third"}} {
		out, stderr, err := run(args...)
		if err != nil || out != strings.Repeat("x", i+1)+"\n" {
			t.Fatalf("%v: %q %q %v", args, out, stderr, err)
		}
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil || len(files) != 1 {
		t.Fatal(files, err)
	}
	source := files[0]
	before, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	out, stderr, err := run("--fork", "orig", "--session-id", "forked", "-p", "fourth")
	if err != nil || out != "xxxx\n" {
		t.Fatal(out, stderr, err)
	}
	after, err := os.ReadFile(source)
	if err != nil || !bytes.Equal(after, before) {
		t.Fatal("fork modified source", err)
	}
	all, err := codingagent.ListSessions(t.Context(), cwd, codingagent.SessionListOptions{SessionDir: &dir})
	if err != nil || len(all) != 2 {
		t.Fatal(all, err)
	}
	var fork *codingagent.SessionInfo
	for i := range all {
		if all[i].ID == "forked" {
			fork = &all[i]
		}
	}
	if fork == nil || fork.Name == nil || *fork.Name != "named" || fork.ParentSessionPath == nil || *fork.ParentSessionPath != source {
		t.Fatalf("fork metadata=%+v", fork)
	}
	_, stderr, err = run("--session", "missing-id", "-p", "bad")
	if err == nil || !strings.Contains(stderr, "No session found matching 'missing-id'") {
		t.Fatal(stderr, err)
	}
	_, stderr, err = run("--fork", "orig", "--session-id", "forked", "-p", "duplicate")
	if err == nil || !strings.Contains(stderr, "already exists") {
		t.Fatal(stderr, err)
	}
}
