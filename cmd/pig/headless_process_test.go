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
	"path/filepath"
	"strings"
	"sync/atomic"
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

func TestPigProcessStreamsSessionFirstHeadlessJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl-json\",\"model\":\"deepseek-v4-flash\",\"choices\":[{\"delta\":{\"content\":\"hello json\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":3}}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()

	command := exec.Command(buildPigBinary(t),
		"--mode", "json",
		"--provider", "deepseek",
		"--model", "deepseek-v4-flash",
		"--api-key", "json-test-key",
		"say hello",
	)
	command.Env = append(os.Environ(), "PIG_DEEPSEEK_BASE_URL="+server.URL)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		t.Fatalf("pig process error = %v; stderr = %q", err, stderr.String())
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}

	records := decodeJSONLines(t, stdout.String())
	if len(records) < 2 {
		t.Fatalf("JSONL records = %d, want header and events; stdout = %q", len(records), stdout.String())
	}
	header := records[0]
	if header["type"] != "session" || header["version"] != float64(3) || header["id"] == "" || header["cwd"] == "" {
		t.Fatalf("first JSONL record = %#v, want complete in-memory Session header", header)
	}
	timestamp, ok := header["timestamp"].(string)
	if !ok {
		t.Fatalf("Session header timestamp = %#v, want RFC3339 string", header["timestamp"])
	}
	if _, err := time.Parse(time.RFC3339Nano, timestamp); err != nil {
		t.Fatalf("Session header timestamp = %q, want RFC3339: %v", timestamp, err)
	}

	wantTypes := []string{
		"agent_start", "turn_start",
		"message_start", "message_end",
		"message_start",
		"message_update", "message_update", "message_update",
		"message_end", "turn_end", "agent_end", "agent_settled",
	}
	gotTypes := make([]string, 0, len(records)-1)
	for index, record := range records[1:] {
		eventType, ok := record["type"].(string)
		if !ok || eventType == "" {
			t.Fatalf("event record %d has no type: %#v", index+1, record)
		}
		gotTypes = append(gotTypes, eventType)
		if eventType != "message_update" {
			continue
		}
		if _, ok := record["message"]; ok {
			t.Fatalf("message_update record %d leaked cumulative message: %#v", index+1, record)
		}
		assistantEvent, ok := record["assistantMessageEvent"].(map[string]any)
		if !ok {
			t.Fatalf("message_update record %d has invalid assistant event: %#v", index+1, record)
		}
		if _, ok := assistantEvent["partial"]; ok {
			t.Fatalf("message_update record %d leaked partial snapshot: %#v", index+1, record)
		}
	}
	if got, want := strings.Join(gotTypes, ","), strings.Join(wantTypes, ","); got != want {
		t.Fatalf("event order = %q, want %q", got, want)
	}
	assertHeadlessJSONGolden(t, records)
}

