//go:build darwin || linux

package codingagent_test

import (
	"context"
	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/terminaltest"
	"github.com/nankedr/pig/tui"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestInteractiveModelsBusyAndDraft117(t *testing.T) {
	catalog, models, _ := config87Runtime(t)
	core, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	response, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("SDK_DONE"))
	core.SetResponses([]ai.FauxResponseStep{response})
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), Model: &models[0], ModelRuntime: catalog, NoTools: codingagent.NoToolsAll, StreamFunction: func(ctx context.Context, m ai.Model, input ai.Context, opts ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
		close(started)
		select {
		case <-release:
		case <-ctx.Done():
		}
		return core.StreamSimple(ctx, m, input, opts)
	}})
	if err != nil {
		t.Fatal(err)
	}
	session := created.Session
	runtime := codingagent.NewAgentSessionRuntime(session, codingagent.AgentSessionServices{}, nil, nil, nil)
	tty := terminaltest.Open(t)
	mode := codingagent.NewInteractiveMode(runtime, codingagent.InteractiveModeOptions{Terminal: tui.NewProcessTerminal(tty.Slave, tty.Slave)})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- mode.Run(ctx) }()
	tty.Wait(t, "> ")
	tty.Send(t, "draft\x0c")
	tty.Wait(t, "Select Model")
	tty.Send(t, "\x1b[27u")
	deadline := time.Now().Add(3 * time.Second)
	for !strings.Contains(tty.ScreenText(), "> draft") && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if !strings.Contains(tty.ScreenText(), "> draft") {
		t.Fatal("cancel lost draft", tty.ScreenText())
	}
	tty.Send(t, "\r")
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("request not started")
	}
	tty.Send(t, "/model deepseek-v4-pro\r")
	tty.Wait(t, "AgentSession is busy")
	if session.Model().ID != models[0].ID {
		t.Fatal("busy switch changed model")
	}
	if err = session.SetEnabledModels([]string{"deepseek/deepseek-v4-pro"}, true); err == nil {
		t.Fatal("scope accepted while busy")
	}
	once.Do(func() { close(release) })
	tty.Wait(t, "SDK_DONE")
	if err = session.WaitForIdle(ctx); err != nil {
		t.Fatal(err)
	}
	tty.Send(t, "\x04")
	if err = awaitInteractive(t, done); err != nil {
		t.Fatal(err)
	}
	if !tty.Restored(t) {
		t.Fatal("terminal not restored")
	}
}
func TestModelSelectionFailurePreservesConfiguration117(t *testing.T) {
	runtime, models, store := config87Runtime(t)
	provider := "deepseek"
	settings, _ := codingagent.NewInMemorySettingsManager(codingagent.Settings{DefaultModel: &models[0].ID, DefaultProvider: &provider})
	created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), Model: &models[0], ModelRuntime: runtime, SettingsManager: settings})
	if err != nil {
		t.Fatal(err)
	}
	defer created.Session.Dispose()
	s := created.Session
	missing := models[0]
	missing.ID = "unknown"
	unsupported, _, _ := runtime.GetModel("openai", "gpt-4o")
	for _, target := range []ai.Model{missing, unsupported, models[1]} {
		if target.ID == models[1].ID {
			if err = store.Delete(context.Background(), "deepseek", ai.AuthOperationOptions{}); err != nil {
				t.Fatal(err)
			}
		}
		before := len(s.SessionManager().GetEntries())
		var failure error
		selector := codingagent.NewModelSelectorComponent([]ai.Model{target}, nil, nil, func(m ai.Model) { failure = s.SetModel(m) }, nil)
		if err = selector.HandleInput("\r"); err != nil {
			t.Fatal(err)
		}
		if failure == nil || s.Model().ID != models[0].ID || len(s.SessionManager().GetEntries()) != before {
			t.Fatal("failed selection partially committed", failure)
		}
		if model, _ := settings.GetDefaultModel(); model != models[0].ID {
			t.Fatal("settings changed", model)
		}
	}
}

func TestInteractiveModelBindings117(t *testing.T) {
	catalog, models, _ := config87Runtime(t)
	core, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), Model: &models[0], ModelRuntime: catalog, NoTools: codingagent.NoToolsAll, StreamFunction: agent.StreamFunction(core.StreamSimple)})
	if err != nil {
		t.Fatal(err)
	}
	runtime := codingagent.NewAgentSessionRuntime(created.Session, codingagent.AgentSessionServices{}, nil, nil, nil)
	bindings, err := codingagent.NewKeybindingsManager(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = bindings.SetUserBindings(tui.KeybindingsConfig{"tui.select.confirm": {"ctrl+y"}, "tui.select.cancel": {"ctrl+g"}, "tui.editor.deleteCharBackward": {"ctrl+q"}}); err != nil {
		t.Fatal(err)
	}
	tty := terminaltest.Open(t)
	mode := codingagent.NewInteractiveMode(runtime, codingagent.InteractiveModeOptions{Terminal: tui.NewProcessTerminal(tty.Slave, tty.Slave), Keybindings: bindings})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- mode.Run(ctx) }()
	tty.Wait(t, "> ")
	tty.Send(t, "/settings\r")
	tty.Wait(t, "Thinking level")
	tty.Send(t, "\x19")
	tty.Wait(t, "Thinking Level")
	tty.Send(t, "\x07")
	time.Sleep(50 * time.Millisecond)
	tty.Send(t, "\x07")
	time.Sleep(50 * time.Millisecond)
	tty.Send(t, "\x0c")
	tty.Wait(t, "Select Model")
	tty.Send(t, "queryz\x11")
	deadline := time.Now().Add(time.Second)
	for !strings.Contains(tty.ScreenText(), "query ") && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if strings.Contains(tty.ScreenText(), "queryz") || !strings.Contains(tty.ScreenText(), "query") {
		t.Fatal("search ignored instance edit key", tty.ScreenText())
	}
	tty.Send(t, "\x07")
	time.Sleep(50 * time.Millisecond)
	tty.Send(t, "\x04")
	if err = awaitInteractive(t, done); err != nil {
		t.Fatal(err)
	}
}
