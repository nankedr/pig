package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/nankedr/pig/codingagent"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

type rpcProcess94 struct {
	cmd    *exec.Cmd
	input  io.WriteCloser
	lines  chan []byte
	stderr bytes.Buffer
}

func startRPC94(t *testing.T, binary string, env ...string) *rpcProcess94 {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	p := &rpcProcess94{cmd: exec.CommandContext(ctx, binary, "--mode", "rpc", "--provider", "deepseek", "--model", "deepseek-v4-flash", "--api-key", "fixture", "--no-session", "--no-tools", "--no-context-files", "--offline"), lines: make(chan []byte, 1024)}
	p.cmd.Dir = t.TempDir()
	p.cmd.Env = append(os.Environ(), "PIG_CODING_AGENT_DIR="+t.TempDir())
	p.cmd.Env = append(p.cmd.Env, env...)
	p.cmd.Stderr = &p.stderr
	var err error
	p.input, err = p.cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := p.cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = p.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	go func() {
		defer close(p.lines)
		r := bufio.NewReader(output)
		for {
			line, err := r.ReadBytes('\n')
			if len(line) > 0 {
				p.lines <- line
			}
			if err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() { _ = p.input.Close(); _ = p.cmd.Process.Kill(); _ = p.cmd.Wait() })
	return p
}
func (p *rpcProcess94) record(t *testing.T) map[string]any {
	t.Helper()
	select {
	case line, ok := <-p.lines:
		if !ok {
			t.Fatal("RPC stdout closed before response")
		}
		if !bytes.HasSuffix(line, []byte{'\n'}) {
			t.Fatalf("record without LF: %q", line)
		}
		var record map[string]any
		if err := json.Unmarshal(line, &record); err != nil {
			t.Fatalf("invalid JSONL: %q: %v", line, err)
		}
		return record
	case <-time.After(10 * time.Second):
		t.Fatal("RPC response timeout")
		return nil
	}
}
func TestRPC94ParityDispatch(t *testing.T) {
	var fixture struct {
		Case struct {
			Input struct{ Scenarios []struct{ Name, Line string } }
		}
		Observation struct {
			Outcome struct {
				Dispatch []struct {
					Name     string
					Response map[string]any
				}
			}
		}
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "parity", "oracle", "fixtures", "rpc.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	p := startRPC94(t, buildPigBinary(t))
	for i, scenario := range fixture.Case.Input.Scenarios {
		t.Run(scenario.Name, func(t *testing.T) {
			for _, b := range []byte(scenario.Line + "\r\n") {
				if _, err := p.input.Write([]byte{b}); err != nil {
					t.Fatal(err)
				}
			}
			got := p.record(t)
			if scenario.Name == "surrogate-id" {
				got["id"] = "<lone-high-surrogate>"
			}
			if got["command"] == "parse" {
				if !strings.HasPrefix(got["error"].(string), "Failed to parse command:") {
					t.Fatal(got)
				}
				got["error"] = "Failed to parse command: <syntax>"
			}
			if want := fixture.Observation.Outcome.Dispatch[i].Response; !reflect.DeepEqual(got, want) {
				t.Fatalf("got %#v; want %#v", got, want)
			}
		})
	}
}

func TestRPC94ClientLifecycle(t *testing.T) {
	binary := buildPigBinary(t)
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started <- struct{}{}
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"id\":\"rpc\",\"model\":\"deepseek-v4-flash\",\"choices\":[{\"delta\":{\"content\":\"你好😀\\u2028world\\u2029\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	cwd := t.TempDir()
	client := codingagent.NewRPCClient(codingagent.RPCClientOptions{CLIPath: &binary, CWD: &cwd, Args: []string{"--provider", "deepseek", "--model", "deepseek-v4-flash", "--api-key", "fixture", "--no-session", "--no-tools", "--no-context-files", "--offline"}, Env: map[string]string{"PIG_CODING_AGENT_DIR": t.TempDir(), "PIG_DEEPSEEK_BASE_URL": server.URL}})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer client.Stop(context.Background())
	seen := make(chan codingagent.AgentSessionEventType, 100)
	unsub, err := client.OnEvent(func(event codingagent.JSONAgentSessionEvent) { seen <- event.AgentSessionEventType() })
	if err != nil {
		t.Fatal(err)
	}
	defer unsub()
	if err = client.Prompt(ctx, "hello"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("provider never started")
	}
	state, err := client.GetState(ctx)
	if err != nil || !state.IsStreaming || state.SessionID == "" {
		t.Fatalf("active state: %+v %v", state, err)
	}
	close(release)
	if err = client.WaitForIdle(ctx); err != nil {
		t.Fatal(err)
	}
	messages, err := client.GetMessages(ctx)
	if err != nil || len(messages) != 2 {
		t.Fatalf("messages: %#v %v", messages, err)
	}
	last, err := client.GetLastAssistantText(ctx)
	if err != nil || last == nil || *last != "你好😀\u2028world" {
		t.Fatalf("last: %v %v", last, err)
	}

	events, err := client.PromptAndWait(ctx, "hello")
	if err != nil {
		t.Fatal(err)
	}
	var oracle struct {
		Observation struct {
			Outcome struct{ Lifecycle map[string]any }
		}
	}
	data, err := os.ReadFile(filepath.Join("..", "..", "parity/oracle/fixtures/rpc.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &oracle); err != nil {
		t.Fatal(err)
	}
	kinds := map[string]bool{}
	projected := true
	for _, event := range events {
		kind := string(event.AgentSessionEventType())
		kinds[kind] = true
		if kind == "message_update" {
			raw, _ := json.Marshal(event)
			var v map[string]any
			json.Unmarshal(raw, &v)
			_, hasMessage := v["message"]
			_, hasPartial := v["assistantMessageEvent"].(map[string]any)["partial"]
			projected = projected && !hasMessage && !hasPartial
		}
	}
	names := []string{}
	for name := range kinds {
		names = append(names, name)
	}
	sort.Strings(names)
	roles := []string{}
	for _, message := range messages {
		roles = append(roles, string(message.MessageRole()))
	}
	observed := map[string]any{"roles": roles, "text": *last, "eventTypes": names, "projected": projected}
	encoded, _ := json.Marshal(observed)
	json.Unmarshal(encoded, &observed)
	if !reflect.DeepEqual(observed, oracle.Observation.Outcome.Lifecycle) {
		t.Fatalf("lifecycle parity: %#v want %#v", observed, oracle.Observation.Outcome.Lifecycle)
	}
	if err = client.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = client.GetState(ctx); err == nil {
		t.Fatal("query after stop succeeded")
	}
}

func TestRPC94ClientProcessFailures(t *testing.T) {
	peer := filepath.Join(t.TempDir(), "peer")
	if runtime.GOOS == "windows" {
		peer += ".exe"
	}
	build := exec.Command("go", "build", "-o", peer, "testdata/rpc-peer.go")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("peer: %s %v", out, err)
	}
	newClient := func(mode string) *codingagent.RPCClient {
		return codingagent.NewRPCClient(codingagent.RPCClientOptions{CLIPath: &peer, Env: map[string]string{"RPC_PEER_MODE": mode}})
	}
	for _, mode := range []string{"startup-exit", "startup-hang"} {
		t.Run(mode, func(t *testing.T) {
			c := newClient(mode)
			ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
			defer cancel()
			if err := c.Start(ctx); err == nil {
				t.Fatal("startup failure accepted")
			}
			if err := c.Stop(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, mode := range []string{"exit", "eof", "stdin-closed", "backpressure"} {
		t.Run(mode, func(t *testing.T) {
			c := newClient(mode)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			if err := c.Start(ctx); err != nil {
				t.Fatal(err)
			}
			defer c.Stop(context.Background())
			collected := make(chan error, 1)
			go func() { _, err := c.CollectEvents(ctx); collected <- err }()
			requestCtx, requestCancel := context.WithTimeout(ctx, 150*time.Millisecond)
			defer requestCancel()
			text := "request"
			if mode == "backpressure" {
				text = strings.Repeat("x", 1<<20)
			}
			if err := c.Prompt(requestCtx, text); err == nil {
				t.Fatal("failed request succeeded")
			}
			if err := c.Stop(ctx); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-collected:
				if err == nil {
					t.Fatal("collector succeeded after exit")
				}
			case <-ctx.Done():
				t.Fatal("collector leaked")
			}
		})
	}
	t.Run("cancel-correlation", func(t *testing.T) {
		c := newClient("cancel")
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := c.Start(ctx); err != nil {
			t.Fatal(err)
		}
		defer c.Stop(context.Background())
		short, stop := context.WithTimeout(ctx, 30*time.Millisecond)
		defer stop()
		if err := c.Prompt(short, "slow"); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		for i := 0; i < 25; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				message := fmt.Sprint(i)
				if err := c.Prompt(ctx, message); err == nil || err.Error() != message {
					t.Errorf("correlation %s: %v", message, err)
				}
			}(i)
		}
		wg.Wait()
		time.Sleep(250 * time.Millisecond)
		if err := c.Prompt(ctx, "after-late"); err == nil || err.Error() != "after-late" {
			t.Fatal(err)
		}
	})
	t.Run("reader-framing", func(t *testing.T) {
		c := newClient("framing")
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := c.Start(ctx); err != nil {
			t.Fatal(err)
		}
		defer c.Stop(context.Background())
		events, err := c.PromptAndWait(ctx, "test")
		if err != nil || len(events) != 2 {
			t.Fatalf("events=%#v err=%v", events, err)
		}
		event, ok := events[0].(*codingagent.AgentSessionQueueUpdateEvent)
		if !ok || len(event.Steering) != 1 || event.Steering[0] != "你好😀\u2028\u2029" {
			t.Fatalf("UTF-8: %#v", events[0])
		}
	})
}

func TestRPC94AbortAndConcurrentEvents(t *testing.T) {
	binary := buildPigBinary(t)
	started := make(chan struct{}, 1)
	cancelled := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"id\":\"rpc-abort\",\"model\":\"deepseek-v4-flash\",\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n")
		w.(http.Flusher).Flush()
		started <- struct{}{}
		<-r.Context().Done()
		cancelled <- struct{}{}
	}))
	defer server.Close()
	cwd := t.TempDir()
	client := codingagent.NewRPCClient(codingagent.RPCClientOptions{CLIPath: &binary, CWD: &cwd, Args: []string{"--provider", "deepseek", "--model", "deepseek-v4-flash", "--api-key", "fixture", "--no-session", "--no-tools", "--offline"}, Env: map[string]string{"PIG_CODING_AGENT_DIR": t.TempDir(), "PIG_DEEPSEEK_BASE_URL": server.URL}})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer client.Stop(context.Background())
	callback := make(chan error, 1)
	unsubscribe, err := client.OnEvent(func(event codingagent.JSONAgentSessionEvent) {
		if event.AgentSessionEventType() == codingagent.AgentSessionEventTypeAgentStart {
			_, err := client.GetState(ctx)
			callback <- err
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribe()
	outcome := make(chan []codingagent.JSONAgentSessionEvent, 1)
	failure := make(chan error, 1)
	go func() { events, err := client.PromptAndWait(ctx, "block"); outcome <- events; failure <- err }()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("run did not start")
	}
	select {
	case err := <-callback:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("callback reentrancy deadlock")
	}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := client.GetMessages(ctx); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if err := client.Abort(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-cancelled:
	case <-ctx.Done():
		t.Fatal("abort leaked provider request")
	}
	if err := <-failure; err != nil {
		t.Fatal(err)
	}
	events := <-outcome
	if len(events) == 0 || events[len(events)-1].AgentSessionEventType() != codingagent.AgentSessionEventTypeAgentSettled {
		t.Fatal("missing settled")
	}
	messages, err := client.GetMessages(ctx)
	if err != nil || len(messages) != 2 {
		t.Fatalf("messages: %v %v", messages, err)
	}
	raw, _ := json.Marshal(messages[1])
	if !bytes.Contains(raw, []byte(`"stopReason":"aborted"`)) || !bytes.Contains(raw, []byte("partial")) {
		t.Fatalf("lost aborted outcome: %s", raw)
	}
}

func TestRPC94EOFAndOutputFailure(t *testing.T) {
	binary := buildPigBinary(t)
	t.Run("final-line", func(t *testing.T) {
		p := startRPC94(t, binary)
		io.WriteString(p.input, `{"id":"tail","type":"get_messages"}`)
		p.input.Close()
		if record := p.record(t); record["id"] != "tail" || record["success"] != true {
			t.Fatal(record)
		}
	})
	t.Run("output-failure", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		path := filepath.Join(t.TempDir(), "readonly")
		if err := os.WriteFile(path, nil, 0600); err != nil {
			t.Fatal(err)
		}
		output, err := os.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer output.Close()
		cmd := exec.CommandContext(ctx, binary, "--mode", "rpc", "--provider", "deepseek", "--model", "deepseek-v4-flash", "--api-key", "fixture", "--no-session", "--no-tools", "--offline")
		cmd.Env = append(os.Environ(), "PIG_CODING_AGENT_DIR="+t.TempDir())
		cmd.Dir = t.TempDir()
		cmd.Stdout = output
		input, err := cmd.StdinPipe()
		if err != nil {
			t.Fatal(err)
		}
		defer input.Close()
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		io.WriteString(input, "{\"type\":\"get_state\"}\n")
		if err := cmd.Wait(); err == nil || ctx.Err() != nil || !strings.Contains(stderr.String(), "Failed to write stdout") {
			t.Fatalf("output error: %v %s", err, stderr.String())
		}
	})
}

