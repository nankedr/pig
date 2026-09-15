package codingagent

import (
	"context"
	"fmt"
	"slices"
	"sync"
)

type ResourceCollision struct {
	ResourceType string
	Name         string
	WinnerPath   string
	LoserPath    string
	WinnerSource *string
	LoserSource  *string
}

func resourceSourceLabel(source SourceInfo) *string {
	label := string(source.Scope) + ":" + source.Source
	return &label
}

type ResourceDiagnostic struct {
	Type      string
	Message   string
	Path      string
	Collision *ResourceCollision
}

type AgentsFile struct {
	Path    string
	Content string
}

type ResourcePathSource struct {
	Path string
}

type SkillLoadResult struct {
	Skills      []Skill
	Diagnostics []ResourceDiagnostic
}

type PromptTemplateLoadResult struct {
	Prompts     []PromptTemplate
	Diagnostics []ResourceDiagnostic
}

type ThemeLoadResult struct {
	Themes      []*Theme
	Diagnostics []ResourceDiagnostic
}

type ResourceExtensionPaths struct {
	SkillPaths  []ResolvedResource
	PromptPaths []ResolvedResource
	ThemePaths  []ResolvedResource
}

// ResourceLoaderReloadOptions is the Go projection of the pinned reload
// options. ResolveProjectTrust remains opaque until the extension execution
// ABI is selected.
type ResourceLoaderReloadOptions struct {
	ResolveProjectTrust ExtensionHandler
}

// DefaultResourceLoaderOptions supports local Context Files and system prompts.
type DefaultResourceLoaderOptions struct {
	CWD      string
	AgentDir string

	SettingsManager *SettingsManager
	EventBus        EventBus

	AdditionalExtensionPaths      []string
	AdditionalSkillPaths          []string
	AdditionalPromptTemplatePaths []string
	AdditionalThemePaths          []string
	ExtensionFactories            []InlineExtension

	NoExtensions      bool
	NoSkills          bool
	NoPromptTemplates bool
	NoThemes          bool
	NoContextFiles    bool

	SystemPrompt       *string
	AppendSystemPrompt []string

	ExtensionsOverride         ExtensionHandler
	SkillsOverride             ExtensionHandler
	PromptsOverride            ExtensionHandler
	ThemesOverride             ExtensionHandler
	AgentsFilesOverride        ExtensionHandler
	SystemPromptOverride       ExtensionHandler
	AppendSystemPromptOverride ExtensionHandler
}

type ResourceLoader interface {
	GetExtensions() (LoadExtensionsResult, error)
	GetSkills() (SkillLoadResult, error)
	GetPrompts() (PromptTemplateLoadResult, error)
	GetThemes() (ThemeLoadResult, error)
	GetAgentsFiles() ([]AgentsFile, error)
	GetSystemPrompt() (*string, error)
	GetSystemPromptSource() (*ResourcePathSource, error)
	GetAppendSystemPrompt() ([]string, error)
	GetAppendSystemPromptSources() ([]ResourcePathSource, error)
	ExtendResources(ResourceExtensionPaths) error
	Reload(context.Context, ...ResourceLoaderReloadOptions) error
}

type DefaultResourceLoader struct {
	mu                sync.RWMutex
	cwd, agentDir     string
	noContextFiles    bool
	files             []AgentsFile
	diagnostics       []ResourceDiagnostic
	settings          *SettingsManager
	systemInput       *string
	appendInputs      []string
	systemPrompt      *string
	systemSource      *ResourcePathSource
	appendPrompts     []string
	appendSources     []ResourcePathSource
	promptDiagnostics []ResourceDiagnostic
	skills            SkillLoadResult
	skillPaths        []string
	noSkills          bool
	themes            ThemeLoadResult
	themePaths        []string
	noThemes          bool
	templates         PromptTemplateLoadResult
	templatePaths     []string
	noPromptTemplates bool
}

