package codingagent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func themeFiles(path string) []string {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	if info.Mode().IsRegular() && strings.HasSuffix(path, ".json") {
		return []string{path}
	}
	if !info.IsDir() {
		return nil
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil
	}
	files := []string{}
	for _, entry := range entries {
		full := filepath.Join(path, entry.Name())
		if !strings.HasSuffix(full, ".json") {
			continue
		}
		if info, err := os.Stat(full); err == nil && info.Mode().IsRegular() {
			files = append(files, full)
		}
	}
	return files
}

func (l *DefaultResourceLoader) loadThemes(ctx context.Context, settings *SettingsManager, trusted bool) (ThemeLoadResult, error) {
	result := ThemeLoadResult{Themes: []*Theme{}, Diagnostics: []ResourceDiagnostic{}}
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
	if !l.noThemes {
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
		}{{filepath.Join(l.cwd, ".pig"), SourceScopeProject, project.Themes, trusted}, {l.agentDir, SourceScopeUser, global.Themes, true}} {
			if !scope.enabled {
				continue
			}
			for _, p := range scope.paths {
				if strings.ContainsAny(p, "*?") || strings.HasPrefix(p, "!") || strings.HasPrefix(p, "+") || strings.HasPrefix(p, "-") {
					continue
				}
				for _, file := range themeFiles(templatePath(p, scope.base)) {
					if templateEnabled(file, scope.base, scope.paths, false) {
						add(file, SourceInfo{Source: "local", Scope: scope.scope, Origin: ResourceOriginTopLevel})
					}
				}
			}
			for _, file := range templateFiles(filepath.Join(scope.base, "themes"), "themes") {
				if templateEnabled(file, scope.base, scope.paths, true) {
					add(file, SourceInfo{Source: "auto", Scope: scope.scope, Origin: ResourceOriginTopLevel, BaseDir: scope.base})
				}
			}
		}
	}
	missing := []string{}
	invalid := []string{}
	for _, p := range l.themePaths {
		path := templatePath(p, l.cwd)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			missing = append(missing, path)
		}
		if info, err := os.Stat(path); err == nil && !info.IsDir() && (!info.Mode().IsRegular() || !strings.HasSuffix(path, ".json")) {
			invalid = append(invalid, path)
		}
		canonical, err := filepath.EvalSymlinks(path)
		if err != nil {
			canonical = path
		}
		if seenPaths[canonical] {
			continue
		}
		seenPaths[canonical] = true
		for _, file := range themeFiles(path) {
			source := SourceInfo{Source: "local", Scope: SourceScopeTemporary, Origin: ResourceOriginTopLevel, BaseDir: filepath.Dir(file)}
			for _, scope := range []struct {
				base  string
				scope SourceScope
			}{{filepath.Join(l.agentDir, "themes"), SourceScopeUser}, {filepath.Join(l.cwd, ".pig/themes"), SourceScopeProject}} {
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
	seen := map[string]*Theme{}
	for _, r := range resources {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		theme, err := LoadThemeFromPath(r.path)
		if err != nil {
			result.Diagnostics = append(result.Diagnostics, ResourceDiagnostic{Type: "warning", Message: err.Error(), Path: r.path})
			continue
		}
		source := r.source
		theme.SourceInfo = &source

		if winner, ok := seen[theme.Name]; ok {
			result.Diagnostics = append(result.Diagnostics, ResourceDiagnostic{Type: "collision", Message: fmt.Sprintf("name \"%s\" collision", theme.Name), Path: r.path, Collision: &ResourceCollision{ResourceType: "theme", Name: theme.Name, WinnerPath: winner.SourcePath, LoserPath: r.path}})
		} else {
			seen[theme.Name] = theme
			result.Themes = append(result.Themes, theme)
		}
	}
	for _, path := range missing {
		result.Diagnostics = append(result.Diagnostics, ResourceDiagnostic{Type: "warning", Message: "theme path does not exist", Path: path})
	}
	for _, path := range invalid {
		result.Diagnostics = append(result.Diagnostics, ResourceDiagnostic{Type: "warning", Message: "theme path is not a json file", Path: path})
	}
	return result, ctx.Err()
}
