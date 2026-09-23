//go:build darwin || linux

package codingagent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/terminaltest"
	"github.com/nankedr/pig/tui"
)

func TestInteractiveBashSDK122(t *testing.T) {
	for _, exit := range []string{"interrupt", "quit", "stop"} {
		t.Run(exit, func(t *testing.T) {
			runtime := interactiveRuntime(t)
			tty := terminaltest.Open(t)
			terminal := tui.NewProcessTerminal(tty.Slave, tty.Slave)
			mode := codingagent.NewInteractiveMode(runtime, codingagent.InteractiveModeOptions{Terminal: terminal})
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() { done <- mode.Run(ctx) }()
			t.Cleanup(func() { cancel(); _ = mode.Stop() })
			tty.Wait(t, "> ")
			marker := filepath.Join(t.TempDir(), "survivor")
			tty.Send(t, fmt.Sprintf("!printf 'BEFORE_%%s\\n' CANCEL; (sleep 1 && touch '%s') & wait\r", marker))
			tty.Wait(t, "BEFORE_CANCEL")
			old := runtime.Session().SessionManager()
			switch exit {
			case "interrupt":
				tty.Send(t, "\x1b[27u")
				tty.Wait(t, "(cancelled)")
				tty.Send(t, "after question\r")
				tty.Wait(t, "FIRST_DONE")
				tty.Send(t, "\x04")
			case "quit":
				tty.Send(t, "/quit\r")
			case "stop":
				if err := mode.Stop(); err != nil {
					t.Fatal(err)
				}

			}
			if err := awaitInteractive(t, done); err != nil {
				t.Fatal(err)
			}
			if !tty.Restored(t) {
				t.Fatal("terminal not restored")
			}
			select {
			case <-terminal.Done():
			default:
				t.Fatal("reader not stopped")
			}
			var bash []map[string]any
			for _, message := range old.BuildSessionContext().Messages {
				if message.MessageRole() == "bashExecution" {
					data, _ := json.Marshal(message)
					var m map[string]any
					_ = json.Unmarshal(data, &m)
					bash = append(bash, m)
				}
			}
			if len(bash) != 1 || bash[0]["cancelled"] != true || !strings.Contains(bash[0]["output"].(string), "BEFORE_CANCEL") {
				t.Fatalf("lost partial Bash: %v", bash)
			}
			time.Sleep(1100 * time.Millisecond)
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatal("descendant survived cancellation", err)
			}
		})
	}
}

func TestInteractiveBashFailureRetention122(t *testing.T) {
	runtime := interactiveRuntime(t)
	if err := runtime.Session().SettingsManager().SetShellPath(filepath.Join(t.TempDir(), "missing-shell")); err != nil {
		t.Fatal(err)
	}
	tty := terminaltest.Open(t)
	mode := codingagent.NewInteractiveMode(runtime, codingagent.InteractiveModeOptions{Terminal: tui.NewProcessTerminal(tty.Slave, tty.Slave)})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- mode.Run(ctx) }()
	t.Cleanup(func() { cancel(); _ = mode.Stop() })
	tty.Wait(t, "> ")
	tty.Send(t, "!echo retained-failure\r")
	tty.Wait(t, "Bash command failed:")
	tty.Send(t, "after question\r")
	tty.Wait(t, "FIRST_DONE")
	if !strings.Contains(tty.ScreenText(), "$ echo retained-failure") {
		t.Fatal("failed command disappeared from transcript")
	}
	tty.Send(t, "\x04")
	if err := awaitInteractive(t, done); err != nil {
		t.Fatal(err)
	}
}
