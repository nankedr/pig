package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPigModelRuntime77(t *testing.T) {
	binary := buildPigBinary(t)
	home, cwd := t.TempDir(), t.TempDir()
	requests := make(chan map[string]any, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		requests <- body
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"runtime-reply\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	run := func(args ...string) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, binary, args...)
		cmd.Dir = cwd
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "DEEPSEEK_API_KEY=fixture", "PIG_DEEPSEEK_BASE_URL=" + server.URL, "PIG_OFFLINE=yes"}
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
	out, err := run("--list-models", "v4fl")
	if err != nil || !strings.Contains(out, "deepseek-v4-flash") || strings.Contains(out, "deepseek-v4-pro") {
		t.Fatalf("list: %s %v", out, err)
	}
	if len(requests) != 0 {
		t.Fatal("listing made a provider request")
	}
	out, err = run("--offline", "--models", "deepseek-v4-flash:high", "--thinking", "off", "--no-tools", "--session-dir", filepath.Join(home, "sessions"), "-p", "hello")
	if err != nil || !strings.Contains(out, "runtime-reply") {
		t.Fatalf("scope: %s %v", out, err)
	}
	body := <-requests
	if body["model"] != "deepseek-v4-flash" {
		t.Fatalf("scope model: %v", body)
	}
	out, err = run("--provider", "deepseek", "--model", "future-model:high", "--no-tools", "--no-session", "-p", "hello")
	if err != nil || !strings.Contains(out, "Using custom model id.") || !strings.Contains(out, "runtime-reply") {
		t.Fatalf("fallback: %s %v", out, err)
	}
	body = <-requests
	if body["model"] != "future-model" {
		t.Fatalf("custom model: %v", body)
	}
	out, err = run("--model", "no-such-model", "--no-tools", "--no-session", "-p", "hello")
	if err == nil || !strings.Contains(out, "not found") {
		t.Fatalf("missing model: %s %v", out, err)
	}
	if len(requests) != 0 {
		t.Fatal("missing model reached inference")
	}
}
