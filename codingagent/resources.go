package codingagent

import (
	"context"
	"fmt"
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

// DefaultResourceLoaderOptions supports local Context Files; other resource inputs remain explicit stubs.
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
	mu             sync.RWMutex
	cwd, agentDir  string
	noContextFiles bool
	files          []AgentsFile
	diagnostics    []ResourceDiagnostic
}

func NewDefaultResourceLoader(options DefaultResourceLoaderOptions) (*DefaultResourceLoader, error) {
	if len(options.AdditionalExtensionPaths) > 0 || len(options.AdditionalSkillPaths) > 0 || len(options.AdditionalPromptTemplatePaths) > 0 || len(options.AdditionalThemePaths) > 0 || len(options.ExtensionFactories) > 0 || options.SystemPrompt != nil || len(options.AppendSystemPrompt) > 0 || options.ExtensionsOverride != nil || options.SkillsOverride != nil || options.PromptsOverride != nil || options.ThemesOverride != nil || options.AgentsFilesOverride != nil || options.SystemPromptOverride != nil || options.AppendSystemPromptOverride != nil {
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
	return &DefaultResourceLoader{cwd: cwd, agentDir: dir, noContextFiles: options.NoContextFiles, files: []AgentsFile{}}, nil
}

func (*DefaultResourceLoader) GetExtensions() (LoadExtensionsResult, error) {
	return LoadExtensionsResult{}, notImplemented("DefaultResourceLoader.GetExtensions")
}
func (*DefaultResourceLoader) GetSkills() (SkillLoadResult, error) {
	return SkillLoadResult{}, notImplemented("DefaultResourceLoader.GetSkills")
}
func (*DefaultResourceLoader) GetPrompts() (PromptTemplateLoadResult, error) {
	return PromptTemplateLoadResult{}, notImplemented("DefaultResourceLoader.GetPrompts")
}
func (*DefaultResourceLoader) GetThemes() (ThemeLoadResult, error) {
	return ThemeLoadResult{}, notImplemented("DefaultResourceLoader.GetThemes")
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
func (*DefaultResourceLoader) GetSystemPrompt() (*string, error) {
	return nil, notImplemented("DefaultResourceLoader.GetSystemPrompt")
}
func (*DefaultResourceLoader) GetSystemPromptSource() (*ResourcePathSource, error) {
	return nil, notImplemented("DefaultResourceLoader.GetSystemPromptSource")
}
func (*DefaultResourceLoader) GetAppendSystemPrompt() ([]string, error) {
	return nil, notImplemented("DefaultResourceLoader.GetAppendSystemPrompt")
}
func (*DefaultResourceLoader) GetAppendSystemPromptSources() ([]ResourcePathSource, error) {
	return nil, notImplemented("DefaultResourceLoader.GetAppendSystemPromptSources")
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
	l.files, l.diagnostics = files, diagnostics
	return nil
}