func TestPigProcessStreamsHeadlessJSONToolContinuation(t *testing.T) {
	const sentinel = "issue-57-json-read-sentinel"
	cwd := t.TempDir()
	if err := os.WriteFile(filepath.Join(cwd, "sentinel.txt"), []byte(sentinel), 0o600); err != nil {
		t.Fatal(err)
	}

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		if requestCount.Add(1) == 1 {
			_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl-json-tool\",\"model\":\"deepseek-v4-flash\",\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"read-json-sentinel\",\"type\":\"function\",\"function\":{\"name\":\"read\",\"arguments\":\"{\\\"path\\\":\\\"sentinel.txt\\\"}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\ndata: [DONE]\n\n")
			return
		}
		_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl-json-final\",\"model\":\"deepseek-v4-flash\",\"choices\":[{\"delta\":{\"content\":\"read complete\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()

	command := exec.Command(buildPigBinary(t),
		"--mode", "json",
		"--provider", "deepseek",
		"--model", "deepseek-v4-flash",
		"--api-key", "json-tool-key",
		"--tools", "read",
		"read sentinel.txt",
	)
	command.Dir = cwd
	command.Env = append(os.Environ(), "PIG_DEEPSEEK_BASE_URL="+server.URL)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		t.Fatalf("pig process error = %v; stderr = %q", err, stderr.String())
	}
	if requestCount.Load() != 2 {
		t.Fatalf("Provider request count = %d, want 2", requestCount.Load())
	}
	records := decodeJSONLines(t, stdout.String())
	wantTypes := []string{
		"agent_start", "turn_start", "message_start", "message_end", "message_start",
		"message_update", "message_update", "message_update", "message_end",
		"tool_execution_start", "tool_execution_end", "message_start", "message_end", "turn_end",
		"turn_start", "message_start", "message_update", "message_update", "message_update",
		"message_end", "turn_end", "agent_end", "agent_settled",
	}
	gotTypes := make([]string, 0, len(records)-1)
	var toolResult map[string]any
	for _, record := range records[1:] {
		eventType, _ := record["type"].(string)
		gotTypes = append(gotTypes, eventType)
		if eventType == "tool_execution_end" {
			toolResult = record
		}
	}
	if got, want := strings.Join(gotTypes, ","), strings.Join(wantTypes, ","); got != want {
		t.Fatalf("Tool event order = %q, want %q", got, want)
	}
	if toolResult == nil || toolResult["toolCallId"] != "read-json-sentinel" || toolResult["toolName"] != "read" || toolResult["isError"] != false {
		t.Fatalf("tool_execution_end = %#v", toolResult)
	}
	result := toolResult["result"].(map[string]any)
	content := result["content"].([]any)
	if len(content) != 1 || content[0].(map[string]any)["text"] != sentinel {
		t.Fatalf("read Tool result = %#v, want sentinel", result)
	}
}

func TestPigProcessStreamsHeadlessJSONProviderError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Should-Retry", "false")
		http.Error(w, "provider unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	command := exec.Command(buildPigBinary(t),
		"--mode", "json",
		"--provider", "deepseek",
		"--model", "deepseek-v4-flash",
		"--api-key", "json-error-key",
		"fail",
	)
	command.Env = append(os.Environ(), "PIG_DEEPSEEK_BASE_URL="+server.URL)
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		t.Fatalf("JSON Provider failure exit = %v, want Pi-compatible zero; stderr = %q", err, stderr.String())
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("stderr = %q, want Provider failure only in JSON events", got)
	}
	records := decodeJSONLines(t, stdout.String())
	last := records[len(records)-1]
	if last["type"] != "agent_settled" {
		t.Fatalf("last JSON event = %#v, want agent_settled", last)
	}
	var failure map[string]any
	for _, record := range records[1:] {
		if record["type"] != "message_end" {
			continue
		}
		message, _ := record["message"].(map[string]any)
		if message["role"] == "assistant" && message["stopReason"] == "error" {
			failure = message
		}
	}
	if failure == nil || !strings.Contains(failure["errorMessage"].(string), "provider response 503") {
		t.Fatalf("Provider failure event = %#v", failure)
	}
}

