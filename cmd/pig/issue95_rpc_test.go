package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/codingagent"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRPC95ParityControl(t *testing.T) {
	var fixture struct {
		Case struct {
			Input struct{ Commands []map[string]any }
		}
		Observation struct {
			Outcome struct {
				Dispatch []struct {
					Response map[string]any
					Events   []map[string]any
				}
			}
		}
	}
	data, err := os.ReadFile("../../parity/oracle/fixtures/rpc-control.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	p := startRPC94(t, buildPigBinary(t))
	for i, command := range fixture.Case.Input.Commands {
		command["id"] = fixture.Observation.Outcome.Dispatch[i].Response["id"]
		data, _ := json.Marshal(command)
		if _, err = p.input.Write(append(data, '\n')); err != nil {
			t.Fatal(err)
		}
		events := []map[string]any{}
		var got map[string]any
		for {
			record := p.record(t)
			if record["type"] == "queue_update" {
				events = append(events, record)
			}
			if record["type"] == "response" {
				got = record
				break
			}
		}
		if got["success"] == true {
			if command["type"] == "get_state" {
				state := got["data"].(map[string]any)
				projected := map[string]any{}
				for _, key := range []string{"steeringMode", "followUpMode", "pendingMessageCount", "messageCount", "isStreaming"} {
					projected[key] = state[key]
				}
				got["data"] = projected
			}
			if command["type"] == "get_session_stats" {
				stats := got["data"].(map[string]any)
				delete(stats, "sessionId")
				delete(stats, "sessionFile")
				delete(stats, "contextUsage")
			}
		}
		want := fixture.Observation.Outcome.Dispatch[i]
		if !reflect.DeepEqual(got, want.Response) || !reflect.DeepEqual(events, want.Events) {
			t.Fatalf("%s: got response=%#v events=%#v; want response=%#v events=%#v", command["type"], got, events, want.Response, want.Events)
		}
	}
}

func rpcClient95(t *testing.T, url, settings string) (*codingagent.RPCClient, context.Context) {
	t.Helper()
	binary, cwd, dir := buildPigBinary(t), t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(settings), 0600); err != nil {
		t.Fatal(err)
	}
	client := codingagent.NewRPCClient(codingagent.RPCClientOptions{CLIPath: &binary, CWD: &cwd, Args: []string{"--provider", "deepseek", "--model", "deepseek-v4-flash", "--thinking", "off", "--api-key", "rpc-secret-95", "--no-tools", "--no-context-files", "--offline"}, Env: map[string]string{"PIG_CODING_AGENT_DIR": dir, "PIG_DEEPSEEK_BASE_URL": url}})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	t.Cleanup(cancel)
	if err := client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Stop(context.Background()) })
	return client, ctx
}

