//go:build darwin && arm64

package codingagent_test

import (
	"context"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/terminaltest"
	"github.com/nankedr/pig/tui"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestImageWorkflowNativeClipboard136(t *testing.T) {
	if os.Getenv("PIG_REQUIRE_CLIPBOARD_IMAGE_NATIVE") != "1" {
		t.Skip("explicit native clipboard smoke; snapshots and restores the user's pasteboard")
	}
	helper := filepath.Join(issue32RepoRoot(t), "parity/terminal/clipboard-image.js")
	snapshot := filepath.Join(t.TempDir(), "pasteboard.json")
	run := func(action, path string, extra ...string) {
		t.Helper()
		if _, err := exec.Command("/usr/bin/osascript", append([]string{"-l", "JavaScript", helper, action, path}, extra...)...).Output(); err != nil {
			t.Fatalf("clipboard fixture %s failed", action)
		}
	}
	run("save", snapshot)
	defer run("restore", snapshot)
	tty := terminaltest.Open(t)
	mode := codingagent.NewInteractiveMode(interactiveRuntime(t), codingagent.InteractiveModeOptions{Terminal: tui.NewProcessTerminal(tty.Slave, tty.Slave)})
	defer mode.Stop()
	done := make(chan error, 1)
	go func() { done <- mode.Run(context.Background()) }()
	tty.Wait(t, "> ")
	for _, fixture := range []struct{ file, uti, mime string }{{"user-image.png", "public.png", "image/png"}, {"tool-images/orientation-1.jpg", "public.jpeg", "image/jpeg"}, {"tool-images/canvas.gif", "com.compuserve.gif", "image/gif"}, {"tool-images/orientation-1.webp", "org.webmproject.webp", "image/webp"}} {
		run("fixture", filepath.Join(issue32RepoRoot(t), "parity/services", fixture.file), fixture.uti)
		tty.Send(t, "\x16")
		tty.Send(t, "/image list\r")
		tty.Wait(t, fixture.mime)
		tty.Send(t, "/image remove 1\r")
	}
	run("text", "CLIPBOARD_TEXT_FALLBACK_136")
	tty.Send(t, "\x16")
	tty.Wait(t, "CLIPBOARD_TEXT_FALLBACK_136")
	if err := mode.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := awaitInteractive(t, done); err != nil {
		t.Fatal(err)
	}
	if !tty.Restored(t) {
		t.Fatal("clipboard entry left terminal in raw mode")
	}
	t.Log("PASS darwin-arm64 NSPasteboard PNG/JPEG/GIF/WebP -> Ctrl+V pending attachment -> remove; text fallback; original pasteboard restored by fixture")
}
