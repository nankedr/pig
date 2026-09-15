package codingagent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf16"
)

func templatePath(path, base string) string {
	path = strings.TrimSpace(path)
	if strings.HasPrefix(path, "file://") {
		if resolved, err := resolveSessionPath(path); err == nil {
			return resolved
		}
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, strings.TrimPrefix(path, "~"))
		}
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(base, path)
	}
	return filepath.Clean(path)
}

func templateMatch(pattern, value string) bool {
	pattern, value = filepath.ToSlash(pattern), filepath.ToSlash(value)
	if start := strings.IndexByte(pattern, '{'); start >= 0 {
		if end := strings.IndexByte(pattern[start:], '}'); end >= 0 {
			for _, part := range strings.Split(pattern[start+1:start+end], ",") {
				if templateMatch(pattern[:start]+part+pattern[start+end+1:], value) {
					return true
				}
			}
			return false
		}
	}
	parts, values := strings.Split(pattern, "/"), strings.Split(value, "/")
	var match func(int, int) bool
	match = func(p, v int) bool {
		if p == len(parts) {
			return v == len(values)
		}
		if parts[p] == "**" {
			if match(p+1, v) {
				return true
			}
			return v < len(values) && !strings.HasPrefix(values[v], ".") && match(p, v+1)
		}
		if v == len(values) || strings.HasPrefix(values[v], ".") && !strings.HasPrefix(parts[p], ".") {
			return false
		}
		ok, _ := filepath.Match(parts[p], values[v])
		return ok && match(p+1, v+1)
	}
	return match(0, 0)
}

func templateFiles(path, mode string) []string {
	extension := ".md"
	if mode == "themes" {
		extension = ".json"
	}
	files := []string{}
	ancestors := map[string]bool{}
	var walk func(string, []string)
	walk = func(current string, ignores []string) {
		info, err := os.Stat(current)
		if err != nil {
			return
		}
		if info.Mode().IsRegular() {
			if strings.HasSuffix(current, extension) {
				files = append(files, current)
			}
			return
		}
		if !info.IsDir() {
			return
		}
		canonical, err := filepath.EvalSymlinks(current)
		if err != nil || ancestors[canonical] {
			return
		}
		ancestors[canonical] = true
		defer delete(ancestors, canonical)
		if mode != "explicit" {
			prefix, _ := filepath.Rel(path, current)
			for _, name := range []string{".gitignore", ".ignore", ".fdignore"} {
				p := filepath.Join(current, name)
				if stat, err := os.Stat(p); err != nil || !stat.Mode().IsRegular() {
					continue
				}
				data, err := os.ReadFile(p)
				if err != nil {
					continue
				}
				for _, line := range strings.Split(string(data), "\n") {
					line = strings.TrimSpace(line)
					if line == "" || strings.HasPrefix(line, "#") {
						continue
					}
					negated := strings.HasPrefix(line, "!")
					line = strings.TrimPrefix(strings.TrimPrefix(line, "!"), "/")
					if prefix != "." {
						line = filepath.ToSlash(prefix) + "/" + line
					}
					if negated {
						line = "!" + line
					}
					ignores = append(ignores, line)
				}
			}
		}
		entries, err := os.ReadDir(current)
		if err != nil {
			return
		}

		isIgnored := func(full string, stat os.FileInfo) bool {
			name := filepath.Base(full)
			rel, _ := filepath.Rel(path, full)
			rel = filepath.ToSlash(rel)
			ignored := false
			for _, pattern := range ignores {
				include := strings.HasPrefix(pattern, "!")
				pattern = strings.TrimPrefix(pattern, "!")
				if strings.HasSuffix(pattern, "/") {
					if !stat.IsDir() {
						continue
					}
					pattern = strings.TrimSuffix(pattern, "/")
				}
				target := rel
				if !strings.Contains(pattern, "/") {
					target = name
				}
				if templateMatch(pattern, target) {
					ignored = !include
				}
			}
			return ignored
		}
		if mode == "skills" || mode == "agents" {
			full := filepath.Join(current, "SKILL.md")
			if stat, err := os.Stat(full); err == nil && stat.Mode().IsRegular() && !isIgnored(full, stat) {
				files = append(files, full)
				return
			}
		}

		for _, entry := range entries {
			name := entry.Name()
			if mode != "explicit" && (strings.HasPrefix(name, ".") || name == "node_modules") {
				continue
			}
			full := filepath.Join(current, name)
			stat, err := os.Stat(full)
			if err != nil {
				continue
			}
			ignored := isIgnored(full, stat)
			if ignored {
				continue
			}
			if stat.IsDir() {
				if mode == "settings" || mode == "skills" || mode == "agents" {
					walk(full, append([]string{}, ignores...))
				}
			} else if stat.Mode().IsRegular() && strings.HasSuffix(name, extension) {
				if mode != "agents" && (mode != "skills" || current == path) || name == "SKILL.md" {
					files = append(files, full)
				}
			}
		}
	}
	walk(path, nil)
	return files
}

