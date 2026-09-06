package codingagent_test

import (
	"context"
	"github.com/nankedr/pig/codingagent"
	"os"
	"path/filepath"
	"testing"
)

func TestProjectSettingsLoadOnlyAfterTrustAndRevoke(t *testing.T) {
	dir := t.TempDir()
	cwd := t.TempDir()
	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(dir, "settings.json"), `{"defaultModel":"global","retry":{"provider":{"maxRetries":7,"maxRetryDelayMs":42}},"skills":["global"]}`)
	write(filepath.Join(cwd, ".pig", "settings.json"), `{"defaultModel":"project","retry":{"provider":{"maxRetries":2}},"skills":["project"]}`)
	m, err := codingagent.NewSettingsManager(cwd, &dir)
	if err != nil {
		t.Fatal(err)
	}
	model, err := m.GetDefaultModel()
	if err != nil || model != "global" {
		t.Fatalf("before trust: %s %v", model, err)
	}
	if err = m.SetProjectTrusted(true); err != nil {
		t.Fatal(err)
	}
	model, err = m.GetDefaultModel()
	if err != nil || model != "project" {
		t.Fatalf("trusted: %s %v", model, err)
	}
	retry, err := m.GetProviderRetrySettings()
	if err != nil || retry.MaxRetries == nil || *retry.MaxRetries != 2 || retry.MaxRetryDelayMS == nil || *retry.MaxRetryDelayMS != 42 {
		t.Fatalf("deep merge: %+v %v", retry, err)
	}
	paths, err := m.GetSkillPaths()
	if err != nil || len(paths) != 1 || paths[0] != "project" {
		t.Fatalf("arrays: %v %v", paths, err)
	}
	if err = m.SetDefaultModel("changed-global"); err != nil {
		t.Fatal(err)
	}
	model, _ = m.GetDefaultModel()
	if model != "project" {
		t.Fatalf("global save lost project: %s", model)
	}
	if err = m.SetProjectTrusted(false); err != nil {
		t.Fatal(err)
	}
	model, _ = m.GetDefaultModel()
	if model != "changed-global" {
		t.Fatalf("revoked: %s", model)
	}
	write(filepath.Join(cwd, ".pig", "settings.json"), `invalid`)
	if err = m.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	diagnostics, _ := m.DrainErrors()
	if len(diagnostics) != 0 {
		t.Fatalf("untrusted project was parsed: %v", diagnostics)
	}
}
