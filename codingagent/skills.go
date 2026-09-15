package codingagent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf16"
)

func LoadSkillsFromDir(ctx context.Context, options LoadSkillsFromDirOptions) (LoadSkillsResult, error) {
	if ctx == nil {
		return LoadSkillsResult{}, fmt.Errorf("skill context must not be nil")
	}
	result := LoadSkillsResult{Skills: []Skill{}, Diagnostics: []ResourceDiagnostic{}}
	for _, path := range templateFiles(options.Dir, "skills") {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		loadSkillFile(path, options.Source, &result)
	}
	return result, ctx.Err()
}

var skillNamePattern = regexp.MustCompile(`^[a-z0-9-]+$`)

func loadSkillFile(path, source string, result *LoadSkillsResult) {
	warn := func(message string) {
		result.Diagnostics = append(result.Diagnostics, ResourceDiagnostic{Type: "warning", Message: message, Path: path})
	}
	info, err := os.Stat(path)
	if err != nil {
		warn(err.Error())
		return
	}
	if !info.Mode().IsRegular() {
		warn("skill path is not a markdown file")
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		warn(err.Error())
		return
	}
	parsed, err := ParseFrontmatter(string(data))
	if err != nil {
		warn(err.Error())
		return
	}
	description, ok := parsed.Frontmatter["description"].(string)
	if !ok && parsed.Frontmatter["description"] != nil {
		warn("frontmatter.description.trim is not a function")
		return
	}
	if strings.TrimSpace(description) == "" {
		warn("description is required")
	} else if n := len(utf16.Encode([]rune(description))); n > 1024 {
		warn(fmt.Sprintf("description exceeds 1024 characters (%d)", n))
	}
	name, ok := parsed.Frontmatter["name"].(string)
	if !ok && parsed.Frontmatter["name"] != nil {
		warn("name.startsWith is not a function")
		return
	}
	if name == "" {
		name = filepath.Base(filepath.Dir(path))
	}
	if n := len(utf16.Encode([]rune(name))); n > 64 {
		warn(fmt.Sprintf("name exceeds 64 characters (%d)", n))
	}
	if !skillNamePattern.MatchString(name) {
		warn("name contains invalid characters (must be lowercase a-z, 0-9, hyphens only)")
	}
	if strings.HasPrefix(name, "-") || strings.HasSuffix(name, "-") {
		warn("name must not start or end with a hyphen")
	}
	if strings.Contains(name, "--") {
		warn("name must not contain consecutive hyphens")
	}
	if strings.TrimSpace(description) == "" {
		return
	}
	metadata := PathMetadata{Source: source, BaseDir: filepath.Dir(path)}
	switch source {
	case "user":
		metadata.Source = "local"
		metadata.Scope = SourceScopeUser
	case "project":
		metadata.Source = "local"
		metadata.Scope = SourceScopeProject
	case "path":
		metadata.Source = "local"
	}
	disabled, _ := parsed.Frontmatter["disable-model-invocation"].(bool)
	result.Skills = append(result.Skills, Skill{Name: name, Description: description, FilePath: path, BaseDir: filepath.Dir(path), SourceInfo: CreateSyntheticSourceInfo(path, metadata), DisableModelInvocation: disabled})
}