func TestRPC95ClientRunningControl(t *testing.T) {
	type request struct {
		Model     string
		Reasoning string `json:"reasoning_effort"`
		Messages  json.RawMessage
	}
	requests := make(chan request, 20)
	release := make(chan struct{}, 20)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body request
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		select {
		case requests <- body:
		case <-r.Context().Done():
			return
		}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"rpc answer\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":2,\"total_tokens\":12}}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	c, ctx := rpcClient95(t, server.URL, `{"compaction":{"enabled":false},"retry":{"enabled":false}}`)
	defer c.Stop(context.Background())
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	next := func() request {
		t.Helper()
		select {
		case r := <-requests:
			return r
		case <-ctx.Done():
			t.Fatal("missing provider request")
			return request{}
		}
	}
	must(c.SetSteeringMode(ctx, agent.QueueAll))
	must(c.SetFollowUpMode(ctx, agent.QueueOneAtATime))
	if err := c.Steer(ctx, "idle"); err == nil {
		t.Fatal("ADR-0018 requires idle queue rejection")
	}
	must(c.Prompt(ctx, "start"))
	first := next()
	if first.Model != "deepseek-v4-flash" {
		t.Fatal(first)
	}
	must(c.Steer(ctx, "steering"))
	must(c.FollowUp(ctx, "follow-up"))
	state, err := c.GetState(ctx)
	must(err)
	pending := state.PendingMessageCount
	if !state.IsStreaming || state.PendingMessageCount != 2 || state.SteeringMode != agent.QueueAll {
		t.Fatalf("active state: %+v", state)
	}
	if err = c.SetThinkingLevel(ctx, "high"); err == nil || !strings.Contains(err.Error(), "busy") {
		t.Fatalf("busy thinking: %v", err)
	}
	if _, err = c.SetModel(ctx, "deepseek", "deepseek-v4-pro"); err == nil || !strings.Contains(err.Error(), "busy") {
		t.Fatalf("busy model: %v", err)
	}
	release <- struct{}{}
	if r := next(); !strings.Contains(string(r.Messages), "steering") || strings.Contains(string(r.Messages), "follow-up") {
		t.Fatalf("steering request: %s", r.Messages)
	}
	release <- struct{}{}
	if r := next(); !strings.Contains(string(r.Messages), "follow-up") {
		t.Fatalf("follow-up request: %s", r.Messages)
	}
	release <- struct{}{}
	must(c.WaitForIdle(ctx))
	models, err := c.GetAvailableModels(ctx)
	must(err)
	if len(models) < 2 {
		t.Fatalf("available models: %+v", models)
	}
	model, err := c.SetModel(ctx, "deepseek", "deepseek-v4-pro")
	must(err)
	if model.ID != "deepseek-v4-pro" || model.ContextWindow == 0 {
		t.Fatalf("set model: %+v", model)
	}
	levels, err := c.GetAvailableThinkingLevels(ctx)
	must(err)
	if len(levels) < 2 {
		t.Fatal(levels)
	}
	must(c.SetThinkingLevel(ctx, "high"))
	level, err := c.CycleThinkingLevel(ctx)
	must(err)
	if level == "" || level == "high" {
		t.Fatal(level)
	}
	must(c.SetThinkingLevel(ctx, "max"))
	must(c.Prompt(ctx, "configured"))
	configured := next()
	if configured.Model != "deepseek-v4-pro" || configured.Reasoning != "max" {
		t.Fatalf("configured request: %+v", configured)
	}
	release <- struct{}{}
	must(c.WaitForIdle(ctx))
	cycle, err := c.CycleModel(ctx)
	must(err)
	if cycle.Model.ID == "" || cycle.Model.ID == "deepseek-v4-pro" || cycle.IsScoped {
		t.Fatalf("cycle: %+v", cycle)
	}
	stats, err := c.GetSessionStats(ctx)
	must(err)
	if stats.UserMessages != 4 || stats.AssistantMessages != 4 || stats.TotalMessages != 8 || stats.Tokens.Input != 40 || stats.Tokens.Output != 8 {
		t.Fatalf("stats: %+v", stats)
	}
	state, err = c.GetState(ctx)
	must(err)
	if state.SessionFile == nil || state.PendingMessageCount != 0 {
		t.Fatalf("state: %+v", state)
	}
	saved, err := codingagent.OpenSessionManager(*state.SessionFile, nil, nil)
	must(err)
	restored := saved.BuildSessionContext()
	if len(restored.Messages) != 8 || restored.Model == nil || restored.Model.ModelID != cycle.Model.ID || restored.ThinkingLevel != string(state.ThinkingLevel) {
		t.Fatalf("persisted context: %+v", restored)
	}
	last, err := c.GetLastAssistantText(ctx)
	must(err)
	if last == nil || *last != "rpc answer" {
		t.Fatal(last)
	}

	var fixture struct {
		Observation struct {
			Outcome struct{ Lifecycle map[string]any }
		}
	}
	fixtureData, err := os.ReadFile("../../parity/oracle/fixtures/rpc-control.json")
	must(err)
	must(json.Unmarshal(fixtureData, &fixture))
	users := []string{}
	for _, message := range restored.Messages {
		if message.MessageRole() == "user" {
			data, _ := json.Marshal(message)
			var user struct{ Content []struct{ Text string } }
			must(json.Unmarshal(data, &user))
			text := ""
			for _, block := range user.Content {
				text += block.Text
			}
			users = append(users, text)
		}
	}
	observation := map[string]any{"pending": pending, "users": users, "text": *last, "userMessages": stats.UserMessages, "assistantMessages": stats.AssistantMessages, "totalMessages": stats.TotalMessages, "tokens": map[string]any{"input": stats.Tokens.Input, "output": stats.Tokens.Output, "cacheRead": stats.Tokens.CacheRead, "cacheWrite": stats.Tokens.CacheWrite, "total": stats.Tokens.TotalTokens}}
	actual, _ := json.Marshal(observation)
	must(json.Unmarshal(actual, &observation))
	if !reflect.DeepEqual(observation, fixture.Observation.Outcome.Lifecycle) {
		t.Fatalf("client lifecycle parity: got %#v want %#v", observation, fixture.Observation.Outcome.Lifecycle)
	}
	encoded, _ := json.Marshal([]any{models, model, cycle, stats, state, restored})
	if strings.Contains(string(encoded), "rpc-secret-95") {
		t.Fatal("provider credential leaked")
	}
	must(c.Stop(ctx))
}

