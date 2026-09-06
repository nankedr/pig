package codingagent_test

import (
	"context"
	"github.com/nankedr/pig/codingagent"
	"os"
	"path/filepath"
	"testing"
)

func TestProjectContextLayeringAndWorktreeDedup(t *testing.T) {
	dir := t.TempDir()
	agentDir := filepath.Join(dir, "agent")
	repo := filepath.Join(dir, "repo")
	worktree := filepath.Join(repo, "trees", "feature")
	cwd := filepath.Join(worktree, "src")
	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(agentDir, "AGENTS.override.md"), "global override")
	write(filepath.Join(agentDir, "AGENTS.md"), "ignored")
	write(filepath.Join(repo, "AGENTS.md"), "shadowed main")
	write(filepath.Join(worktree, "AGENTS.md"), "worktree")
	write(filepath.Join(cwd, "CLAUDE.md"), "leaf")
	gitDir := filepath.Join(repo, ".git", "worktrees", "feature")
	write(filepath.Join(gitDir, "commondir"), "../..\n")
	write(filepath.Join(worktree, ".git"), "gitdir: "+gitDir+"\n")
	files, err := codingagent.LoadProjectContextFiles(context.Background(), cwd, agentDir)
	if err != nil {
		t.Fatal(err)
	}
	content := []string{}
	for _, file := range files {
		if file.Content == "ignored" || file.Content == "shadowed main" {
			t.Fatalf("loaded shadowed file: %s", file.Path)
		}
		if file.Content == "global override" || file.Content == "worktree" || file.Content == "leaf" {
			content = append(content, file.Content)
		}
	}
	if len(content) != 3 || content[0] != "global override" || content[1] != "worktree" || content[2] != "leaf" {
		t.Fatalf("context order: %v", content)
	}
}
