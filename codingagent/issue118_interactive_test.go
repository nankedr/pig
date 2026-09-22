//go:build darwin || linux

package codingagent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/terminaltest"
	"github.com/nankedr/pig/tui"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestInteractiveSessionSwitchTrustAndBusy118(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	started, aborted := make(chan struct{}), make(chan struct{})
	var first sync.Once
	requests := make(chan string, 10)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Role    string
				Content json.RawMessage
			}
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		last, system := "", ""
		for _, m := range body.Messages {
			if m.Role == "user" {
				if json.Unmarshal(m.Content, &last) != nil {
					var parts []struct{ Text string }
					_ = json.Unmarshal(m.Content, &parts)
					last = ""
					for _, p := range parts {
						last += p.Text
					}
				}
			}
			if m.Role == "system" {
				_ = json.Unmarshal(m.Content, &system)
			}
		}
		if last == "blocking turn" {
			first.Do(func() { close(started) })
			<-r.Context().Done()
			close(aborted)
			return
		}
		requests <- system
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"SWITCH_DONE\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	cwd, other, dir, agentDir := t.TempDir(), t.TempDir(), t.TempDir(), t.TempDir()
	catalog, _, _ := config87Runtime(t)
	create := func(cwd string) *codingagent.AgentSessionRuntime {
		provider, model := "deepseek", "deepseek-v4-flash"
		settings, _ := codingagent.NewInMemorySettingsManager(codingagent.Settings{DefaultProvider: &provider, DefaultModel: &model})
		manager, err := codingagent.NewSessionManager(cwd, &dir)
		if err != nil {
			t.Fatal(err)
		}
		runtime, err := codingagent.CreateHeadlessSession(ctx, codingagent.CreateHeadlessSessionOptions{CWD: cwd, AgentDir: agentDir, SessionManager: manager, SettingsManager: settings, ModelRuntime: catalog, BaseURL: &server.URL, NoTools: codingagent.NoToolsAll})
		if err != nil {
			t.Fatal(err)
		}
		return runtime
	}
	target := create(other)
	if err := target.Session().SetSessionName("TARGET_TWO"); err != nil {
		t.Fatal(err)
	}
	if err := target.Session().Prompt(ctx, "target history"); err != nil {
		t.Fatal(err)
	}
	targetPath := *target.Session().SessionFile()
	target.Dispose(ctx)
	<-requests
	// The untrusted FIFO must never be opened during cross-project replacement.
	if err := os.Mkdir(filepath.Join(other, ".pig"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := syscall.Mkfifo(filepath.Join(other, ".pig", "settings.json"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "AGENTS.md"), []byte("TARGET_CONTEXT_118"), 0600); err != nil {
		t.Fatal(err)
	}
	runtime := create(cwd)
	old := runtime.Session()
	if err := old.Prompt(ctx, "source history"); err != nil {
		t.Fatal(err)
	}
	<-requests
	var cleaned atomic.Bool
	unregister, err := ai.RegisterSessionResourceCleanup(func(ids ...string) {
		for _, id := range ids {
			if id == old.SessionID() {
				cleaned.Store(true)
			}
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer unregister()
	tty := terminaltest.Open(t)
	mode := codingagent.NewInteractiveMode(runtime, codingagent.InteractiveModeOptions{Terminal: tui.NewProcessTerminal(tty.Slave, tty.Slave)})
	done := make(chan error, 1)
	go func() { done <- mode.Run(ctx) }()
	tty.Wait(t, "> ")
	tty.Send(t, "blocking turn\r")
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("generation not started")
	}
	tty.Send(t, "/resume\r")
	tty.Wait(t, "Resume Session")
	tty.Send(t, "\x1b[27u")
	time.Sleep(100 * time.Millisecond)
	select {
	case <-aborted:
		t.Fatal("cancelling selector aborted current generation")
	default:
	}
	tty.Send(t, "/resume\r")
	time.Sleep(100 * time.Millisecond)
	tty.Send(t, "\t")
	tty.Wait(t, "TARGET_TWO")
	tty.Send(t, "TARGET_TWO\r")
	tty.Wait(t, "Trust project folder?")
	select {
	case <-aborted:
	case <-ctx.Done():
		t.Fatal("old generation not cancelled")
	}
	tty.Send(t, "\x1b[27u")
	tty.Wait(t, "Resumed session")
	tty.Send(t, "after switch\r")
	var system string
	select {
	case system = <-requests:
	case <-ctx.Done():
		t.Fatal("new Session did not generate")
	}
	if !strings.Contains(system, "TARGET_CONTEXT_118") {
		t.Fatal("stale project context", system)
	}
	time.Sleep(100 * time.Millisecond)
	tty.Send(t, "\x04")
	if err = awaitInteractive(t, done); err != nil {
		t.Fatal(err)
	}
	if !cleaned.Load() || !tty.Restored(t) {
		t.Fatal("old runtime or terminal leaked")
	}
	if *runtime.Session().SessionFile() != targetPath || runtime.CWD() != other {
		t.Fatal("wrong Session identity")
	}
	if trusted, _ := runtime.Session().SettingsManager().IsProjectTrusted(); trusted {
		t.Fatal("cancel granted project trust")
	}
	if err = old.Prompt(context.Background(), "stale"); err == nil {
		t.Fatal("old Session usable")
	}
	reopened, err := codingagent.OpenSessionManager(targetPath, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	users := []string{}
	for _, m := range reopened.BuildSessionContext().Messages {
		if m.MessageRole() == ai.MessageRoleUser {
			users = append(users, message85Text(m))
		}
	}
	if strings.Join(users, "|") != "target history|after switch" {
		t.Fatal(users)
	}
}
