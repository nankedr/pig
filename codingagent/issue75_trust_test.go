package codingagent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/nankedr/pig/codingagent"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

func TestProjectTrustStoreConcurrentProcessesAndPermissions(t *testing.T) {
	if dir := os.Getenv("PIG_TEST_TRUST_DIR"); dir != "" {
		if err := codingagent.NewProjectTrustStore(dir).Set(context.Background(), os.Getenv("PIG_TEST_TRUST_PATH"), codingagent.ProjectTrustDecisionTrusted()); err != nil {
			t.Fatal(err)
		}
		return
	}
	dir := filepath.Join(t.TempDir(), "agent")
	parent := t.TempDir()
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			command := exec.Command(os.Args[0], "-test.run=^TestProjectTrustStoreConcurrentProcessesAndPermissions$")
			command.Env = append(os.Environ(), "PIG_TEST_TRUST_DIR="+dir, "PIG_TEST_TRUST_PATH="+filepath.Join(parent, fmt.Sprint(i)))
			if output, err := command.CombinedOutput(); err != nil {
				t.Errorf("writer: %v %s", err, output)
			}
		}(i)
	}
	wg.Wait()
	store := codingagent.NewProjectTrustStore(dir)
	for i := 0; i < 5; i++ {
		d, err := store.Get(context.Background(), filepath.Join(parent, fmt.Sprint(i)))
		if err != nil || d == nil || !*d {
			t.Fatalf("lost decision %d: %v %v", i, d, err)
		}
	}
	for path, want := range map[string]os.FileMode{dir: 0700, filepath.Join(dir, "trust.json"): 0600} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != want {
			t.Fatalf("permission %s: %v %v", path, info, err)
		}
	}
	lock := filepath.Join(dir, "trust.json.lock")
	if err := os.Mkdir(lock, 0700); err != nil {
		t.Fatal(err)
	}
	if err := store.Set(context.Background(), parent, codingagent.ProjectTrustDecisionUntrusted()); err == nil {
		t.Fatal("write ignored held lock")
	}
	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	d, err := store.Get(context.Background(), parent)
	if err != nil || d != nil {
		t.Fatalf("failed locked write changed decision: %v %v", d, err)
	}
}

func TestProjectTrustCanonicalPathsRejectCorruptionAndIgnorePi(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	cwd := filepath.Join(dir, "project")
	if err := os.Mkdir(cwd, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "alias")
	if err := os.Symlink(cwd, link); err != nil {
		t.Skip(err)
	}
	piDir := filepath.Join(dir, ".pi", "agent")
	if err := os.MkdirAll(piDir, 0700); err != nil {
		t.Fatal(err)
	}
	real, err := filepath.EvalSymlinks(cwd)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(map[string]bool{real: true})
	if err := os.WriteFile(filepath.Join(piDir, "trust.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	agentDir := filepath.Join(dir, ".pig", "agent")
	store := codingagent.NewProjectTrustStore(agentDir)
	ctx := context.Background()
	if d, err := store.Get(ctx, cwd); err != nil || d != nil {
		t.Fatalf("inherited Pi trust: %v %v", d, err)
	}
	if err := store.Set(ctx, link, codingagent.ProjectTrustDecisionUntrusted()); err != nil {
		t.Fatal(err)
	}
	entry, err := store.GetEntry(ctx, cwd)
	if err != nil || entry == nil || entry.Decision || entry.Path != real {
		t.Fatalf("canonical decision: %+v %v", entry, err)
	}
	for _, invalid := range []string{`{`, `[]`, `null`, `{"path":"yes"}`, `{"path":1}`} {
		path := filepath.Join(agentDir, "trust.json")
		if err := os.WriteFile(path, []byte(invalid), 0600); err != nil {
			t.Fatal(err)
		}
		if _, err := store.Get(ctx, cwd); err == nil {
			t.Fatalf("accepted invalid trust %s", invalid)
		}
		if err := store.Set(ctx, cwd, codingagent.ProjectTrustDecisionTrusted()); err == nil {
			t.Fatal("overwrote corrupt trust")
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != invalid {
			t.Fatal("corrupt trust changed")
		}
	}
}

func TestProjectTrustResourceExistenceScope(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	cwd := filepath.Join(dir, "project")
	if err := os.Mkdir(cwd, 0700); err != nil {
		t.Fatal(err)
	}
	probe := func(path string, want bool) {
		t.Helper()
		got, err := codingagent.HasTrustRequiringProjectResources(context.Background(), path)
		if err != nil || got != want {
			t.Fatalf("probe %s=%v want %v: %v", path, got, want, err)
		}
	}
	for _, name := range []string{"settings.json", "extensions", "skills", "prompts", "themes", "SYSTEM.md", "APPEND_SYSTEM.md"} {
		path := filepath.Join(cwd, ".pig", name)
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
		probe(cwd, true)
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		probe(cwd, false)
	}
	for _, path := range []string{filepath.Join(dir, ".agents", "skills"), filepath.Join(dir, ".pig", "settings.json"), filepath.Join(cwd, ".pi", "settings.json")} {
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	probe(cwd, false)
	child := filepath.Join(cwd, "child")
	if err := os.Mkdir(child, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(cwd, ".agents", "skills"), 0700); err != nil {
		t.Fatal(err)
	}
	probe(child, true)
}
