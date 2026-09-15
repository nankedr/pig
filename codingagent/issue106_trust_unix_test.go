//go:build !windows

package codingagent_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestSessionReloadPreTrustDoesNotReadProjectSettings(t *testing.T) {
	if os.Getenv("PIG_RELOAD106_CHILD") == "1" {
		s, _, _, cwd, dir := reload106Session(t)
		if err := os.MkdirAll(filepath.Join(cwd, ".pig"), 0700); err != nil {
			t.Fatal(err)
		}
		if err := syscall.Mkfifo(filepath.Join(cwd, ".pig/settings.json"), 0600); err != nil {
			t.Fatal(err)
		}
		context100Write(t, filepath.Join(dir, "settings.json"), `{"defaultProjectTrust":"always"}`)
		if err := s.Reload(context.Background()); err != nil {
			t.Fatal(err)
		}
		if trusted, _ := s.SettingsManager().IsProjectTrusted(); trusted {
			t.Fatal("reload changed established trust")
		}
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestSessionReloadPreTrustDoesNotReadProjectSettings$", "-test.count=1")
	cmd.Env = append(os.Environ(), "PIG_RELOAD106_CHILD=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("pre-trust reload read project settings: %v %v %s", ctx.Err(), err, out)
	}
}
