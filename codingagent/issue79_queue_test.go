package codingagent_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

type editTestOperations struct {
	access func(context.Context, string) error
	read   func(context.Context, string) ([]byte, error)
	write  func(context.Context, string, []byte) error
}

func (o editTestOperations) Access(ctx context.Context, path string) error {
	if o.access != nil {
		return o.access(ctx, path)
	}
	_, err := os.Stat(path)
	return err
}
func (o editTestOperations) ReadFile(ctx context.Context, path string) ([]byte, error) {
	if o.read != nil {
		return o.read(ctx, path)
	}
	return os.ReadFile(path)
}
func (o editTestOperations) WriteFile(ctx context.Context, path string, data []byte) error {
	if o.write != nil {
		return o.write(ctx, path, data)
	}
	return os.WriteFile(path, data, 0600)
}
func editArgs(path, old, newText string) map[string]any {
	return map[string]any{"path": path, "edits": []any{map[string]any{"oldText": old, "newText": newText}}}
}

func TestEditToolMixedMutationQueue(t *testing.T) {
	for _, stage := range []string{"write-before-edit", "edit-before-write", "cancel-access", "cancel-read", "cancel-write", "error-access", "error-read", "error-write"} {
		t.Run(stage, func(t *testing.T) {
			cwd := t.TempDir()
			target := filepath.Join(cwd, "target")
			if err := os.WriteFile(target, []byte("original"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, filepath.Join(cwd, "alias")); err != nil {
				t.Fatal(err)
			}
			started, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			defer once.Do(func() { close(release) })
			block := func() error {
				close(started)
				<-release
				if strings.HasPrefix(stage, "error-") {
					return os.ErrPermission
				}
				return nil
			}
			operations := editTestOperations{}
			switch {
			case strings.HasSuffix(stage, "access"):
				operations.access = func(context.Context, string) error { return block() }
			case strings.HasSuffix(stage, "read"):
				operations.read = func(_ context.Context, path string) ([]byte, error) {
					if err := block(); err != nil {
						return nil, err
					}
					return os.ReadFile(path)
				}
			default:
				operations.write = func(_ context.Context, path string, data []byte) error {
					if err := block(); err != nil {
						return err
					}
					return os.WriteFile(path, data, 0600)
				}
			}
			first, err := codingagent.CreateEditToolDefinition(cwd, codingagent.EditToolOptions{Operations: operations})
			if err != nil {
				t.Fatal(err)
			}
			firstArgs := editArgs("target", "original", "first")
			next, err := codingagent.CreateWriteToolDefinition(cwd)
			if err != nil {
				t.Fatal(err)
			}
			nextArgs := map[string]any{"path": "alias", "content": "second"}
			if stage == "write-before-edit" {
				first, err = codingagent.CreateWriteToolDefinition(cwd, codingagent.WriteToolOptions{Operations: writeTestOperations{write: operations.write}})
				if err != nil {
					t.Fatal(err)
				}
				firstArgs = map[string]any{"path": "target", "content": "first"}
				next, err = codingagent.CreateEditToolDefinition(cwd)
				if err != nil {
					t.Fatal(err)
				}
				nextArgs = editArgs("alias", "first", "second")
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			firstDone := make(chan error, 1)
			go func() { _, err := first.Execute(ctx, "first", firstArgs, nil); firstDone <- err }()
			select {
			case <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("mutation did not start")
			}
			if strings.HasPrefix(stage, "cancel-") {
				cancel()
			}
			nextDone := make(chan error, 1)
			go func() { _, err := next.Execute(context.Background(), "next", nextArgs, nil); nextDone <- err }()
			independent, _ := codingagent.CreateWriteToolDefinition(cwd)
			otherDone := make(chan error, 1)
			go func() {
				_, err := independent.Execute(context.Background(), "other", map[string]any{"path": "other", "content": "independent"}, nil)
				otherDone <- err
			}()
			select {
			case err := <-otherDone:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("unrelated file blocked")
			}
			select {
			case err := <-nextDone:
				t.Fatalf("same file ran before settlement: %v", err)
			case <-time.After(30 * time.Millisecond):
			}
			once.Do(func() { close(release) })
			select {
			case err := <-firstDone:
				switch {
				case strings.HasPrefix(stage, "cancel-"):
					if !errors.Is(err, context.Canceled) {
						t.Fatalf("cancel: %v", err)
					}
				case strings.HasPrefix(stage, "error-"):
					if err == nil {
						t.Fatal("error reported as success")
					}
				default:
					if err != nil {
						t.Fatal(err)
					}
				}
			case <-time.After(3 * time.Second):
				t.Fatal("first mutation did not settle")
			}
			select {
			case err := <-nextDone:
				if err != nil {
					t.Fatal(err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("queue did not release")
			}
			back := runReadTool(t, context.Background(), cwd, map[string]any{"path": "target"})
			if back.IsError || readToolResultText(t, back) != "second" {
				t.Fatalf("lost mixed update: %#v", back)
			}
		})
	}
}

func TestEditToolInvalidArgumentsAndErrorsContinueWithoutWrites(t *testing.T) {
	cwd := t.TempDir()
	for _, args := range []map[string]any{
		{"path": "target"}, {"path": "target", "edits": "not json"}, {"path": "target", "edits": []any{nil}},
		{"path": "target", "edits": []any{map[string]any{"oldText": "original", "newText": 3}}},
		{"path": "target", "edits": []any{map[string]any{"newText": "bad"}}},
		{"edits": []any{}}, editArgs("bad\x00path", "original", "bad"), editArgs("missing", "original", "bad"), editArgs("directory", "original", "bad"),
	} {
		if err := os.WriteFile(filepath.Join(cwd, "target"), []byte("original"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(cwd, "directory"), 0700); err != nil {
			t.Fatal(err)
		}
		tool, err := codingagent.CreateEditTool(cwd)
		if err != nil {
			t.Fatal(err)
		}
		read, err := codingagent.CreateReadTool(cwd)
		if err != nil {
			t.Fatal(err)
		}
		got := runFileToolSession(t, context.Background(), cwd, []agent.ErasedAgentTool{tool, read}, []ai.ToolCall{
			{Type: "toolCall", Name: "edit", ID: "bad", Arguments: args},
			{Type: "toolCall", Name: "read", ID: "read", Arguments: map[string]any{"path": "target"}},
		})
		if !got[0].IsError || got[1].IsError || readToolResultText(t, got[1]) != "original" {
			t.Fatalf("invalid call changed file: %#v", got)
		}
	}
	for _, stage := range []string{"access", "read", "write"} {
		ops := editTestOperations{}
		switch stage {
		case "access":
			ops.access = func(context.Context, string) error { return errors.New("disk offline") }
		case "read":
			ops.read = func(context.Context, string) ([]byte, error) { return nil, os.ErrPermission }
		case "write":
			ops.write = func(context.Context, string, []byte) error { return os.ErrPermission }
		}
		tool, err := codingagent.CreateEditTool(cwd, codingagent.EditToolOptions{Operations: ops})
		if err != nil {
			t.Fatal(err)
		}
		got := runFileToolSession(t, context.Background(), cwd, []agent.ErasedAgentTool{tool}, []ai.ToolCall{{Type: "toolCall", Name: "edit", ID: "error", Arguments: editArgs("target", "original", "bad")}})
		if !got[0].IsError {
			t.Fatalf("false success: %#v", got)
		}
		if stage == "access" && readToolResultText(t, got[0]) != "Could not edit file: target. Error: disk offline." {
			t.Fatal(readToolResultText(t, got[0]))
		}
	}
}

func TestEditToolSessionAbortWaitsForMutation(t *testing.T) {
	cwd := t.TempDir()
	if err := os.WriteFile(filepath.Join(cwd, "result"), []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	write, err := codingagent.CreateEditTool(cwd, codingagent.EditToolOptions{Operations: editTestOperations{write: func(_ context.Context, path string, content []byte) error {
		close(started)
		<-release
		return os.WriteFile(path, content, 0600)
	}}})
	if err != nil {
		t.Fatal(err)
	}
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	call := ai.ToolCall{Type: "toolCall", ID: "abort", Name: "edit", Arguments: editArgs("result", "original", "in flight")}
	response, err := ai.FauxAssistantMessage(ai.FauxAssistantBlocks(call), ai.FauxAssistantMessageOptions{StopReason: ai.Some(ai.StopReasonToolUse)})
	if err != nil {
		t.Fatal(err)
	}
	core.SetResponses([]ai.FauxResponseStep{response})
	model, _ := core.GetModel()
	created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: cwd, Model: &model, AgentTools: []agent.ErasedAgentTool{write}, Tools: []string{"edit"}, StreamFunction: agent.StreamFunction(core.StreamSimple)})
	if err != nil {
		t.Fatal(err)
	}
	defer created.Session.Dispose()
	done := make(chan error, 1)
	go func() { done <- created.Session.Prompt(context.Background(), "save") }()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("write did not start")
	}
	if err := created.Session.Abort(); err != nil {
		t.Fatal(err)
	}
	waitCtx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := created.Session.WaitForIdle(waitCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("settled before mutation finished: %v", err)
	}
	once.Do(func() { close(release) })
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("abort did not settle")
	}
	found := false
	for _, message := range created.Session.Messages() {
		if result, ok := message.(ai.ToolResultMessage); ok {
			found = true
			if !result.IsError || strings.Contains(readToolResultText(t, result), "Successfully replaced") {
				t.Fatalf("false success on cancel: %#v", result)
			}
		}
	}
	if !found {
		t.Fatal("missing canceled ToolResult")
	}
	next, err := codingagent.CreateWriteToolDefinition(cwd)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := next.Execute(context.Background(), "next", map[string]any{"path": "result", "content": "next"}, nil); err != nil {
		t.Fatal(err)
	}
	back := runReadTool(t, context.Background(), cwd, map[string]any{"path": "result"})
	if back.IsError || readToolResultText(t, back) != "next" {
		t.Fatalf("queue after abort: %#v", back)
	}
}
