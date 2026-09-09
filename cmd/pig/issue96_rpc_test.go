package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	"sync/atomic"
	"testing"
	"time"
)

func TestRPC96ParityLifecycle(t *testing.T) {
	var fixture struct {
		Case struct {
			Input struct{ Commands []map[string]any }
		}
		Observation struct {
			Outcome struct{ Dispatch []map[string]any }
		}
	}
	data, err := os.ReadFile("../../parity/oracle/fixtures/rpc-lifecycle.json")
	if err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	p := startRPC94(t, buildPigBinary(t))
	for i, command := range fixture.Case.Input.Commands {
		command["id"] = strconv.Itoa(i)
		data, _ := json.Marshal(command)
		if _, err = p.input.Write(append(data, '\n')); err != nil {
			t.Fatal(err)
		}
		var got map[string]any
		for {
			got = p.record(t)
			if got["type"] == "response" {
				break
			}
		}
		if command["type"] == "get_state" && got["success"] == true {
			state := got["data"].(map[string]any)
			projected := map[string]any{}
			for _, key := range []string{"messageCount", "isStreaming", "autoCompactionEnabled", "sessionName"} {
				if v, ok := state[key]; ok {
					projected[key] = v
				}
			}
			got["data"] = projected
		}
		if !reflect.DeepEqual(got, fixture.Observation.Outcome.Dispatch[i]) {
			t.Fatalf("%s: got %#v; want %#v", command["type"], got, fixture.Observation.Outcome.Dispatch[i])
		}
	}
}