func LoadSkills(ctx context.Context, cwd, agentDir string, paths []string, includeDefaults bool) (LoadSkillsResult, error) {
	result := LoadSkillsResult{Skills: []Skill{}, Diagnostics: []ResourceDiagnostic{}}
	if ctx == nil {
		return result, fmt.Errorf("skill context must not be nil")
	}
	cwd, err := resolveSessionPath(cwd)
	if err != nil {
		return result, err
	}
	if agentDir == "" {
		agentDir, err = GetAgentDir()
	} else {
		agentDir, err = resolveSessionPath(agentDir)
	}
	if err != nil {
		return result, err
	}
	if includeDefaults {
		paths = append([]string{filepath.Join(agentDir, "skills"), filepath.Join(cwd, ".pig/skills")}, paths...)
	}
	seen := map[string]Skill{}
	realPaths := map[string]bool{}
	collisions := []ResourceDiagnostic{}
	for i, p := range paths {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		path := templatePath(p, cwd)
		info, err := os.Stat(path)
		if err != nil {
			if !includeDefaults || i >= 2 {
				result.Diagnostics = append(result.Diagnostics, ResourceDiagnostic{Type: "warning", Message: "skill path does not exist", Path: path})
			}
			continue
		}
		source := "path"
		if !includeDefaults || i < 2 {
			if skillUnder(path, filepath.Join(agentDir, "skills")) {
				source = "user"
			} else if skillUnder(path, filepath.Join(cwd, ".pig/skills")) {
				source = "project"
			}
		}
		loaded := LoadSkillsResult{}
		if info.IsDir() {
			loaded, err = LoadSkillsFromDir(ctx, LoadSkillsFromDirOptions{Dir: path, Source: source})
		} else if info.Mode().IsRegular() && strings.HasSuffix(path, ".md") {
			loadSkillFile(path, source, &loaded)
		} else {
			loaded.Diagnostics = append(loaded.Diagnostics, ResourceDiagnostic{Type: "warning", Message: "skill path is not a markdown file", Path: path})
		}
		if err != nil {
			return result, err
		}
		result.Diagnostics = append(result.Diagnostics, loaded.Diagnostics...)
		for _, skill := range loaded.Skills {
			real, err := filepath.EvalSymlinks(skill.FilePath)
			if err != nil {
				real = skill.FilePath
			}
			if realPaths[real] {
				continue
			}
			if winner, ok := seen[skill.Name]; ok {
				collisions = append(collisions, ResourceDiagnostic{Type: "collision", Message: fmt.Sprintf("name %q collision", skill.Name), Path: skill.FilePath, Collision: &ResourceCollision{ResourceType: "skill", Name: skill.Name, WinnerPath: winner.FilePath, LoserPath: skill.FilePath, WinnerSource: resourceSourceLabel(winner.SourceInfo), LoserSource: resourceSourceLabel(skill.SourceInfo)}})
			} else {
				seen[skill.Name] = skill
				realPaths[real] = true
				result.Skills = append(result.Skills, skill)
			}
		}
	}
	result.Diagnostics = append(result.Diagnostics, collisions...)
	return result, ctx.Err()
}

func skillUnder(path, root string) bool {
	return path == root || strings.HasPrefix(path, root+string(filepath.Separator))
}

