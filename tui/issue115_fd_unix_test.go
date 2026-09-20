//go:build darwin || linux

package tui_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nankedr/pig/tui"
)

func TestAutocompleteFDCancellation115(t *testing.T) {
	root := t.TempDir()
	fd := filepath.Join(root, "fd")
	if err := os.WriteFile(fd, []byte("#!/bin/sh\nexec sleep 10\n"), 0755); err != nil {
		t.Fatal(err)
	}
	p := tui.NewCombinedAutocompleteProvider(nil, root, &fd)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, ok, err := p.GetSuggestions(ctx, []string{"@a"}, 0, 2, tui.AutocompleteOptions{})
	if ok || !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > time.Second {
		t.Fatalf("cancellation: ok=%v err=%v elapsed=%v", ok, err, time.Since(start))
	}
}
