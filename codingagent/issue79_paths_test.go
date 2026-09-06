package codingagent_test

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/nankedr/pig/codingagent"
)

func TestEditToolDefinitionAndHostPaths(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "workspace")
	if err := os.Mkdir(cwd, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", root)
	t.Setenv("USERPROFILE", root)
	target := filepath.Join(root, "space name.txt")
	if err := os.Symlink(target, filepath.Join(cwd, "alias")); err != nil {
		t.Fatal(err)
	}
	for _, base := range []string{cwd, "~/workspace", (&url.URL{Scheme: "file", Path: cwd}).String()} {
		definition, err := codingagent.CreateEditToolDefinition(base)
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{target, "alias", "../space name.txt", "~/space name.txt", "@../space\u202fname.txt", (&url.URL{Scheme: "file", Path: target}).String()} {
			if err := os.WriteFile(target, []byte("before"), 0600); err != nil {
				t.Fatal(err)
			}
			args, err := definition.PrepareArguments(map[string]any{"path": path, "oldText": "before", "newText": "after"})
			if err != nil {
				t.Fatal(err)
			}
			result, err := definition.Execute(context.Background(), "edit", args, nil)
			if err != nil || result.Details == nil {
				t.Fatalf("definition %q/%q: %#v %v", base, path, result, err)
			}
			back := runReadTool(t, context.Background(), cwd, map[string]any{"path": target})
			if back.IsError || readToolResultText(t, back) != "after" {
				t.Fatalf("read: %#v", back)
			}
		}
	}
	definition, err := codingagent.CreateEditToolDefinition(cwd)
	if err != nil {
		t.Fatal(err)
	}
	_, err = definition.Execute(context.Background(), "missing", editArgs("missing", "a", "b"), nil)
	if err == nil || err.Error() != "Could not edit file: missing. Error code: ENOENT." {
		t.Fatalf("missing: %v", err)
	}
	if err := os.Chmod(target, 0444); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(target, 0600)
	_, err = definition.Execute(context.Background(), "readonly", editArgs(target, "after", "bad"), nil)
	if err == nil || err.Error() != "Could not edit file: "+target+". Error code: EACCES." {
		t.Fatalf("permissions: %v", err)
	}
}

func TestEditToolPreCanceledAndDefinitionValidation(t *testing.T) {
	called := false
	definition, err := codingagent.CreateEditToolDefinition(t.TempDir(), codingagent.EditToolOptions{Operations: editTestOperations{access: func(context.Context, string) error { called = true; return nil }}})
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range []any{nil, "bad", map[string]any{"edits": []any{}}, map[string]any{"path": "target", "edits": []any{nil}}, map[string]any{"path": "target", "edits": []any{map[string]any{"oldText": 3, "newText": "bad"}}}} {
		if _, err := definition.Execute(context.Background(), "invalid", args, nil); err == nil {
			t.Fatalf("invalid definition accepted %#v", args)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := definition.Execute(ctx, "cancel", editArgs("target", "a", "b"), nil)
	if !errors.Is(err, context.Canceled) || called || len(result.Content) > 0 {
		t.Fatalf("pre-canceled: %#v %v called=%v", result, err, called)
	}
	for _, input := range []any{nil, "bad", map[string]any{"path": "target", "edits": "not json"}} {
		prepared, err := definition.PrepareArguments(input)
		if err != nil || !reflect.DeepEqual(prepared, input) {
			t.Fatalf("prepare malformed: %#v %v", prepared, err)
		}
	}
}

func TestEditToolHeadlessExplicitSelection(t *testing.T) {
	for _, test := range []struct{ names, excluded, want []string }{
		{want: []string{"read"}}, {names: []string{"edit"}, want: []string{"edit"}},
		{names: []string{"read", "edit", "write"}, want: []string{"read", "edit", "write"}},
		{names: []string{"read", "edit"}, excluded: []string{"edit"}, want: []string{"read"}},
	} {
		settings, err := codingagent.NewInMemorySettingsManager(codingagent.Settings{})
		if err != nil {
			t.Fatal(err)
		}
		runtime, err := codingagent.CreateHeadlessSession(context.Background(), codingagent.CreateHeadlessSessionOptions{CWD: t.TempDir(), SettingsManager: settings, SessionManager: codingagent.NewInMemorySessionManager(""), Provider: "deepseek", Model: "deepseek-v4-flash", Tools: test.names, ExcludeTools: test.excluded})
		if err != nil {
			t.Fatal(err)
		}
		if got := runtime.Session().GetActiveToolNames(); !reflect.DeepEqual(got, test.want) {
			t.Fatalf("tools: %v want %v", got, test.want)
		}
		if strings.Contains(runtime.Session().SystemPrompt(), "Use edit for precise changes") != slices.Contains(test.want, "edit") {
			t.Fatal("prompt does not match tools")
		}
		runtime.Dispose(context.Background())
	}
}