func (l *DefaultResourceLoader) loadSkills(ctx context.Context, settings *SettingsManager, trusted bool) (SkillLoadResult, error) {
	paths := []string{}
	sources := map[string]SourceInfo{}
	seen := map[string]bool{}
	add := func(path string, source SourceInfo, enabled bool) {
		real, err := filepath.EvalSymlinks(path)
		if err != nil {
			real = path
		}
		if seen[real] {
			return
		}
		seen[real] = true
		source.Path = path
		sources[path] = source
		if enabled && !l.noSkills {
			paths = append(paths, path)
		}
	}
	{
		global, err := settings.GetGlobalSettings()
		if err != nil {
			return SkillLoadResult{}, err
		}
		project, err := settings.GetProjectSettings()
		if err != nil {
			return SkillLoadResult{}, err
		}
		type scope struct {
			base     string
			scope    SourceScope
			patterns []string
			mode     string
		}
		scopes := []scope{}
		if trusted {
			scopes = append(scopes, scope{filepath.Join(l.cwd, ".pig"), SourceScopeProject, project.Skills, "skills"})
		}
		home, err := os.UserHomeDir()
		if err != nil {
			return SkillLoadResult{}, err
		}
		if trusted {
			for dir := l.cwd; ; dir = filepath.Dir(dir) {
				base := filepath.Join(dir, ".agents")
				if base != filepath.Join(home, ".agents") {
					scopes = append(scopes, scope{base, SourceScopeProject, project.Skills, "agents"})
				}
				if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
					break
				}
				if filepath.Dir(dir) == dir {
					break
				}
			}
		}
		scopes = append(scopes, scope{l.agentDir, SourceScopeUser, global.Skills, "skills"}, scope{filepath.Join(home, ".agents"), SourceScopeUser, global.Skills, "agents"})
		for _, s := range scopes {
			if s.mode == "skills" {
				for _, p := range s.patterns {
					if strings.ContainsAny(p, "*?") || strings.HasPrefix(p, "!") || strings.HasPrefix(p, "+") || strings.HasPrefix(p, "-") {
						continue
					}
					for _, file := range templateFiles(templatePath(p, s.base), "skills") {
						add(file, SourceInfo{Source: "local", Scope: s.scope, Origin: ResourceOriginTopLevel}, skillEnabled(file, s.base, s.patterns, false))
					}
				}
			}
			for _, file := range templateFiles(filepath.Join(s.base, "skills"), s.mode) {
				add(file, SourceInfo{Source: "auto", Scope: s.scope, Origin: ResourceOriginTopLevel, BaseDir: s.base}, skillEnabled(file, s.base, s.patterns, true))
			}
		}
	}
	seen = map[string]bool{}
	for _, path := range paths {
		real, err := filepath.EvalSymlinks(path)
		if err != nil {
			real = path
		}
		seen[real] = true
	}
	for _, p := range l.skillPaths {
		path := templatePath(p, l.cwd)
		real, err := filepath.EvalSymlinks(path)
		if err != nil {
			real = path
		}
		if !seen[real] {
			paths = append(paths, path)
			seen[real] = true
		}
	}
	loaded, err := LoadSkills(ctx, l.cwd, l.agentDir, paths, false)
	if err != nil {
		return SkillLoadResult{}, err
	}
	for i, skill := range loaded.Skills {
		if source, ok := sources[skill.FilePath]; ok {
			loaded.Skills[i].SourceInfo = source
		}
	}
	for _, d := range loaded.Diagnostics {
		if c := d.Collision; c != nil {
			if source, ok := sources[c.WinnerPath]; ok {
				c.WinnerSource = resourceSourceLabel(source)
			}
			if source, ok := sources[c.LoserPath]; ok {
				c.LoserSource = resourceSourceLabel(source)
			}
		}
	}
	return SkillLoadResult{Skills: loaded.Skills, Diagnostics: loaded.Diagnostics}, nil
}

func skillEnabled(path, base string, patterns []string, auto bool) bool {
	if filepath.Base(path) != "SKILL.md" {
		return templateEnabled(path, base, patterns, auto)
	}
	expanded := append([]string{}, patterns...)
	for _, p := range patterns {
		if strings.HasPrefix(p, "!") && !templateEnabled(filepath.Dir(path), base, []string{p}, true) {
			expanded = append(expanded, "!"+filepath.ToSlash(path))
		}
		if (strings.HasPrefix(p, "+") || strings.HasPrefix(p, "-")) && templatePath(p[1:], base) == filepath.Dir(path) {
			expanded = append(expanded, p[:1]+path)
		}
	}
	return templateEnabled(path, base, expanded, auto)
}

func (s *AgentSession) expandSkill(text string) string {
	if !strings.HasPrefix(text, "/skill:") {
		return text
	}
	name, args, _ := strings.Cut(text[7:], " ")
	loaded, err := s.resourceLoader.GetSkills()
	if err != nil {
		return text
	}
	for _, skill := range loaded.Skills {
		if skill.Name != name {
			continue
		}
		info, err := os.Stat(skill.FilePath)
		if err != nil || !info.Mode().IsRegular() {
			return text
		}
		data, err := os.ReadFile(skill.FilePath)
		if err != nil {
			return text
		}
		body, err := StripFrontmatter(string(data))
		if err != nil {
			return text
		}
		block := fmt.Sprintf("<skill name=\"%s\" location=\"%s\">\nReferences are relative to %s.\n\n%s\n</skill>", skill.Name, skill.FilePath, skill.BaseDir, strings.TrimSpace(body))
		if args = strings.TrimSpace(args); args != "" {
			block += "\n\n" + args
		}
		return block
	}
	return text
}
