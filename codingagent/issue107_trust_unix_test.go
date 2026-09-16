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

	"github.com/nankedr/pig/codingagent"
)

func TestLocalExtensionsNoSensitiveReads(t *testing.T) {
	if os.Getenv("PIG_EXTENSION107_CHILD") == "1" {
		root := t.TempDir()
		cwd, dir := filepath.Join(root, "repo"), filepath.Join(root, "agent")
		for _, p := range []string{filepath.Join(cwd, ".pig/extensions"), filepath.Join(dir, "extensions/plugin")} {
			if err := os.MkdirAll(p, 0700); err != nil {
				t.Fatal(err)
			}
		}
		for _, p := range []string{filepath.Join(cwd, ".pig/settings.json"), filepath.Join(cwd, ".pig/extensions/.gitignore"), filepath.Join(dir, "extensions/package.json"), filepath.Join(dir, "extensions/plugin/package.json"), filepath.Join(dir, "extensions/effect.ts")} {
			if err := syscall.Mkfifo(p, 0600); err != nil {
				t.Fatal(err)
			}
		}
		context100Write(t, filepath.Join(dir, "extensions/plugin/index.js"), "throw Error('not imported')")
		loader, err := codingagent.NewDefaultResourceLoader(codingagent.DefaultResourceLoaderOptions{CWD: cwd, AgentDir: dir})
		if err != nil {
			t.Fatal(err)
		}
		if err = loader.Reload(context.Background()); err != nil {
			t.Fatal(err)
		}
		got, err := loader.GetExtensionDiscovery()
		if err != nil || len(got.Entries) != 1 || got.Entries[0].Metadata.Scope != codingagent.SourceScopeUser {
			t.Fatalf("%+v %v", got, err)
		}
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestLocalExtensionsNoSensitiveReads$", "-test.count=1")
	cmd.Env = append(os.Environ(), "PIG_EXTENSION107_CHILD=1")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("sensitive read or blocked FIFO: %v %v %s", ctx.Err(), err, out)
	}
}
