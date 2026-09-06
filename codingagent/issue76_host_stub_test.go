//go:build !darwin && !linux

package codingagent_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func TestCredential76UnsupportedHostIsInert(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "missing")
	store, err := codingagent.NewAuthStorage(filepath.Join(dir, "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Modify(context.Background(), "deepseek", func(context.Context, ai.Credential) (ai.Credential, error) {
		t.Fatal("unsupported host ran callback")
		return nil, nil
	}, ai.AuthOperationOptions{})
	if !errors.Is(err, codingagent.ErrNotImplemented) {
		t.Fatal(err)
	}
	if err := store.Delete(context.Background(), "deepseek", ai.AuthOperationOptions{}); !errors.Is(err, codingagent.ErrNotImplemented) {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("unsupported host created credential state")
	}
}