func TestRPC96ClientLifecyclePersistence(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"lifecycle answer\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":2,\"total_tokens\":12}}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	c, ctx := rpcClient95(t, server.URL, `{"compaction":{"enabled":false,"keepRecentTokens":1},"retry":{"enabled":false}}`)
	defer c.Stop(context.Background())
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	_, err := c.PromptAndWait(ctx, "first")
	must(err)
	_, err = c.PromptAndWait(ctx, "second")
	must(err)
	must(c.SetSessionName(ctx, "  source  "))
	state, err := c.GetState(ctx)
	must(err)
	if state.SessionName == nil || *state.SessionName != "source" {
		t.Fatal(state)
	}
	source := *state.SessionFile
	entries, leaf, err := c.GetEntries(ctx)
	must(err)
	if leaf == nil || *leaf != entries[len(entries)-1].ID {
		t.Fatal("wrong leaf")
	}
	tail, tailLeaf, err := c.GetEntries(ctx, entries[0].ID)
	must(err)
	if !reflect.DeepEqual(tail, entries[1:]) || *tailLeaf != *leaf {
		t.Fatal("since cursor mismatch")
	}
	tree, treeLeaf, err := c.GetTree(ctx)
	must(err)
	if len(tree) != 1 || *treeLeaf != *leaf {
		t.Fatal("wrong tree")
	}
	var ids []string
	for len(tree) == 1 {
		ids = append(ids, tree[0].Entry.ID)
		tree = tree[0].Children
	}
	if len(ids) != len(entries) {
		t.Fatal("tree lost entries")
	}
	users, err := c.GetForkMessages(ctx)
	must(err)
	if len(users) != 2 || users[1].Text != "second" {
		t.Fatal(users)
	}
	before, err := os.ReadFile(source)
	must(err)
	text, cancelled, err := c.Fork(ctx, users[1].EntryID)
	must(err)
	if cancelled || text != "second" {
		t.Fatal(text, cancelled)
	}
	fork, err := c.GetState(ctx)
	must(err)
	if fork.SessionID == state.SessionID || *fork.SessionFile == source || fork.MessageCount != 2 {
		t.Fatal(fork)
	}
	_, err = c.PromptAndWait(ctx, "fork continued")
	must(err)
	after, err := os.ReadFile(source)
	must(err)
	if !bytes.Equal(before, after) {
		t.Fatal("fork wrote source")
	}
	cloneCancelled, err := c.Clone(ctx)
	must(err)
	if cloneCancelled {
		t.Fatal("clone cancelled")
	}
	clone, err := c.GetState(ctx)
	must(err)
	if clone.SessionID == fork.SessionID || clone.MessageCount != 4 {
		t.Fatal(clone)
	}
	_, err = c.NewSession(ctx, source)
	must(err)
	fresh, err := c.GetState(ctx)
	must(err)
	if fresh.MessageCount != 0 || fresh.SessionID == clone.SessionID {
		t.Fatal(fresh)
	}
	_, err = c.PromptAndWait(ctx, "fresh continued")
	must(err)
	manager, err := codingagent.OpenSessionManager(*fresh.SessionFile, nil, nil)
	must(err)
	parentPreserved := manager.GetHeader().ParentSession != nil && *manager.GetHeader().ParentSession == source
	if !parentPreserved {
		t.Fatal(manager.GetHeader())
	}
	_, err = c.SwitchSession(ctx, source)
	must(err)
	restored, err := c.GetState(ctx)
	must(err)
	if restored.SessionID != state.SessionID || restored.MessageCount != 4 {
		t.Fatal(restored)
	}
	result, err := c.Compact(ctx, "preserve important details")
	must(err)
	if result.Summary == "" || result.FirstKeptEntryID == "" || result.TokensBefore <= 0 {
		t.Fatal(result)
	}
	compacted, _, err := c.GetEntries(ctx)
	must(err)
	if len(compacted) != len(entries)+1 || !reflect.DeepEqual(compacted[:len(entries)], entries) {
		t.Fatal("compaction erased history")
	}
	must(c.SetAutoCompaction(ctx, true))
	auto, err := c.GetState(ctx)
	must(err)
	if !auto.AutoCompactionEnabled {
		t.Fatal("auto compaction setting lost")
	}
	must(c.SetAutoCompaction(ctx, false))
	_, err = c.PromptAndWait(ctx, "after summary")
	must(err)
	expected, err := c.GetMessages(ctx)
	must(err)
	must(c.Stop(context.Background()))
	must(c.Start(ctx))
	_, err = c.SwitchSession(ctx, source)
	must(err)
	reopened, err := c.GetMessages(ctx)
	must(err)
	if !reflect.DeepEqual(reopened, expected) {
		t.Fatal("cross-process RPC reopen changed messages")
	}
	manager, err = codingagent.OpenSessionManager(source, nil, nil)
	must(err)
	wantJSON, _ := json.Marshal(manager.BuildSessionContext().Messages)
	gotJSON, _ := json.Marshal(reopened)
	if !bytes.Equal(wantJSON, gotJSON) {
		t.Fatalf("RPC and v3 reader disagree: %s / %s", gotJSON, wantJSON)
	}
	_, err = c.PromptAndWait(ctx, "reopened continued")
	must(err)
	observation := map[string]any{
		"users": []string{users[0].Text, users[1].Text}, "name": *state.SessionName, "forkText": text, "forkMessages": fork.MessageCount, "forkIdentityChanged": fork.SessionID != state.SessionID, "sourceUnchanged": bytes.Equal(before, after), "cloneMessages": clone.MessageCount, "cloneIdentityChanged": clone.SessionID != fork.SessionID, "newMessages": fresh.MessageCount, "parentPreserved": parentPreserved, "restoredIdentity": restored.SessionID == state.SessionID, "restoredMessages": restored.MessageCount, "cursorMatches": reflect.DeepEqual(tail, entries[1:]) && *tailLeaf == *leaf, "treeLeafMatches": *treeLeaf == *leaf, "compactionRetainsHistory": reflect.DeepEqual(compacted[:len(entries)], entries), "summaryPresent": result.Summary != "", "autoEnabled": auto.AutoCompactionEnabled, "reopenMatches": reflect.DeepEqual(expected, reopened),
	}
	fixtureData, err := os.ReadFile("../../parity/oracle/fixtures/rpc-lifecycle.json")
	must(err)
	var fixture struct {
		Observation struct {
			Outcome struct{ Lifecycle map[string]any }
		}
	}
	must(json.Unmarshal(fixtureData, &fixture))
	projected, _ := json.Marshal(observation)
	var got map[string]any
	must(json.Unmarshal(projected, &got))
	if !reflect.DeepEqual(got, fixture.Observation.Outcome.Lifecycle) {
		t.Fatalf("Pi lifecycle mismatch: %#v / %#v", got, fixture.Observation.Outcome.Lifecycle)
	}
}