func TestRPC95ClientBashAndCancellation(t *testing.T) {
	requests := make(chan string, 8)
	release := make(chan struct{}, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		requests <- string(data)
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	c, ctx := rpcClient95(t, server.URL, `{"compaction":{"enabled":false},"retry":{"enabled":false}}`)
	defer c.Stop(context.Background())
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	chunks := make(chan *codingagent.AgentSessionBashExecutionUpdateEvent, 100)
	unsub, err := c.OnEvent(func(event codingagent.JSONAgentSessionEvent) {
		if chunk, ok := event.(*codingagent.AgentSessionBashExecutionUpdateEvent); ok {
			chunks <- chunk
		}
	})
	must(err)
	defer unsub()
	result, err := c.Bash(ctx, "printf before-prompt")
	must(err)
	if result.Output != "before-prompt" || result.ExitCode == nil || *result.ExitCode != 0 || result.Cancelled {
		t.Fatalf("bash: %+v", result)
	}
	must(c.Prompt(ctx, "first"))
	select {
	case body := <-requests:
		if !strings.Contains(body, "before-prompt") {
			t.Fatal(body)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	result, err = c.Bash(ctx, "printf while-streaming")
	must(err)
	messages, err := c.GetMessages(ctx)
	must(err)
	raw, _ := json.Marshal(messages)
	if strings.Contains(string(raw), "while-streaming") {
		t.Fatal("active Bash result prematurely entered current context")
	}
	release <- struct{}{}
	must(c.WaitForIdle(ctx))
	must(c.Prompt(ctx, "continue"))
	select {
	case body := <-requests:
		if !strings.Contains(body, "while-streaming") {
			t.Fatal(body)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	release <- struct{}{}
	must(c.WaitForIdle(ctx))
	longDone := make(chan error, 1)
	go func() {
		r, e := c.Bash(ctx, "printf cancel-ready; sleep 30")
		if e == nil && !r.Cancelled {
			e = errors.New("remote bash not cancelled")
		}
		longDone <- e
	}()
	for {
		select {
		case chunk := <-chunks:
			if strings.Contains(chunk.Delta, "cancel-ready") {
				if chunk.ID == nil || *chunk.ID == "" {
					t.Fatal("Bash update lacks request ID")
				}
				goto ready
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
ready:
	var group sync.WaitGroup
	for i := 0; i < 12; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			state, e := c.GetState(ctx)
			if e != nil || state.SessionID == "" {
				t.Errorf("concurrent state: %+v %v", state, e)
			}
			stats, e := c.GetSessionStats(ctx)
			if e != nil || stats.UserMessages != 2 {
				t.Errorf("concurrent stats: %+v %v", stats, e)
			}
		}()
	}
	group.Wait()
	must(c.AbortBash(ctx))
	must(<-longDone)
	stats, err := c.GetSessionStats(ctx)
	must(err)
	if stats.TotalMessages != 7 {
		t.Fatalf("Bash persistence: %+v", stats)
	}
}

func TestRPC95ClientRetryCancel(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"},\"finish_reason\":null}]}\n\n")
	}))
	defer server.Close()
	c, ctx := rpcClient95(t, server.URL, `{"compaction":{"enabled":false},"retry":{"enabled":false,"maxRetries":2,"baseDelayMs":30000,"provider":{"maxRetries":0}}}`)
	defer c.Stop(context.Background())
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	events := make(chan codingagent.JSONAgentSessionEvent, 100)
	unsub, err := c.OnEvent(func(e codingagent.JSONAgentSessionEvent) { events <- e })
	must(err)
	defer unsub()
	must(c.SetAutoRetry(ctx, true))
	must(c.Prompt(ctx, "retry"))
	for {
		select {
		case event := <-events:
			if event.AgentSessionEventType() == codingagent.AgentSessionEventTypeAutoRetryStart {
				goto retry
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
retry:
	stats, err := c.GetSessionStats(ctx)
	must(err)
	if stats.AssistantMessages != 1 {
		t.Fatalf("failed assistant was not persisted: %+v", stats)
	}
	must(c.AbortRetry(ctx))
	must(c.WaitForIdle(ctx))
	for {
		select {
		case event := <-events:
			if ended, ok := event.(*codingagent.AgentSessionAutoRetryEndEvent); ok {
				if ended.Success || (ended.FinalError == nil || *ended.FinalError != "Retry cancelled") {
					t.Fatalf("retry end: %+v", ended)
				}
				goto ended
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
ended:
	if calls.Load() != 1 {
		t.Fatalf("cancelled retry issued %d requests", calls.Load())
	}
	must(c.SetAutoRetry(ctx, false))
	result, err := c.PromptAndWait(ctx, "disabled")
	must(err)
	for _, e := range result {
		if e.AgentSessionEventType() == codingagent.AgentSessionEventTypeAutoRetryStart {
			t.Fatal("disabled retry started")
		}
	}
	if calls.Load() != 2 {
		t.Fatal(calls.Load())
	}
	stats, err = c.GetSessionStats(ctx)
	must(err)
	if stats.AssistantMessages != 2 || stats.UserMessages != 2 {
		t.Fatalf("retry stats: %+v", stats)
	}
}

func TestRPC95WireIDsAndBoundaries(t *testing.T) {
	p := startRPC94(t, buildPigBinary(t))
	if _, err := io.WriteString(p.input, "{\"id\":{\"bash\":95},\"type\":\"bash\",\"command\":\"printf wire-id\",\"excludeFromContext\":true}\n"); err != nil {
		t.Fatal(err)
	}
	sawUpdate := false
	for {
		record := p.record(t)
		if record["type"] == "bash_execution_update" || record["type"] == "response" {
			if !reflect.DeepEqual(record["id"], map[string]any{"bash": float64(95)}) {
				t.Fatalf("wire ID lost: %#v", record)
			}
		}
		if record["type"] == "bash_execution_update" {
			sawUpdate = true
		}
		if record["type"] == "response" {
			if record["success"] != true || !sawUpdate {
				t.Fatal(record)
			}
			break
		}
	}
	for _, command := range []map[string]any{
		{"type": "set_active_tools", "tools": []string{"read"}},
		{"type": "get_commands"}, {"type": "export_html"},
		{"type": "steer", "message": "image", "images": []any{map[string]any{"type": "image", "data": "AA==", "mimeType": "image/png"}}},
		{"type": "set_thinking_level", "level": "invalid"}, {"type": "set_follow_up_mode", "mode": "invalid"},
	} {
		command["id"] = 95
		data, _ := json.Marshal(command)
		if _, err := p.input.Write(append(data, '\n')); err != nil {
			t.Fatal(err)
		}
		record := p.record(t)
		if record["success"] != false || record["error"] == nil || record["id"] != float64(95) {
			t.Fatal(record)
		}
	}
	if _, err := io.WriteString(p.input, "{\"id\":96,\"type\":\"get_messages\"}\n"); err != nil {
		t.Fatal(err)
	}
	record := p.record(t)
	messages := record["data"].(map[string]any)["messages"].([]any)
	if len(messages) != 1 || messages[0].(map[string]any)["excludeFromContext"] != true {
		t.Fatalf("excluded Bash transcript: %#v", messages)
	}
}

func TestRPC95PipelinedConfiguration(t *testing.T) {
	p := startRPC94(t, buildPigBinary(t))
	for i := 0; i < 200; i++ {
		io.WriteString(p.input, "{\"type\":\"set_steering_mode\",\"mode\":\"one-at-a-time\"}\n")
		if record := p.record(t); record["success"] != true {
			t.Fatal(record)
		}
		io.WriteString(p.input, "{\"id\":\"set\",\"type\":\"set_steering_mode\",\"mode\":\"all\"}\n{\"id\":\"get\",\"type\":\"get_state\"}\n")
		for j := 0; j < 2; j++ {
			record := p.record(t)
			if record["id"] == "get" && record["data"].(map[string]any)["steeringMode"] != "all" {
				t.Fatalf("iteration %d observed stale configuration: %+v", i, record)
			}
		}
	}
}

func TestRPC95SDKObserverChild(t *testing.T) {
	if os.Getenv("PIG_RPC_OBSERVER_CHILD") != "1" {
		return
	}
	dir, key := os.Getenv("PIG_CODING_AGENT_DIR"), "fixture"
	runtime, err := codingagent.CreateHeadlessSession(context.Background(), codingagent.CreateHeadlessSessionOptions{CWD: dir, AgentDir: dir, Provider: "deepseek", Model: "deepseek-v4-flash", APIKey: &key, Offline: true, NoContextFiles: true, NoTools: codingagent.NoToolsAll})
	if err != nil {
		t.Fatal(err)
	}
	_, err = runtime.Session().Subscribe(func(event codingagent.AgentSessionEvent) {
		if event.AgentSessionEventType() == codingagent.AgentSessionEventTypeBashExecutionUpdate {
			json.NewEncoder(os.Stderr).Encode(event)
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = codingagent.RunRPCMode(context.Background(), runtime); err != nil {
		t.Fatal(err)
	}
	os.Exit(0)
}

func TestRPC95BashSDKObserver(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, "-test.run=^TestRPC95SDKObserverChild$")
	cmd.Env = append(os.Environ(), "PIG_RPC_OBSERVER_CHILD=1", "PIG_CODING_AGENT_DIR="+t.TempDir())
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { input.Close(); cmd.Process.Kill(); cmd.Wait() }()
	reader := bufio.NewReader(output)
	for _, id := range []string{`"typed"`, `{"raw":95}`, `null`} {
		io.WriteString(input, `{"id":`+id+`,"type":"bash","command":"printf observed"}`+"\n")
		updates := 0
		for {
			line, e := reader.ReadBytes('\n')
			if e != nil {
				t.Fatal(e)
			}
			var record map[string]any
			if e = json.Unmarshal(line, &record); e != nil {
				t.Fatal(e)
			}
			if record["type"] == "bash_execution_update" {
				updates++
			}
			if record["type"] == "response" {
				if record["success"] != true || updates != 1 {
					t.Fatal(record, updates)
				}
				break
			}
		}
	}
	input.Close()
	if err = cmd.Wait(); err != nil {
		t.Fatal(err, stderr.String())
	}
	lines := strings.Split(strings.TrimSpace(stderr.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("SDK observers: %s", stderr.String())
	}
	for i, line := range lines {
		var event codingagent.AgentSessionBashExecutionUpdateEvent
		if err = json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if event.Delta != "observed" || (i == 0 && (event.ID == nil || *event.ID != "typed")) || (i > 0 && event.ID != nil) {
			t.Fatalf("SDK event: %+v", event)
		}
	}
}

func TestRPC95PipelinedQueues(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	defer close(release)
	p := startRPC94(t, buildPigBinary(t), "PIG_DEEPSEEK_BASE_URL="+server.URL)
	defer p.cmd.Process.Kill()
	io.WriteString(p.input, "{\"id\":\"start\",\"type\":\"prompt\",\"message\":\"hold\"}\n")
	for {
		record := p.record(t)
		if record["id"] == "start" {
			if record["success"] != true {
				t.Fatal(record)
			}
			break
		}
	}
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("provider not started")
	}
	var batch strings.Builder
	for i := 0; i < 20; i++ {
		command := "steer"
		if i >= 10 {
			command = "follow_up"
		}
		data, _ := json.Marshal(map[string]any{"id": i, "type": command, "message": strconv.Itoa(i)})
		batch.Write(data)
		batch.WriteByte('\n')
	}
	batch.WriteString("{\"id\":\"state\",\"type\":\"get_state\"}\n")
	io.WriteString(p.input, batch.String())
	updates := 0
	for {
		record := p.record(t)
		if record["type"] == "queue_update" {
			updates++
			steering := record["steering"].([]any)
			follow := record["followUp"].([]any)
			if len(steering)+len(follow) != updates {
				t.Fatal(record)
			}
			for i, text := range append(steering, follow...) {
				if text != strconv.Itoa(i) {
					t.Fatalf("queue FIFO: %+v", record)
				}
			}
		}
		if record["id"] == "state" {
			if record["data"].(map[string]any)["pendingMessageCount"] != float64(20) || updates != 20 {
				t.Fatal(record, updates)
			}
			break
		}
	}
}
