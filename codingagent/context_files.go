package codingagent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func contextFileFromDir(dir string) *AgentsFile {
	for _, name := range []string{"AGENTS.override.md", "AGENTS.md", "AGENTS.MD", "CLAUDE.md", "CLAUDE.MD"} {
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		return &AgentsFile{Path: path, Content: string(content)}
	}
	return nil
}

func shadowedContextFile(cwd string) string {
	for dir := cwd; ; dir = filepath.Dir(dir) {
		path := filepath.Join(dir, ".git")
		if info, err := os.Stat(path); err == nil {
			if info.IsDir() {
				return ""
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return ""
			}
			target, ok := strings.CutPrefix(strings.TrimSpace(string(data)), "gitdir: ")
			if !ok {
				return ""
			}
			if !filepath.IsAbs(target) {
				target = filepath.Join(dir, target)
			}
			common, err := os.ReadFile(filepath.Join(target, "commondir"))
			if err != nil {
				return ""
			}
			commonDir := strings.TrimSpace(string(common))
			if !filepath.IsAbs(commonDir) {
				commonDir = filepath.Join(target, commonDir)
			}
			commonDir, err = canonicalTrustPath(commonDir)
			if err != nil {
				return ""
			}
			main := filepath.Dir(commonDir)
			worktree, err := canonicalTrustPath(dir)
			if err != nil {
				return ""
			}
			mainGit, err := canonicalTrustPath(filepath.Join(main, ".git"))
			if err != nil || mainGit != commonDir || !strings.HasPrefix(worktree, main+string(filepath.Separator)) {
				return ""
			}
			if file := contextFileFromDir(dir); file != nil {
				return filepath.Join(main, filepath.Base(file.Path))
			}
			return ""
		}
		if filepath.Dir(dir) == dir {
			return ""
		}
	}
}

func LoadProjectContextFiles(ctx context.Context, cwd, agentDir string) ([]AgentsFile, error) {
	if ctx == nil {
		return nil, fmt.Errorf("context files context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cwd, err := resolveSessionPath(cwd)
	if err != nil {
		return nil, err
	}
	agentDir, err = resolveSessionPath(agentDir)
	if err != nil {
		return nil, err
	}
	files := []AgentsFile{}
	seen := map[string]bool{}
	if file := contextFileFromDir(agentDir); file != nil {
		files = append(files, *file)
		seen[file.Path] = true
	}
	shadowed := shadowedContextFile(cwd)
	ancestors := []AgentsFile{}
	for dir := cwd; ; dir = filepath.Dir(dir) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if file := contextFileFromDir(dir); file != nil && !seen[file.Path] {
			canonical, err := canonicalTrustPath(file.Path)
			if err != nil {
				return nil, err
			}
			if canonical != shadowed {
				ancestors = append(ancestors, *file)
				seen[file.Path] = true
			}
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}
	for i := len(ancestors) - 1; i >= 0; i-- {
		files = append(files, ancestors[i])
	}
	return files, nil
}
