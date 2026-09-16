package codingagent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// ExtensionDiscoveryResult describes local candidates, never loaded extensions.
// Enabled is selection by configuration, not execution status.
type ExtensionDiscoveryResult struct {
	Entries     []ResolvedResource
	Diagnostics []ResourceDiagnostic
}

func (l *DefaultResourceLoader) GetExtensionDiscovery() (ExtensionDiscoveryResult, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.cwd == "" {
		return ExtensionDiscoveryResult{}, notImplemented("DefaultResourceLoader.GetExtensionDiscovery")
	}
	return ExtensionDiscoveryResult{Entries: append([]ResolvedResource{}, l.extensionDiscovery.Entries...), Diagnostics: slices.Clone(l.extensionDiscovery.Diagnostics)}, nil
}

func localExtensionIndex(dir string) string {
	for _, name := range []string{"index.ts", "index.js"} {
		path := filepath.Join(dir, name)
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			return path
		}
	}
	return ""
}

func localExtensionFiles(path string) []string {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	if info.Mode().IsRegular() {
		return []string{path}
	}
	if !info.IsDir() {
		return nil
	}
	if index := localExtensionIndex(path); index != "" {
		return []string{index}
	}
	ignores := []string{}
	for _, name := range []string{".gitignore", ".ignore", ".fdignore"} {
		p := filepath.Join(path, name)
		if stat, err := os.Stat(p); err != nil || !stat.Mode().IsRegular() {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			line = strings.TrimSpace(line)
			if line != "" && !strings.HasPrefix(line, "#") {
				ignores = append(ignores, line)
			}
		}
	}
	entries, _ := os.ReadDir(path)
	files := []string{}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") || name == "node_modules" {
			continue
		}
		full := filepath.Join(path, name)
		stat, err := os.Stat(full)
		if err != nil {
			continue
		}
		ignored := false
		for _, pattern := range ignores {
			include := strings.HasPrefix(pattern, "!")
			pattern = strings.TrimPrefix(strings.TrimPrefix(pattern, "!"), "/")
			if strings.HasSuffix(pattern, "/") && !stat.IsDir() {
				continue
			}
			if templateMatch(strings.TrimSuffix(pattern, "/"), name) {
				ignored = !include
			}
		}
		if ignored {
			continue
		}
		if stat.IsDir() {
			if index := localExtensionIndex(full); index != "" {
				files = append(files, index)
			}
		} else if stat.Mode().IsRegular() && (strings.HasSuffix(name, ".ts") || strings.HasSuffix(name, ".js")) {
			files = append(files, full)
		}
	}
	return files
}

func (l *DefaultResourceLoader) discoverLocalExtensions(ctx context.Context, settings *SettingsManager, trusted bool) (ExtensionDiscoveryResult, error) {
	result := ExtensionDiscoveryResult{Entries: []ResolvedResource{}, Diagnostics: []ResourceDiagnostic{}}
	seen := map[string]bool{}
	add := func(path string, metadata PathMetadata, enabled bool) {
		canonical, err := filepath.EvalSymlinks(path)
		if err != nil {
			canonical = path
		}
		if seen[canonical] {
			return
		}
		seen[canonical] = true
		result.Entries = append(result.Entries, ResolvedResource{Path: path, Metadata: metadata, Enabled: enabled})
		state := "discovered"
		if !enabled {
			state = "disabled"
		}
		result.Diagnostics = append(result.Diagnostics, ResourceDiagnostic{Type: "warning", Path: path, Message: fmt.Sprintf("Extension %s (%s:%s): %s; not executed (extension runtime not implemented)", path, metadata.Scope, metadata.Source, state)})
	}
	for _, input := range l.extensionPaths {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		if strings.HasPrefix(input, "npm:") || strings.HasPrefix(input, "git:") || strings.Contains(input, "://") || strings.HasPrefix(input, "git@") {
			result.Diagnostics = append(result.Diagnostics, ResourceDiagnostic{Type: "warning", Path: input, Message: "Extension source " + input + ": not executed (package sources not implemented)"})
			continue
		}
		path := templatePath(input, l.cwd)
		files := localExtensionFiles(path)
		if len(files) == 0 {
			result.Diagnostics = append(result.Diagnostics, ResourceDiagnostic{Type: "warning", Path: path, Message: "Extension path " + path + ": no local entry; not executed (extension runtime not implemented)"})
		}
		base := path
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			base = filepath.Dir(path)
		}
		for _, file := range files {
			add(file, PathMetadata{Source: input, Scope: SourceScopeTemporary, Origin: ResourceOriginTopLevel, BaseDir: base}, true)
		}
	}
	if l.noExtensions {
		return result, ctx.Err()
	}
	global, err := settings.GetGlobalSettings()
	if err != nil {
		return result, err
	}
	project, err := settings.GetProjectSettings()
	if err != nil {
		return result, err
	}
	scopes := []struct {
		base    string
		scope   SourceScope
		paths   []string
		allowed bool
	}{
		{filepath.Join(l.cwd, ".pig"), SourceScopeProject, project.Extensions, trusted},
		{l.agentDir, SourceScopeUser, global.Extensions, true},
	}
	candidates := []ResolvedResource{}
	seenRaw := map[string]bool{}
	for _, auto := range []bool{false, true} {
		for _, scope := range scopes {
			if !scope.allowed {
				continue
			}
			if err := ctx.Err(); err != nil {
				return result, err
			}
			paths := scope.paths
			metadata := PathMetadata{Source: "local", Scope: scope.scope, Origin: ResourceOriginTopLevel}
			if auto {
				paths = []string{"extensions"}
				metadata.Source, metadata.BaseDir = "auto", scope.base
			}
			for _, p := range paths {
				if strings.ContainsAny(p, "*?") || strings.HasPrefix(p, "!") || strings.HasPrefix(p, "+") || strings.HasPrefix(p, "-") {
					continue
				}
				for _, file := range localExtensionFiles(templatePath(p, scope.base)) {
					if seenRaw[file] {
						continue
					}
					seenRaw[file] = true
					candidates = append(candidates, ResolvedResource{Path: file, Metadata: metadata, Enabled: templateEnabled(file, scope.base, scope.paths, auto)})
				}
			}
		}
	}
	slices.SortStableFunc(candidates, func(a, b ResolvedResource) int {
		return strings.Compare(string(a.Metadata.Scope), string(b.Metadata.Scope))
	})
	for _, entry := range candidates {
		add(entry.Path, entry.Metadata, entry.Enabled)
	}
	return result, ctx.Err()
}

func reportCLIExtensionDiscovery(ctx context.Context, parsed Args) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	agentDir, err := GetAgentDir()
	if err != nil {
		return err
	}
	settings, err := NewSettingsManager(cwd, &agentDir)
	if err != nil {
		return err
	}
	if err = prepareHeadlessProjectSettings(ctx, cwd, agentDir, settings, parsed.ProjectTrustOverride); err != nil {
		return err
	}
	trusted, err := settings.IsProjectTrusted()
	if err != nil {
		return err
	}
	loader, err := NewDefaultResourceLoader(DefaultResourceLoaderOptions{CWD: cwd, AgentDir: agentDir, AdditionalExtensionPaths: parsed.Extensions, NoExtensions: parsed.NoExtensions})
	if err != nil {
		return err
	}
	result, err := loader.discoverLocalExtensions(ctx, settings, trusted)
	if err != nil {
		return err
	}
	for _, d := range result.Diagnostics {
		fmt.Fprintln(os.Stderr, "Warning: "+d.Message)
	}
	return notImplemented("extension.discovery")
}
