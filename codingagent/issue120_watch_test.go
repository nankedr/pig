package codingagent_test

import (
	"context"
	"github.com/nankedr/pig/codingagent"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestThemeWatchDebounce120(t *testing.T) {
	data, err := os.ReadFile("themes/dark.json")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "local.json")
	data = []byte(strings.Replace(string(data), `"name": "dark"`, `"name": "local"`, 1))
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	theme, err := codingagent.LoadThemeFromPath(path)
	if err != nil {
		t.Fatal(err)
	}
	name := theme.Name
	settings, _ := codingagent.NewInMemorySettingsManager(codingagent.Settings{Theme: &name})
	c := codingagent.NewThemeController(settings, codingagent.ThemeLoadResult{Themes: []*codingagent.Theme{theme}})
	if err := c.ApplySettings(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	changes := make(chan error, 20)
	done := make(chan struct{})
	go func() { defer close(done); c.Watch(ctx, func(err error) { changes <- err }) }()
	time.Sleep(100 * time.Millisecond)
	for i := 0; i < 8; i++ {
		data = append(data, ' ')
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		time.Sleep(30 * time.Millisecond)
		select {
		case err := <-changes:
			t.Fatalf("reload before writes settled: %v", err)
		default:
		}
	}
	select {
	case err := <-changes:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no debounced reload")
	}
	if err := c.SetTheme("light"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-changes:
		t.Fatalf("old file notified after theme switch: %v", err)
	case <-time.After(180 * time.Millisecond):
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("watcher not released")
	}
}
