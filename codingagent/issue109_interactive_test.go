//go:build darwin || linux

package codingagent_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/terminaltest"
	"github.com/nankedr/pig/tui"
)

func interactiveRuntime(t *testing.T) *codingagent.AgentSessionRuntime {
	t.Helper()
	provider, err := ai.NewFauxProvider()
	if err != nil {
		t.Fatal(err)
	}
	first, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("FIRST_STREAM_TOKEN FIRST_DONE"))
	second, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("SECOND_DONE"))
	provider.SetResponses([]ai.FauxResponseStep{first, second})
	model, _ := provider.GetModel()
	created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), Model: &model, Provider: provider.Provider, NoTools: codingagent.NoToolsAll})
	if err != nil {
		t.Fatal(err)
	}
	runtime := codingagent.NewAgentSessionRuntime(created.Session, codingagent.AgentSessionServices{}, nil, nil, nil)
	t.Cleanup(func() { _ = runtime.Dispose(context.Background()) })
	return runtime
}
func awaitInteractive(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("Interactive did not stop")
		return nil
	}
}
func TestInteractiveSDKConversationAndStop(t *testing.T) {
	tty := terminaltest.Open(t)
	runtime := interactiveRuntime(t)
	terminal := tui.NewProcessTerminal(tty.Slave, tty.Slave)
	mode := codingagent.NewInteractiveMode(runtime, codingagent.InteractiveModeOptions{Terminal: terminal})
	if err := mode.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- mode.Run(context.Background()) }()
	tty.Send(t, "first questiox\x7fn\r")
	tty.Wait(t, "FIRST_DONE")
	tty.Send(t, "second question\r")
	tty.Wait(t, "SECOND_DONE")
	if err := mode.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := awaitInteractive(t, done); err != nil {
		t.Fatal(err)
	}
	if !tty.Restored(t) {
		t.Fatal("raw mode survived Stop")
	}
	select {
	case <-terminal.Done():
	default:
		t.Fatal("terminal reader survived Stop")
	}
	messages := runtime.Session().SessionManager().BuildSessionContext().Messages
	if len(messages) != 4 {
		t.Fatalf("Session messages: %v", messages)
	}
	user := messages[0].(ai.UserMessage)
	text, _ := user.Content.Text()
	if blocks, ok := user.Content.Blocks(); ok {
		for _, block := range blocks {
			if b, ok := block.(ai.TextContent); ok {
				text += b.Text
			}
		}
	}
	if text != "first question" {
		t.Fatalf("edited input: %q", text)
	}
	if err := mode.Stop(); err != nil {
		t.Fatalf("second Stop: %v", err)
	}
}
func TestInteractiveSDKCancel(t *testing.T) {
	tty := terminaltest.Open(t)
	mode := codingagent.NewInteractiveMode(interactiveRuntime(t), codingagent.InteractiveModeOptions{Terminal: tui.NewProcessTerminal(tty.Slave, tty.Slave)})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- mode.Run(ctx) }()
	tty.Wait(t, "> ")
	cancel()
	if err := awaitInteractive(t, done); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel: %v", err)
	}
	if !tty.Restored(t) {
		t.Fatal("raw mode survived cancellation")
	}
}

type interactiveFailWriter struct {
	mu             sync.Mutex
	output         io.Writer
	writes, failAt int
}

func (w *interactiveFailWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.writes++
	if w.writes == w.failAt {
		return 0, io.ErrClosedPipe
	}
	return w.output.Write(p)
}
func TestInteractiveSDKRenderingFailureRestoresTerminal(t *testing.T) {
	for _, failAt := range []int{1, 2, 4} {
		t.Run(string(rune('0'+failAt)), func(t *testing.T) {
			tty := terminaltest.Open(t)
			writer := &interactiveFailWriter{output: tty.Slave, failAt: failAt}
			prompt := "first question"
			mode := codingagent.NewInteractiveMode(interactiveRuntime(t), codingagent.InteractiveModeOptions{Terminal: tui.NewProcessTerminal(tty.Slave, writer), InitialMessage: &prompt})
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := mode.Run(ctx); !errors.Is(err, io.ErrClosedPipe) {
				t.Fatalf("render failure: %v", err)
			}
			if !tty.Restored(t) {
				t.Fatal("raw mode survived output failure")
			}
			tty.Wait(t, "\x1b[?2004l")
			if !tty.CursorVisible() || !tty.PasteDisabled() {
				t.Fatal("terminal output state was not restored")
			}
		})
	}
}
func TestInteractiveSDKRequiresInitBeforeRendering(t *testing.T) {
	tty := terminaltest.Open(t)
	mode := codingagent.NewInteractiveMode(interactiveRuntime(t), codingagent.InteractiveModeOptions{Terminal: tui.NewProcessTerminal(tty.Slave, tty.Slave)})
	defer mode.Stop()
	if err := mode.ShowError("before initialization"); err == nil {
		t.Fatal("rendering before Init unexpectedly succeeded")
	}
	if !tty.Restored(t) {
		t.Fatal("constructor changed raw mode")
	}
}

