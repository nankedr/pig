//go:build !windows

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestPigProcessRunsHeadlessTextWithExplicitDeepSeekInputs(t *testing.T) {
	request := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer explicit-test-key" {
			t.Errorf("Authorization = %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		request <- body
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl-headless\",\"model\":\"deepseek-v4-flash\",\"choices\":[{\"delta\":{\"content\":\"hello from subprocess\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":3}}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()

	binary := buildPigBinary(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, binary,
		"--provider", "deepseek",
		"--model", "deepseek-v4-flash",
		"--api-key", "explicit-test-key",
		"--print", "say hello",
	)
	command.Env = append(filteredEnvironment(os.Environ(), "DEEPSEEK_API_KEY", "PIG_DEEPSEEK_BASE_URL"), "PIG_DEEPSEEK_BASE_URL="+server.URL)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		t.Fatalf("pig process error = %v; stderr = %q", err, stderr.String())
	}
	if got, want := stdout.String(), "hello from subprocess\n"; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
	select {
	case body := <-request:
		if body["model"] != "deepseek-v4-flash" || body["stream"] != true {
			t.Fatalf("request body = %#v", body)
		}
		messages, ok := body["messages"].([]any)
		if !ok || len(messages) != 2 {
			t.Fatalf("request messages = %#v", body["messages"])
		}
		user := messages[1].(map[string]any)
		content := user["content"].([]any)
		if user["role"] != "user" || len(content) != 1 || content[0].(map[string]any)["text"] != "say hello" {
			t.Fatalf("request user message = %#v", user)
		}
	case <-ctx.Done():
		t.Fatal("local DeepSeek endpoint did not receive a request")
	}
}

func TestPigProcessUsesStandardDeepSeekAPIKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer ambient-test-key" {
			t.Errorf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl-env\",\"model\":\"deepseek-v4-flash\",\"choices\":[{\"delta\":{\"content\":\"ambient key works\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()

	command := exec.Command(buildPigBinary(t), "--provider", "deepseek", "--model", "deepseek-v4-flash", "-p", "hello")
	command.Env = append(os.Environ(), "PIG_DEEPSEEK_BASE_URL="+server.URL, "DEEPSEEK_API_KEY=ambient-test-key")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("pig process error = %v; output = %q", err, output)
	}
	if got, want := string(output), "ambient key works\n"; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
}

func TestPigProcessReportsStableHeadlessFailures(t *testing.T) {
	binary := buildPigBinary(t)
	tests := []struct {
		name      string
		arguments []string
		want      string
	}{
		{name: "missing provider", arguments: []string{"-p", "hello"}, want: "Error: Headless text mode requires --provider <provider>\n"},
		{name: "unknown provider", arguments: []string{"--provider", "unknown", "--model", "model", "-p", "hello"}, want: "Error: Unknown provider \"unknown\"\n"},
		{name: "provider error", arguments: []string{"--provider", "deepseek", "--model", "deepseek-v4-flash", "-p", "hello"}, want: "auth: Provider is not configured: deepseek\n"},
		{name: "json stub", arguments: []string{"--mode", "json", "--provider", "deepseek", "--model", "deepseek-v4-flash", "hello"}, want: "codingagent.mode.json: not implemented\n"},
		{name: "persistent session stub", arguments: []string{"--provider", "deepseek", "--model", "deepseek-v4-flash", "--session", "old.jsonl", "-p", "hello"}, want: "codingagent.headless.session-persistence: not implemented\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := exec.Command(binary, test.arguments...)
			command.Env = append(filteredEnvironment(os.Environ(), "DEEPSEEK_API_KEY", "PIG_DEEPSEEK_BASE_URL"), "DEEPSEEK_API_KEY=")
			var stdout, stderr bytes.Buffer
			command.Stdout = &stdout
			command.Stderr = &stderr
			err := command.Run()
			if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
				t.Fatalf("process error = %v, want exit 1", err)
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want empty", stdout.String())
			}
			if got := stderr.String(); got != test.want {
				t.Fatalf("stderr = %q, want %q", got, test.want)
			}
		})
	}
}

func TestPigProcessSIGINTCancelsHeadlessRun(t *testing.T) {
	requestStarted := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("response writer does not support flushing")
			return
		}
		_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl-signal\",\"model\":\"deepseek-v4-flash\",\"choices\":[{\"delta\":{\"content\":\"partial\"},\"finish_reason\":null}]}\n\n")
		flusher.Flush()
		close(requestStarted)
		<-r.Context().Done()
	}))
	defer server.Close()

	command := exec.Command(buildPigBinary(t), "--provider", "deepseek", "--model", "deepseek-v4-flash", "--api-key", "signal-key", "-p", "hello")
	command.Env = append(os.Environ(), "PIG_DEEPSEEK_BASE_URL="+server.URL)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-requestStarted:
	case <-time.After(5 * time.Second):
		_ = command.Process.Kill()
		t.Fatal("pig did not start its Provider request")
	}
	if err := command.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	err := command.Wait()
	if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 130 {
		t.Fatalf("process error = %v, want exit 130", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want no successful final output", stdout.String())
	}
	if got, want := stderr.String(), "stream: request canceled\n"; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
}

func filteredEnvironment(environment []string, names ...string) []string {
	filtered := make([]string, 0, len(environment))
	for _, entry := range environment {
		remove := false
		for _, name := range names {
			if strings.HasPrefix(entry, name+"=") {
				remove = true
				break
			}
		}
		if !remove {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}
