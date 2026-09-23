package tui_test

import (
	"context"
	"testing"
	"time"

	"github.com/nankedr/pig/tui"
)

type synchronousTerminal123 struct{ tui.Terminal }

func (*synchronousTerminal123) Start(_ func(string), resize func()) error { resize(); return nil }
func (*synchronousTerminal123) Stop() error                               { return nil }
func (*synchronousTerminal123) Write(string) error                        { return nil }
func (*synchronousTerminal123) Rows() (int, error)                        { return 24, nil }
func (*synchronousTerminal123) Columns() (int, error)                     { return 80, nil }
func (*synchronousTerminal123) KittyProtocolActive() (bool, error)        { return false, nil }
func (*synchronousTerminal123) DrainInput(context.Context, time.Duration, time.Duration) error {
	return nil
}

func TestExternalEditorSynchronousTerminal123(t *testing.T) {
	ui := tui.NewTextUI(&synchronousTerminal123{})
	if err := ui.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		done <- ui.EditExternally(context.Background(), func(context.Context, string) (string, error) { return "saved", nil })
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("terminal restart deadlocked in synchronous resize callback")
	}
	if err := ui.Stop(); err != nil {
		t.Fatal(err)
	}
}