func TestRPC94EOFFlushFailure(t *testing.T) {
	binary := buildPigBinary(t)
	for _, mode := range []string{"readonly", "large-id", "parse-errors"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, binary, "--mode", "rpc", "--provider", "deepseek", "--model", "deepseek-v4-flash", "--api-key", "fixture", "--no-session", "--no-tools", "--offline")
			cmd.Dir = t.TempDir()
			cmd.Env = append(os.Environ(), "PIG_CODING_AGENT_DIR="+t.TempDir())
			var stderr bytes.Buffer
			cmd.Stderr = &stderr
			input := `{"type":"get_messages"}`
			if mode != "readonly" {
				reader, writer, err := os.Pipe()
				if err != nil {
					t.Fatal(err)
				}
				defer reader.Close()
				defer writer.Close()
				cmd.Stdout = writer
				input = `{"type":"get_messages","id":"` + strings.Repeat("x", 1<<20) + `"}`
				if mode == "parse-errors" {
					input = strings.Repeat("\n", 1000)
				}
			} else {
				path := filepath.Join(t.TempDir(), "readonly")
				if err := os.WriteFile(path, nil, 0600); err != nil {
					t.Fatal(err)
				}
				output, err := os.Open(path)
				if err != nil {
					t.Fatal(err)
				}
				defer output.Close()
				cmd.Stdout = output
			}
			cmd.Stdin = strings.NewReader(input)
			if err := cmd.Run(); err == nil || ctx.Err() != nil || !strings.Contains(stderr.String(), "Failed to write stdout") {
				t.Fatalf("EOF failure: %v %s", err, stderr.String())
			}
		})
	}
}

