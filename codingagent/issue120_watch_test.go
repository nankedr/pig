package codingagent_test

import (
	"context"
	"encoding/json"
	"github.com/nankedr/pig/codingagent"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	data = append(data, ' ')
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	changes := make(chan error, 20)
	done := make(chan struct{})
	go func() { defer close(done); c.Watch(ctx, func(err error) { changes <- err }) }()
	select {
	case err := <-changes:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("modification before Watch was lost")
	}
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

func TestThemeWatchRebindWindow120(t *testing.T) {
	raw, err := os.ReadFile("themes/dark.json")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	write := func(name, accent string) string {
		t.Helper()
		var doc map[string]any
		if err := json.Unmarshal(raw, &doc); err != nil {
			t.Fatal(err)
		}
		doc["name"] = name
		doc["colors"].(map[string]any)["accent"] = accent
		data, err := json.Marshal(doc)
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(root, name, "theme.json")
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	a, err := codingagent.LoadThemeFromPath(write("a", "#111111"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := codingagent.LoadThemeFromPath(write("b", "#222222"))
	if err != nil {
		t.Fatal(err)
	}
	name := "a"
	settings, _ := codingagent.NewInMemorySettingsManager(codingagent.Settings{Theme: &name})
	c := codingagent.NewThemeController(settings, codingagent.ThemeLoadResult{Themes: []*codingagent.Theme{a, b}})
	if err := c.ApplySettings(); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	entered, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var once sync.Once
	changes := make(chan error, 10)
	go func() {
		defer close(done)
		c.Watch(ctx, func(err error) { once.Do(func() { close(entered); <-release }); changes <- err })
	}()
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("watcher did not stop")
		}
	}()
	write("a", "#333333")
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("initial modification was lost")
	}
	if err := c.SetTheme("b"); err != nil {
		t.Fatal(err)
	}
	write("b", "#444444")
	close(release)
	for i := 0; i < 2; i++ {
		select {
		case err := <-changes:
			if err != nil {
				t.Fatal(err)
			}
		case <-time.After(2 * time.Second):
			t.Fatal("rebind modification was lost")
		}
	}
	if got := c.Current().ResolvedColors()["accent"]; got != "#444444" {
		t.Fatal("stale theme", got)
	}
}
