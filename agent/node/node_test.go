package node_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/agent/node"
)

func TestNodeExecutionEnvDefersWritesWithoutSideEffects(t *testing.T) {
	directory := t.TempDir()
	env := node.NewNodeExecutionEnv(directory)
	path := filepath.Join(directory, "created.txt")

	result := env.WriteFile(context.Background(), path, []byte("content"))
	if result.OK || !errors.Is(result.Error, agent.ErrNotImplemented) {
		t.Fatalf("WriteFile() = %#v", result)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("WriteFile changed the filesystem: %v", err)
	}
	if env.CWD() != directory {
		t.Fatalf("CWD() = %q, want %q", env.CWD(), directory)
	}
}