func NewDefaultResourceLoader(options DefaultResourceLoaderOptions) (*DefaultResourceLoader, error) {
	if len(options.AdditionalExtensionPaths) > 0 || len(options.ExtensionFactories) > 0 || options.ExtensionsOverride != nil || options.SkillsOverride != nil || options.PromptsOverride != nil || options.ThemesOverride != nil || options.AgentsFilesOverride != nil || options.SystemPromptOverride != nil || options.AppendSystemPromptOverride != nil {
		return nil, notImplemented("NewDefaultResourceLoader")
	}
	cwd, err := resolveSessionPath(options.CWD)
	if err != nil {
		return nil, err
	}
	dir := options.AgentDir
	if dir == "" {
		dir, err = GetAgentDir()
	} else {
		dir, err = resolveSessionPath(dir)
	}
	if err != nil {
		return nil, err
	}
	return &DefaultResourceLoader{cwd: cwd, agentDir: dir, noContextFiles: options.NoContextFiles, noSkills: options.NoSkills, noThemes: options.NoThemes, themePaths: slices.Clone(options.AdditionalThemePaths), skillPaths: slices.Clone(options.AdditionalSkillPaths), noPromptTemplates: options.NoPromptTemplates, templatePaths: slices.Clone(options.AdditionalPromptTemplatePaths), files: []AgentsFile{}, settings: options.SettingsManager, systemInput: cloneStringPointer(options.SystemPrompt), appendInputs: slices.Clone(options.AppendSystemPrompt)}, nil
}

func (*DefaultResourceLoader) GetExtensions() (LoadExtensionsResult, error) {
	return LoadExtensionsResult{}, notImplemented("DefaultResourceLoader.GetExtensions")
}
func (l *DefaultResourceLoader) GetSkills() (SkillLoadResult, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.cwd == "" {
		return SkillLoadResult{}, notImplemented("DefaultResourceLoader.GetSkills")
	}
	result := SkillLoadResult{Skills: append([]Skill{}, l.skills.Skills...), Diagnostics: append([]ResourceDiagnostic{}, l.skills.Diagnostics...)}
	for i, d := range result.Diagnostics {
		if d.Collision != nil {
			c := *d.Collision
			c.WinnerSource = cloneStringPointer(c.WinnerSource)
			c.LoserSource = cloneStringPointer(c.LoserSource)
			result.Diagnostics[i].Collision = &c
		}
	}
	return result, nil
}
func (l *DefaultResourceLoader) GetPrompts() (PromptTemplateLoadResult, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.cwd == "" {
		return PromptTemplateLoadResult{}, notImplemented("DefaultResourceLoader.GetPrompts")
	}
	result := PromptTemplateLoadResult{Prompts: append([]PromptTemplate{}, l.templates.Prompts...), Diagnostics: append([]ResourceDiagnostic{}, l.templates.Diagnostics...)}
	for i, d := range result.Diagnostics {
		if d.Collision != nil {
			collision := *d.Collision
			collision.WinnerSource = cloneStringPointer(collision.WinnerSource)
			collision.LoserSource = cloneStringPointer(collision.LoserSource)
			result.Diagnostics[i].Collision = &collision
		}
	}
	return result, nil
}
func (l *DefaultResourceLoader) GetThemes() (ThemeLoadResult, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.cwd == "" {
		return ThemeLoadResult{}, notImplemented("DefaultResourceLoader.GetThemes")
	}
	result := ThemeLoadResult{Themes: []*Theme{}, Diagnostics: append([]ResourceDiagnostic{}, l.themes.Diagnostics...)}
	for _, theme := range l.themes.Themes {
		copy := *theme
		if copy.SourceInfo != nil {
			source := *copy.SourceInfo
			copy.SourceInfo = &source
		}
		result.Themes = append(result.Themes, &copy)
	}
	for i, d := range result.Diagnostics {
		if d.Collision != nil {
			c := *d.Collision
			c.WinnerSource = cloneStringPointer(c.WinnerSource)
			c.LoserSource = cloneStringPointer(c.LoserSource)
			result.Diagnostics[i].Collision = &c
		}
	}
	return result, nil
}
func (l *DefaultResourceLoader) GetAgentsFiles() ([]AgentsFile, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.cwd == "" {
		return nil, notImplemented("DefaultResourceLoader.GetAgentsFiles")
	}
	return append([]AgentsFile{}, l.files...), nil
}