func TestRPC96FailedReplacementPreservesSession(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	c, ctx := rpcClient95(t, server.URL, `{"compaction":{"enabled":false}}`)
	defer c.Stop(context.Background())
	if _, err := c.PromptAndWait(ctx, "original"); err != nil {
		t.Fatal(err)
	}
	before, err := c.GetState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cwd, dir := t.TempDir(), t.TempDir()
	data := `{"type":"session","version":3,"id":"bad-target","timestamp":"2026-01-01T00:00:00.000Z","cwd":` + strconv.Quote(filepath.Join(cwd, "missing")) + "}\n"
	target := filepath.Join(dir, "bad.jsonl")
	if err = os.WriteFile(target, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = c.SwitchSession(ctx, target); err == nil {
		t.Fatal("missing cwd accepted")
	}
	if _, _, err = c.Fork(ctx, "missing"); err == nil {
		t.Fatal("missing fork accepted")
	}
	if _, _, err = c.GetEntries(ctx, "missing"); err == nil || !strings.Contains(err.Error(), "Entry not found") {
		t.Fatal(err)
	}
	after, err := c.GetState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if before.SessionID != after.SessionID || before.MessageCount != after.MessageCount {
		t.Fatal("failed operation replaced session")
	}
	if _, err = c.PromptAndWait(ctx, "still usable"); err != nil {
		t.Fatal(err)
	}
}

func TestRPC96ConcurrentCompactionReplacement(t *testing.T) {
	var hold atomic.Bool
	started := make(chan struct{}, 20)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		if hold.Load() {
			started <- struct{}{}
			<-r.Context().Done()
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":2,\"total_tokens\":12}}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	binary := filepath.Join(t.TempDir(), "pig")
	build := exec.Command("go", "build", "-race", "-o", binary, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	dir, cwd := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"compaction":{"enabled":false,"keepRecentTokens":1},"retry":{"enabled":false}}`), 0600); err != nil {
		t.Fatal(err)
	}
	c := codingagent.NewRPCClient(codingagent.RPCClientOptions{CLIPath: &binary, CWD: &cwd, Args: []string{"--provider", "deepseek", "--model", "deepseek-v4-flash", "--thinking", "off", "--api-key", "fixture", "--no-tools", "--no-context-files", "--offline"}, Env: map[string]string{"PIG_CODING_AGENT_DIR": dir, "PIG_DEEPSEEK_BASE_URL": server.URL}})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(c.Start(ctx))
	defer c.Stop(context.Background())
	wait := func() {
		t.Helper()
		select {
		case <-started:
		case <-ctx.Done():
			t.Fatal("missing held request")
		}
	}
	_, err := c.PromptAndWait(ctx, "first")
	must(err)
	_, err = c.PromptAndWait(ctx, "second")
	must(err)
	entries, _, err := c.GetEntries(ctx)
	must(err)
	hold.Store(true)
	compact := make(chan error, 1)
	go func() { _, err := c.Compact(ctx); compact <- err }()
	wait()
	state, err := c.GetState(ctx)
	must(err)
	if !state.IsCompacting {
		t.Fatal("compaction not visible")
	}
	if err = c.Prompt(ctx, "must be busy"); err == nil {
		t.Fatal("prompt admitted during manual compaction")
	}
	if _, err = c.Compact(ctx); err == nil {
		t.Fatal("second compaction admitted")
	}
	_, _, err = c.GetEntries(ctx)
	must(err)
	must(c.Abort(ctx))
	select {
	case err = <-compact:
		if err == nil {
			t.Fatal("aborted compaction succeeded")
		}
	case <-ctx.Done():
		t.Fatal("compact waiter stuck")
	}
	after, _, err := c.GetEntries(ctx)
	must(err)
	if !reflect.DeepEqual(entries, after) {
		t.Fatal("cancelled compaction changed history")
	}
	// Replacement cancels a pending summary and publishes the new binding once.
	go func() { _, err := c.Compact(ctx); compact <- err }()
	wait()
	_, err = c.NewSession(ctx)
	must(err)
	select {
	case err = <-compact:
		if err == nil {
			t.Fatal("replaced compaction succeeded")
		}
	case <-ctx.Done():
		t.Fatal("stale compact response")
	}
	fresh, err := c.GetState(ctx)
	must(err)
	if fresh.SessionID == state.SessionID || fresh.MessageCount != 0 {
		t.Fatal(fresh)
	}
	sourceBefore, err := os.ReadFile(*state.SessionFile)
	must(err)
	hold.Store(false)
	for i := 0; i < 3; i++ {
		_, err = c.PromptAndWait(ctx, "new generation")
		must(err)
	}
	sourceAfter, err := os.ReadFile(*state.SessionFile)
	must(err)
	if !bytes.Equal(sourceBefore, sourceAfter) {
		t.Fatal("old task wrote after replacement")
	}
	hold.Store(true)
	must(c.Prompt(ctx, "abort on switch"))
	wait()
	_, err = c.SwitchSession(ctx, *state.SessionFile)
	must(err)
	hold.Store(false)
	restored, err := c.GetState(ctx)
	must(err)
	if restored.SessionID != state.SessionID {
		t.Fatal(restored)
	}
	_, err = c.PromptAndWait(ctx, "restored generation")
	must(err)
	hold.Store(true)
	must(c.Prompt(ctx, "clone running turn"))
	wait()
	_, err = c.Clone(ctx)
	must(err)
	hold.Store(false)
	clonedMessages, err := c.GetMessages(ctx)
	must(err)
	drained, err := codingagent.OpenSessionManager(*restored.SessionFile, nil, nil)
	must(err)
	drainedJSON, _ := json.Marshal(drained.BuildSessionContext().Messages)
	clonedJSON, _ := json.Marshal(clonedMessages)
	if !bytes.Equal(drainedJSON, clonedJSON) {
		t.Fatalf("clone missed drained turn: %s / %s", clonedJSON, drainedJSON)
	}
	bashStarted := make(chan struct{}, 1)
	unsubscribe, err := c.OnEvent(func(event codingagent.JSONAgentSessionEvent) {
		if event, ok := event.(*codingagent.AgentSessionBashExecutionUpdateEvent); ok && strings.Contains(event.Delta, "bash-ready") {
			select {
			case bashStarted <- struct{}{}:
			default:
			}
		}
	})
	must(err)
	defer unsubscribe()
	bash := make(chan error, 1)
	go func() {
		result, err := c.Bash(ctx, "printf bash-ready; sleep 30")
		if err == nil && !result.Cancelled {
			err = fmt.Errorf("replacement did not cancel Bash")
		}
		bash <- err
	}()
	select {
	case <-bashStarted:
	case <-ctx.Done():
		t.Fatal("Bash did not start")
	}
	_, err = c.Clone(ctx)
	must(err)
	select {
	case err = <-bash:
		must(err)
	case <-ctx.Done():
		t.Fatal("Bash response stuck")
	}
	bashMessages, err := c.GetMessages(ctx)
	must(err)
	bashJSON, _ := json.Marshal(bashMessages)
	if !strings.Contains(string(bashJSON), "bash-ready") {
		t.Fatal("clone lost drained Bash result")
	}
	generation := make(chan error, 1)
	go func() {
		for i := 0; i < 12; i++ {
			if _, err := c.PromptAndWait(ctx, "concurrent tree"); err != nil {
				generation <- err
				return
			}
		}
		generation <- nil
	}()
	querying := true
	for querying {
		tree, leaf, err := c.GetTree(ctx)
		must(err)
		found := leaf == nil
		for len(tree) > 0 {
			node := tree[0]
			tree = tree[1:]
			if leaf != nil && node.Entry.ID == *leaf {
				found = true
			}
			tree = append(tree, node.Children...)
		}
		if !found {
			t.Fatal("tree leaf is outside returned snapshot")
		}
		select {
		case err = <-generation:
			must(err)
			querying = false
		default:
		}
	}
	must(c.Stop(context.Background()))
	stderr, err := c.GetStderr()
	must(err)
	if strings.Contains(stderr, "DATA RACE") {
		t.Fatal(stderr)
	}
}

func TestRPC96AutoCompactionAfterReplacement(t *testing.T) {
	var overflow atomic.Bool
	var count atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if overflow.CompareAndSwap(true, false) {
			w.WriteHeader(400)
			io.WriteString(w, `{"error":{"message":"maximum context length exceeded"}}`)
			return
		}
		count.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"auto answer\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":2,\"total_tokens\":12}}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	c, ctx := rpcClient95(t, server.URL, `{"compaction":{"enabled":false,"keepRecentTokens":1},"retry":{"enabled":false}}`)
	defer c.Stop(context.Background())
	if _, err := c.NewSession(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := c.PromptAndWait(ctx, "seed"); err != nil {
		t.Fatal(err)
	}
	if err := c.SetAutoCompaction(ctx, true); err != nil {
		t.Fatal(err)
	}
	overflow.Store(true)
	events, err := c.PromptAndWait(ctx, "overflow and continue")
	if err != nil {
		t.Fatal(err)
	}
	var start, end, retried bool
	for _, event := range events {
		switch event := event.(type) {
		case *codingagent.AgentSessionCompactionStartEvent:
			start = true
		case *codingagent.AgentSessionCompactionEndEvent:
			end = event.Result != nil
			retried = event.WillRetry
		}
	}
	if !start || !end || !retried || count.Load() < 3 {
		t.Fatalf("auto recovery: start=%v end=%v retried=%v calls=%d", start, end, retried, count.Load())
	}
	entries, _, err := c.GetEntries(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range entries {
		if entry.Type == "compaction" {
			found = true
		}
	}
	if !found {
		t.Fatal("automatic compaction not persisted")
	}
	text, err := c.GetLastAssistantText(ctx)
	if err != nil || text == nil || *text != "auto answer" {
		t.Fatal(text, err)
	}
}

func TestRPC96RestoresModelAndCWD(t *testing.T) {
	requests := make(chan map[string]any, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		requests <- request
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"restored config\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	binary, dir, cwd := buildPigBinary(t), t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"defaultProvider":"deepseek","defaultModel":"deepseek-v4-flash","compaction":{"enabled":false}}`), 0600); err != nil {
		t.Fatal(err)
	}
	c := codingagent.NewRPCClient(codingagent.RPCClientOptions{CLIPath: &binary, CWD: &cwd, Args: []string{"--provider", "deepseek", "--no-tools", "--no-context-files", "--offline"}, Env: map[string]string{"DEEPSEEK_API_KEY": "fixture", "PIG_CODING_AGENT_DIR": dir, "PIG_DEEPSEEK_BASE_URL": server.URL}})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(c.Start(ctx))
	defer c.Stop(context.Background())
	_, err := c.SetModel(ctx, "deepseek", "deepseek-v4-pro")
	must(err)
	must(c.SetThinkingLevel(ctx, "max"))
	_, err = c.PromptAndWait(ctx, "save model")
	must(err)
	<-requests
	state, err := c.GetState(ctx)
	must(err)
	data, err := os.ReadFile(*state.SessionFile)
	must(err)
	lines := bytes.Split(data, []byte{'\n'})
	var header map[string]any
	must(json.Unmarshal(lines[0], &header))
	targetCWD := t.TempDir()
	header["id"], header["cwd"] = "rpc96-restored", targetCWD
	lines[0], err = json.Marshal(header)
	must(err)
	target := filepath.Join(t.TempDir(), "target.jsonl")
	must(os.WriteFile(target, bytes.Join(lines, []byte{'\n'}), 0600))
	_, err = c.SetModel(ctx, "deepseek", "deepseek-v4-flash")
	must(err)
	must(c.SetThinkingLevel(ctx, "off"))
	original, err := os.ReadFile(*state.SessionFile)
	must(err)
	_, err = c.SwitchSession(ctx, target)
	must(err)
	restored, err := c.GetState(ctx)
	must(err)
	if restored.SessionID != "rpc96-restored" || restored.Model.ID != "deepseek-v4-pro" || restored.ThinkingLevel != "max" {
		t.Fatal(restored)
	}
	bash, err := c.Bash(ctx, "pwd")
	must(err)
	actualCWD, err := filepath.EvalSymlinks(strings.TrimSpace(bash.Output))
	must(err)
	expectedCWD, err := filepath.EvalSymlinks(targetCWD)
	must(err)
	if actualCWD != expectedCWD {
		t.Fatalf("Bash cwd=%s, want %s", actualCWD, expectedCWD)
	}
	_, err = c.PromptAndWait(ctx, "use restored config")
	must(err)
	request := <-requests
	if request["model"] != "deepseek-v4-pro" || request["reasoning_effort"] != "max" {
		t.Fatal("provider configuration not rebound")
	}
	after, err := os.ReadFile(*state.SessionFile)
	must(err)
	if !bytes.Equal(original, after) {
		t.Fatal("target wrote original source")
	}
	_, err = c.SwitchSession(ctx, target)
	must(err)
	if _, err = c.PromptAndWait(ctx, "same identity reload"); err != nil {
		t.Fatal(err)
	}
}