func templateEnabled(path, base string, patterns []string, auto bool) bool {
	rel, _ := filepath.Rel(base, path)
	exact := func(pattern string) bool { return templatePath(pattern, base) == path }
	matches := func(pattern string) bool {
		for _, candidate := range []string{rel, filepath.Base(path), path} {
			if templateMatch(pattern, candidate) {
				return true
			}
		}
		return false
	}
	enabled, hasInclude, included := true, false, false
	for _, pattern := range patterns {
		if strings.HasPrefix(pattern, "!") {
			if matches(pattern[1:]) {
				enabled = false
			}
		} else if !auto && strings.ContainsAny(pattern, "*?") && !strings.HasPrefix(pattern, "+") && !strings.HasPrefix(pattern, "-") {
			hasInclude = true
			included = included || matches(pattern)
		}
	}
	enabled = enabled && (!hasInclude || included)
	for _, pattern := range patterns {
		if strings.HasPrefix(pattern, "+") && exact(pattern[1:]) {
			enabled = true
		}
	}
	for _, pattern := range patterns {
		if strings.HasPrefix(pattern, "-") && exact(pattern[1:]) {
			enabled = false
		}
	}
	return enabled
}

func (l *DefaultResourceLoader) loadPromptTemplates(ctx context.Context, settings *SettingsManager, trusted bool) (PromptTemplateLoadResult, error) {
	result := PromptTemplateLoadResult{Prompts: []PromptTemplate{}, Diagnostics: []ResourceDiagnostic{}}
	type resource struct {
		path   string
		source SourceInfo
	}
	resources := []resource{}
	seenPaths := map[string]bool{}
	add := func(path string, source SourceInfo) {
		canonical, err := filepath.EvalSymlinks(path)
		if err != nil {
			canonical = path
		}
		if seenPaths[canonical] {
			return
		}
		seenPaths[canonical] = true
		source.Path = path
		resources = append(resources, resource{path, source})
	}
	if !l.noPromptTemplates {
		global, err := settings.GetGlobalSettings()
		if err != nil {
			return result, err
		}
		project, err := settings.GetProjectSettings()
		if err != nil {
			return result, err
		}
		for _, scope := range []struct {
			base    string
			scope   SourceScope
			paths   []string
			enabled bool
		}{{filepath.Join(l.cwd, ".pig"), SourceScopeProject, project.Prompts, trusted}, {l.agentDir, SourceScopeUser, global.Prompts, true}} {
			if !scope.enabled {
				continue
			}
			for _, p := range scope.paths {
				if strings.ContainsAny(p, "*?") || strings.HasPrefix(p, "!") || strings.HasPrefix(p, "+") || strings.HasPrefix(p, "-") {
					continue
				}
				for _, file := range templateFiles(templatePath(p, scope.base), "settings") {
					if templateEnabled(file, scope.base, scope.paths, false) {
						add(file, SourceInfo{Source: "local", Scope: scope.scope, Origin: ResourceOriginTopLevel})
					}
				}
			}
			for _, file := range templateFiles(filepath.Join(scope.base, "prompts"), "auto") {
				if templateEnabled(file, scope.base, scope.paths, true) {
					add(file, SourceInfo{Source: "auto", Scope: scope.scope, Origin: ResourceOriginTopLevel, BaseDir: scope.base})
				}
			}
		}
	}
	missing := []string{}
	for _, p := range l.templatePaths {
		path := templatePath(p, l.cwd)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			missing = append(missing, path)
		}
		canonical, err := filepath.EvalSymlinks(path)
		if err != nil {
			canonical = path
		}
		if seenPaths[canonical] {
			continue
		}
		seenPaths[canonical] = true
		for _, file := range templateFiles(path, "explicit") {
			source := SourceInfo{Source: "local", Scope: SourceScopeTemporary, Origin: ResourceOriginTopLevel, BaseDir: filepath.Dir(file)}
			for _, scope := range []struct {
				base  string
				scope SourceScope
			}{{filepath.Join(l.agentDir, "prompts"), SourceScopeUser}, {filepath.Join(l.cwd, ".pig/prompts"), SourceScopeProject}} {
				if file == scope.base || strings.HasPrefix(file, scope.base+string(filepath.Separator)) {
					source.Scope, source.BaseDir = scope.scope, scope.base
					break
				}
			}
			for _, existing := range resources {
				if existing.path == file {
					source = existing.source
					break
				}
			}
			source.Path = file
			resources = append(resources, resource{file, source})
		}
	}
	seen := map[string]PromptTemplate{}
	for _, r := range resources {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		data, err := os.ReadFile(r.path)
		if err != nil {
			continue
		}
		parsed, err := ParseFrontmatter(string(data))
		if err != nil {
			continue
		}
		description, _ := parsed.Frontmatter["description"].(string)
		if description == "" {
			for _, line := range strings.Split(parsed.Body, "\n") {
				if strings.TrimSpace(line) != "" {
					units := utf16.Encode([]rune(line))
					description = line
					if len(units) > 60 {
						description = string(utf16.Decode(units[:60])) + "..."
					}
					break
				}
			}
		}
		hint, _ := parsed.Frontmatter["argument-hint"].(string)
		prompt := PromptTemplate{Name: strings.TrimSuffix(filepath.Base(r.path), ".md"), Description: description, ArgumentHint: hint, Content: parsed.Body, FilePath: r.path, SourceInfo: r.source}
		if winner, ok := seen[prompt.Name]; ok {
			result.Diagnostics = append(result.Diagnostics, ResourceDiagnostic{Type: "collision", Message: fmt.Sprintf("name \"/%s\" collision", prompt.Name), Path: r.path, Collision: &ResourceCollision{ResourceType: "prompt", Name: prompt.Name, WinnerPath: winner.FilePath, LoserPath: r.path}})
		} else {
			seen[prompt.Name] = prompt
			result.Prompts = append(result.Prompts, prompt)
		}
	}
	for _, path := range missing {
		result.Diagnostics = append(result.Diagnostics, ResourceDiagnostic{Type: "error", Message: "Prompt template path does not exist", Path: path})
	}
	return result, ctx.Err()
}