func (l *DefaultResourceLoader) GetContextFileDiagnostics() []ResourceDiagnostic {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return append([]ResourceDiagnostic{}, l.diagnostics...)
}
func (l *DefaultResourceLoader) GetSystemPrompt() (*string, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.cwd == "" {
		return nil, notImplemented("DefaultResourceLoader.GetSystemPrompt")
	}
	return cloneStringPointer(l.systemPrompt), nil
}
func (l *DefaultResourceLoader) GetSystemPromptSource() (*ResourcePathSource, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.cwd == "" {
		return nil, notImplemented("DefaultResourceLoader.GetSystemPromptSource")
	}
	if l.systemSource == nil {
		return nil, nil
	}
	source := *l.systemSource
	return &source, nil
}
func (l *DefaultResourceLoader) GetAppendSystemPrompt() ([]string, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.cwd == "" {
		return nil, notImplemented("DefaultResourceLoader.GetAppendSystemPrompt")
	}
	return append([]string{}, l.appendPrompts...), nil
}
func (l *DefaultResourceLoader) GetAppendSystemPromptSources() ([]ResourcePathSource, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.cwd == "" {
		return nil, notImplemented("DefaultResourceLoader.GetAppendSystemPromptSources")
	}
	return append([]ResourcePathSource{}, l.appendSources...), nil
}
func (l *DefaultResourceLoader) GetSystemPromptDiagnostics() []ResourceDiagnostic {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return append([]ResourceDiagnostic{}, l.promptDiagnostics...)
}
func (*DefaultResourceLoader) ExtendResources(ResourceExtensionPaths) error {
	return notImplemented("DefaultResourceLoader.ExtendResources")
}
func (*DefaultResourceLoader) LoadProjectTrustExtensions(context.Context) (LoadExtensionsResult, error) {
	return LoadExtensionsResult{}, notImplemented("DefaultResourceLoader.LoadProjectTrustExtensions")
}
func (l *DefaultResourceLoader) Reload(ctx context.Context, options ...ResourceLoaderReloadOptions) error {
	if l.cwd == "" || len(options) > 1 || len(options) == 1 && options[0].ResolveProjectTrust != nil {
		return notImplemented("DefaultResourceLoader.Reload")
	}
	if ctx == nil {
		return fmt.Errorf("resource loader context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	settings := l.settings
	if settings == nil {
		var err error
		settings, err = NewSettingsManager(l.cwd, &l.agentDir)
		if err != nil {
			return err
		}
		if err = prepareHeadlessProjectSettings(ctx, l.cwd, l.agentDir, settings, nil); err != nil {
			return err
		}
	}
	trusted, err := settings.IsProjectTrusted()
	if err != nil {
		return err
	}
	promptDiagnostics := []ResourceDiagnostic{}
	systemInput := l.systemInput
	if systemInput == nil {
		systemInput = l.discoverPrompt("SYSTEM.md", trusted)
	}
	system, source := resolveResourcePrompt(systemInput, "system prompt", &promptDiagnostics)
	appendInputs := l.appendInputs
	if appendInputs == nil {
		if input := l.discoverPrompt("APPEND_SYSTEM.md", trusted); input != nil {
			appendInputs = []string{*input}
		}
	}
	appended, sources := []string{}, []ResourcePathSource{}
	for _, input := range appendInputs {
		prompt, source := resolveResourcePrompt(&input, "append system prompt", &promptDiagnostics)
		if prompt != nil {
			appended = append(appended, *prompt)
		}
		if source != nil {
			sources = append(sources, *source)
		}
	}
	files := []AgentsFile{}
	diagnostics := []ResourceDiagnostic{}
	if !l.noContextFiles {
		var err error
		files, err = loadProjectContextFiles(ctx, l.cwd, l.agentDir, &diagnostics)
		if err != nil {
			return err
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	templates, err := l.loadPromptTemplates(ctx, settings, trusted)
	if err != nil {
		return err
	}
	skills, err := l.loadSkills(ctx, settings, trusted)
	if err != nil {
		return err
	}
	themes, err := l.loadThemes(ctx, settings, trusted)
	if err != nil {
		return err
	}
	l.themes = themes
	l.skills = skills
	l.templates = templates
	l.files, l.diagnostics = files, diagnostics
	l.systemPrompt, l.systemSource = system, source
	l.appendPrompts, l.appendSources, l.promptDiagnostics = appended, sources, promptDiagnostics
	return nil
}
