package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func TestResponsesRPCQueuesAbortAndConcurrentRequests(t *testing.T) {
	started, release := make(chan struct{}, 2), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		input := body["input"].([]any)
		last, _ := json.Marshal(input[len(input)-1])
		w.Header().Set("Content-Type", "text/event-stream")
		if strings.Contains(string(last), "block") {
			fmt.Fprint(w, frame133(map[string]any{"type": "response.output_text.delta", "delta": "PARTIAL"}))
			w.(http.Flusher).Flush()
			started <- struct{}{}
			select {
			case <-release:
			case <-r.Context().Done():
				return
			}
		}
		fmt.Fprint(w, frame133(map[string]any{"type": "response.output_text.delta", "delta": "OK"})+frame133(map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed"}}))
	}))
	defer server.Close()
	root, dir := taskRoot133(t)
	binary := binary133(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client := codingagent.NewRPCClient(codingagent.RPCClientOptions{CLIPath: &binary, CWD: &root, Args: []string{"--no-tools", "--no-context-files"}, Env: map[string]string{"HOME": root, "PIG_CODING_AGENT_DIR": dir, "PIG_DEEPSEEK_BASE_URL": server.URL}})
	if err := client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer client.Stop(context.Background())
	if err := client.Prompt(ctx, "block"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("request not started")
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			state, err := client.GetState(ctx)
			if err != nil || !state.IsStreaming {
				t.Errorf("concurrent state: %+v %v", state, err)
			}
		}()
	}
	wg.Wait()
	if _, err := client.SetModel(ctx, "deepseek", "deepseek-v4-flash"); err == nil {
		t.Fatal("busy model switch succeeded")
	}
	if err := client.Steer(ctx, "steer"); err != nil {
		t.Fatal(err)
	}
	if err := client.FollowUp(ctx, "follow"); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := client.WaitForIdle(ctx); err != nil {
		t.Fatal(err)
	}
	messages, err := client.GetMessages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(messages)
	if !strings.Contains(string(data), "steer") || !strings.Contains(string(data), "follow") {
		t.Fatalf("queue history: %s", data)
	}
	old, err := client.GetState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled, err := client.NewSession(ctx); err != nil || cancelled {
		t.Fatalf("replacement: %v %v", cancelled, err)
	}
	if _, err := client.PromptAndWait(ctx, "fresh"); err != nil {
		t.Fatal(err)
	}
	fresh, err := client.GetState(ctx)
	if err != nil || fresh.SessionID == old.SessionID || fresh.MessageCount != 2 {
		t.Fatalf("replacement state: %+v %v", fresh, err)
	}
	blocked := make(chan struct{}, 1)
	abortServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, frame133(map[string]any{"type": "response.output_text.delta", "delta": "ABORT_PARTIAL"}))
		w.(http.Flusher).Flush()
		blocked <- struct{}{}
		<-r.Context().Done()
	}))
	defer abortServer.Close()
	if err := client.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	client = codingagent.NewRPCClient(codingagent.RPCClientOptions{CLIPath: &binary, CWD: &root, Args: []string{"--no-tools", "--no-session", "--no-context-files"}, Env: map[string]string{"HOME": root, "PIG_CODING_AGENT_DIR": dir, "PIG_DEEPSEEK_BASE_URL": abortServer.URL}})
	if err := client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer client.Stop(context.Background())
	partials := make(chan struct{}, 2)
	unsubscribe, err := client.OnEvent(func(event codingagent.JSONAgentSessionEvent) {
		if update, ok := event.(*codingagent.JSONAgentSessionMessageUpdateEvent); ok && strings.Contains(string(update.AssistantMessageEvent), "text_delta") {
			partials <- struct{}{}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribe()
	if err := client.Prompt(ctx, "abort"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-blocked:
	case <-ctx.Done():
		t.Fatal("abort request not started")
	}
	select {
	case <-partials:
	case <-ctx.Done():
		t.Fatal("partial not delivered")
	}
	if err := client.Abort(ctx); err != nil {
		t.Fatal(err)
	}
	if err := client.WaitForIdle(ctx); err != nil {
		t.Fatal(err)
	}
	messages, err = client.GetMessages(ctx)
	if err != nil {
		t.Fatal(err)
	}
	last := messages[len(messages)-1].(ai.AssistantMessage)
	if last.StopReason != ai.StopReasonAborted {
		t.Fatalf("abort outcome: %+v", last)
	}
	if last.Content[0].(ai.TextContent).Text != "ABORT_PARTIAL" {
		t.Fatal("abort lost partial")
	}
	if err := client.Prompt(ctx, "replace active"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-blocked:
	case <-ctx.Done():
		t.Fatal("replacement request not started")
	}
	select {
	case <-partials:
	case <-ctx.Done():
		t.Fatal("replacement partial not delivered")
	}
	previous, err := client.GetState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled, err := client.NewSession(ctx); err != nil || cancelled {
		t.Fatalf("active replacement: %v %v", cancelled, err)
	}
	state, err := client.GetState(ctx)
	if err != nil || state.IsStreaming || state.MessageCount != 0 || state.SessionID == previous.SessionID {
		t.Fatalf("active replacement state: %+v %v", state, err)
	}
}