func TestPigProcessTreatsPipedJSONAsPromptNotRPCCommand(t *testing.T) {
	request := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		request <- body
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl-json-stdin\",\"model\":\"deepseek-v4-flash\",\"choices\":[{\"delta\":{\"content\":\"literal prompt\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()

	const input = `{"type":"prompt","message":"not an RPC command"}`
	command := exec.Command(buildPigBinary(t),
		"--mode", "json",
		"--provider", "deepseek",
		"--model", "deepseek-v4-flash",
		"--api-key", "json-stdin-key",
	)
	command.Env = append(os.Environ(), "PIG_DEEPSEEK_BASE_URL="+server.URL)
	command.Stdin = strings.NewReader(input + "\n")
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		t.Fatalf("pig process error = %v; stderr = %q", err, stderr.String())
	}
	decodeJSONLines(t, stdout.String())

	body := <-request
	messages, ok := body["messages"].([]any)
	if !ok || len(messages) != 2 {
		t.Fatalf("request messages = %#v", body["messages"])
	}
	user := messages[1].(map[string]any)
	content := user["content"].([]any)
	if got := content[0].(map[string]any)["text"]; got != input {
		t.Fatalf("piped JSON became prompt %q, want literal %q", got, input)
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
		{name: "missing provider", arguments: []string{"-p", "hello"}, want: "Error: Headless mode requires --provider <provider>\n"},
		{name: "JSON missing provider", arguments: []string{"--mode", "json", "hello"}, want: "Error: Headless mode requires --provider <provider>\n"},
		{name: "unknown provider", arguments: []string{"--provider", "unknown", "--model", "model", "-p", "hello"}, want: "Error: Unknown provider \"unknown\"\n"},
		{name: "provider error", arguments: []string{"--provider", "deepseek", "--model", "deepseek-v4-flash", "-p", "hello"}, want: "auth: Provider is not configured: deepseek\n"},
		{name: "rpc stub", arguments: []string{"--mode", "rpc", "--provider", "deepseek", "--model", "deepseek-v4-flash"}, want: "codingagent.mode.rpc: not implemented\n"},
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

func TestPigProcessSIGINTCompletesHeadlessJSONStream(t *testing.T) {
	requestStarted := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("response writer does not support flushing")
			return
		}
		_, _ = io.WriteString(w, "data: {\"id\":\"chatcmpl-json-signal\",\"model\":\"deepseek-v4-flash\",\"choices\":[{\"delta\":{\"content\":\"partial\"},\"finish_reason\":null}]}\n\n")
		flusher.Flush()
		close(requestStarted)
		<-r.Context().Done()
	}))
	defer server.Close()

	command := exec.Command(buildPigBinary(t),
		"--mode", "json",
		"--provider", "deepseek",
		"--model", "deepseek-v4-flash",
		"--api-key", "json-signal-key",
		"hello",
	)
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
	if got, want := stderr.String(), "stream: request canceled\n"; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
	}
	records := decodeJSONLines(t, stdout.String())
	if records[0]["type"] != "session" || records[len(records)-1]["type"] != "agent_settled" {
		t.Fatalf("canceled JSONL boundaries = first %#v, last %#v", records[0], records[len(records)-1])
	}
	var aborted bool
	for _, record := range records[1:] {
		if record["type"] != "message_end" {
			continue
		}
		message, _ := record["message"].(map[string]any)
		if message["role"] == "assistant" && message["stopReason"] == "aborted" {
			aborted = true
		}
	}
	if !aborted {
		t.Fatalf("canceled JSONL has no aborted Assistant message: %#v", records)
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

func decodeJSONLines(t *testing.T, output string) []map[string]any {
	t.Helper()
	if output == "" || !strings.HasSuffix(output, "\n") {
		t.Fatalf("stdout = %q, want non-empty newline-terminated JSONL", output)
	}
	lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
	records := make([]map[string]any, len(lines))
	for index, line := range lines {
		if line == "" {
			t.Fatalf("stdout line %d is empty: %q", index+1, output)
		}
		if err := json.Unmarshal([]byte(line), &records[index]); err != nil {
			t.Fatalf("decode stdout line %d %q: %v", index+1, line, err)
		}
	}
	return records
}

func assertHeadlessJSONGolden(t *testing.T, records []map[string]any) {
	t.Helper()
	for _, record := range records {
		normalizeHeadlessJSONValue(record)
		if record["type"] == "session" {
			record["id"] = "SESSION_ID"
			record["cwd"] = "CWD"
		}
	}
	var snapshot strings.Builder
	for _, record := range records {
		encoded, err := json.Marshal(record)
		if err != nil {
			t.Fatalf("encode normalized JSONL record: %v", err)
		}
		snapshot.Write(encoded)
		snapshot.WriteByte('\n')
	}
	path := filepath.Join("testdata", "headless_json_text.golden.jsonl")
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := snapshot.String(); got != string(want) {
		t.Fatalf("Headless JSON golden drifted\n--- got ---\n%s--- want ---\n%s", got, want)
	}
}

func normalizeHeadlessJSONValue(value any) {
	switch value := value.(type) {
	case map[string]any:
		for key, child := range value {
			if key == "timestamp" {
				value[key] = "TIMESTAMP"
				continue
			}
			normalizeHeadlessJSONValue(child)
		}
	case []any:
		for _, child := range value {
			normalizeHeadlessJSONValue(child)
		}
	}
}