type eofInteractiveTerminal struct {
	tui.Terminal
	done chan struct{}
}

func (t *eofInteractiveTerminal) Done() <-chan struct{} { return t.done }
func (t *eofInteractiveTerminal) Err() error            { return io.EOF }
func TestInteractiveSDKEOF(t *testing.T) {
	tty := terminaltest.Open(t)
	terminal := &eofInteractiveTerminal{Terminal: tui.NewProcessTerminal(tty.Slave, tty.Slave), done: make(chan struct{})}
	mode := codingagent.NewInteractiveMode(interactiveRuntime(t), codingagent.InteractiveModeOptions{Terminal: terminal})
	done := make(chan error, 1)
	go func() { done <- mode.Run(context.Background()) }()
	tty.Wait(t, "> ")
	close(terminal.done)
	if err := awaitInteractive(t, done); err != nil {
		t.Fatal(err)
	}
	if !tty.Restored(t) {
		t.Fatal("raw mode survived EOF")
	}
}

func TestInteractiveSDKCancelActiveTurn(t *testing.T) { testInteractiveCancel(t, false) }
func TestInteractiveSDKInterruptKey111(t *testing.T)  { testInteractiveCancel(t, true) }
func testInteractiveCancel(t *testing.T, shortcut bool) {
	canceled := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"PARTIAL_BEFORE_CANCEL\"},\"finish_reason\":null}]}\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(canceled)
	}))
	defer server.Close()
	key, baseURL := "synthetic", server.URL
	runtime, err := codingagent.CreateHeadlessSession(context.Background(), codingagent.CreateHeadlessSessionOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), Provider: "deepseek", Model: "deepseek-v4-flash", APIKey: &key, BaseURL: &baseURL, NoTools: codingagent.NoToolsAll})
	if err != nil {
		t.Fatal(err)
	}
	tty := terminaltest.Open(t)
	bindings, err := codingagent.NewKeybindingsManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	bindings.SetUserBindings(tui.KeybindingsConfig{"app.interrupt": {"ctrl+g"}})
	mode := codingagent.NewInteractiveMode(runtime, codingagent.InteractiveModeOptions{Terminal: tui.NewProcessTerminal(tty.Slave, tty.Slave), Keybindings: bindings})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- mode.Run(ctx) }()
	tty.Wait(t, "> ")
	tty.Send(t, "hold request\r")
	tty.Wait(t, "PARTIAL_BEFORE_CANCEL")
	if shortcut {
		tty.Send(t, "\x1b[103;5:3u")
		select {
		case <-canceled:
			t.Fatal("release aborted turn")
		case <-time.After(30 * time.Millisecond):
		}
		tty.Send(t, "\x1b[103;5u")
	} else {
		cancel()
	}

	select {
	case <-canceled:
	case <-time.After(5 * time.Second):
		t.Fatal("Provider request survived cancellation")
	}
	if shortcut {
		tty.Send(t, "\x04")
	}
	if err := awaitInteractive(t, done); shortcut && err != nil || !shortcut && !errors.Is(err, context.Canceled) {
		t.Fatalf("active cancel: %v", err)
	}

	if !tty.Restored(t) {
		t.Fatal("raw mode survived active cancellation")
	}
	messages := runtime.Session().SessionManager().BuildSessionContext().Messages
	if len(messages) != 2 {
		t.Fatalf("canceled transcript: %v", messages)
	}
	assistant := messages[1].(ai.AssistantMessage)
	if assistant.StopReason != ai.StopReasonAborted {
		t.Fatalf("stop reason: %s", assistant.StopReason)
	}
}

func TestInteractiveSDKInvalidRunRestoresInitializedTerminal(t *testing.T) {
	tty := terminaltest.Open(t)
	mode := codingagent.NewInteractiveMode(interactiveRuntime(t), codingagent.InteractiveModeOptions{Terminal: tui.NewProcessTerminal(tty.Slave, tty.Slave)})
	defer mode.Stop()
	if err := mode.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := mode.Run(nil); err == nil {
		t.Fatal("nil context accepted")
	}
	if !tty.Restored(t) {
		t.Fatal("raw mode survived invalid Run")
	}
}

type restoreFailWriter struct{ io.Writer }

func (w restoreFailWriter) Write(p []byte) (int, error) {
	if strings.Contains(string(p), "\x1b[?2004l") {
		return 0, io.ErrClosedPipe
	}
	return w.Writer.Write(p)
}
func TestInteractiveSDKCancellationPreservesCleanupFailure(t *testing.T) {
	tty := terminaltest.Open(t)
	mode := codingagent.NewInteractiveMode(interactiveRuntime(t), codingagent.InteractiveModeOptions{Terminal: tui.NewProcessTerminal(tty.Slave, restoreFailWriter{tty.Slave})})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- mode.Run(ctx) }()
	tty.Wait(t, "> ")
	cancel()
	err := awaitInteractive(t, done)
	if !errors.Is(err, context.Canceled) || !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("cleanup error lost: %v", err)
	}
	if !tty.Restored(t) {
		t.Fatal("raw mode survived cleanup output failure")
	}
}
