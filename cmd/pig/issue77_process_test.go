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
	out, err = run("--api-key", "fixture", "--no-tools", "--no-session", "-p", "hello")
	if err == nil || !strings.Contains(out, "--api-key requires a model") {
		t.Fatalf("unscoped key: %s %v", out, err)
	}
	settingsDir := filepath.Join(home, ".pig", "agent")
	if err = os.MkdirAll(settingsDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(settingsDir, "settings.json"), []byte(`{"defaultProvider":"deepseek","defaultModel":"deepseek-v4-pro","enabledModels":["deepseek/deepseek-v4-flash:high"]}`), 0600); err != nil {
		t.Fatal(err)
	}
	out, err = run("--no-session", "--no-tools", "-p", "settings scope")
	if err != nil {
		t.Fatalf("settings scope: %s %v", out, err)
	}
	body = <-requests
	if body["model"] != "deepseek-v4-flash" {
		t.Fatal("enabledModels ignored")
	}
	out, err = run("--models", "deepseek-v4-pro", "--no-session", "--no-tools", "-p", "explicit scope")
	if err != nil {
		t.Fatalf("explicit scope: %s %v", out, err)
	}
	body = <-requests
	if body["model"] != "deepseek-v4-pro" {
		t.Fatal("settings overrode explicit scope")
	}
	out, err = run("--model", "no-such-model", "--no-tools", "--no-session", "-p", "hello")
	if err == nil || !strings.Contains(out, "not found") {
		t.Fatalf("missing model: %s %v", out, err)
	}
	if len(requests) != 0 {
		t.Fatal("missing model reached inference")
	}
}
