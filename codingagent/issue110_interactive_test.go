//go:build darwin || linux

package codingagent_test

import (
	"context"
	"testing"
	"time"

	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/terminaltest"
	"github.com/nankedr/pig/tui"
)

func TestInteractiveSDKRestoresEditorHistory110(t *testing.T) {
	runtime := interactiveRuntime(t)
	prompt := "previous\n中文"
	if _, err := codingagent.RunHeadless(context.Background(), runtime, codingagent.HeadlessRunOptions{InitialMessage: &prompt}); err != nil {
		t.Fatal(err)
	}
	tty := terminaltest.Open(t)
	mode := codingagent.NewInteractiveMode(runtime, codingagent.InteractiveModeOptions{Terminal: tui.NewProcessTerminal(tty.Slave, tty.Slave)})
	defer mode.Stop()
	if err := mode.Init(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := mode.RenderInitialMessages(); err != nil {
		t.Fatal(err)
	}
	tty.Send(t, "discard")
	tty.Wait(t, "discard")
	if err := mode.ClearEditor(); err != nil {
		t.Fatal(err)
	}
	tty.Send(t, "\x1b[A\r")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	got, err := mode.GetUserInput(ctx)
	if err != nil || got != prompt {
		t.Fatalf("history input=%q err=%v", got, err)
	}
	if got := len(runtime.Session().Messages()); got != 2 {
		t.Fatalf("reading input made a Provider request: %d messages", got)
	}
}