func TestRPC94PreservesRawID(t *testing.T) {
	p := startRPC94(t, buildPigBinary(t))
	io.WriteString(p.input, "{\"id\":\"\\ud800\",\"type\":\"foobar\"}\n")
	select {
	case line := <-p.lines:
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(line, &fields); err != nil {
			t.Fatal(err)
		}
		if string(fields["id"]) != `"\ud800"` {
			t.Fatalf("id rewritten: %s", fields["id"])
		}
	case <-time.After(5 * time.Second):
		t.Fatal("raw ID timeout")
	}
}

func TestRPC94CancelledStartCleansSubscriptions(t *testing.T) {
	peer := filepath.Join(t.TempDir(), "peer")
	if runtime.GOOS == "windows" {
		peer += ".exe"
	}
	if out, err := exec.Command("go", "build", "-o", peer, "testdata/rpc-peer.go").CombinedOutput(); err != nil {
		t.Fatalf("peer: %s %v", out, err)
	}
	client := codingagent.NewRPCClient(codingagent.RPCClientOptions{CLIPath: &peer, Env: map[string]string{"RPC_PEER_MODE": "framing"}})
	stale := make(chan struct{}, 10)
	unsubscribe, err := client.OnEvent(func(codingagent.JSONAgentSessionEvent) { stale <- struct{}{} })
	if err != nil {
		t.Fatal(err)
	}
	defer unsubscribe()
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := client.Start(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	ctx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	if err := client.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer client.Stop(context.Background())
	if _, err := client.PromptAndWait(ctx, "events"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-stale:
		t.Fatal("failed-start subscription survived restart")
	case <-time.After(50 * time.Millisecond):
	}
}
