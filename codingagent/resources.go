package codingagent

import (
	"context"
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

// DefaultResourceLoaderOptions preserves the pinned construction inputs while
// resource loading remains deferred. Callback fields are inert data at this
// milestone and are never invoked by the constructor.
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

type DefaultResourceLoader struct{}

// NewDefaultResourceLoader fails before resolving paths, constructing default
// collaborators, or invoking overrides. Resource loading is deliberately
// deferred without selecting an extension ABI (ADR-0009).
func NewDefaultResourceLoader(DefaultResourceLoaderOptions) (*DefaultResourceLoader, error) {
	return nil, notImplemented("NewDefaultResourceLoader")
}

func LoadProjectContextFiles(context.Context, string, string) ([]AgentsFile, error) {
	return nil, notImplemented("LoadProjectContextFiles")
}

func (DefaultResourceLoader) GetExtensions() (LoadExtensionsResult, error) {
	return LoadExtensionsResult{}, notImplemented("DefaultResourceLoader.GetExtensions")
}
func (DefaultResourceLoader) GetSkills() (SkillLoadResult, error) {
	return SkillLoadResult{}, notImplemented("DefaultResourceLoader.GetSkills")
}
func (DefaultResourceLoader) GetPrompts() (PromptTemplateLoadResult, error) {
	return PromptTemplateLoadResult{}, notImplemented("DefaultResourceLoader.GetPrompts")
}
func (DefaultResourceLoader) GetThemes() (ThemeLoadResult, error) {
	return ThemeLoadResult{}, notImplemented("DefaultResourceLoader.GetThemes")
}
func (DefaultResourceLoader) GetAgentsFiles() ([]AgentsFile, error) {
	return nil, notImplemented("DefaultResourceLoader.GetAgentsFiles")
}
func (DefaultResourceLoader) GetSystemPrompt() (*string, error) {
	return nil, notImplemented("DefaultResourceLoader.GetSystemPrompt")
}
func (DefaultResourceLoader) GetSystemPromptSource() (*ResourcePathSource, error) {
	return nil, notImplemented("DefaultResourceLoader.GetSystemPromptSource")
}
func (DefaultResourceLoader) GetAppendSystemPrompt() ([]string, error) {
	return nil, notImplemented("DefaultResourceLoader.GetAppendSystemPrompt")
}
func (DefaultResourceLoader) GetAppendSystemPromptSources() ([]ResourcePathSource, error) {
	return nil, notImplemented("DefaultResourceLoader.GetAppendSystemPromptSources")
}
func (DefaultResourceLoader) ExtendResources(ResourceExtensionPaths) error {
	return notImplemented("DefaultResourceLoader.ExtendResources")
}
func (DefaultResourceLoader) LoadProjectTrustExtensions(context.Context) (LoadExtensionsResult, error) {
	return LoadExtensionsResult{}, notImplemented("DefaultResourceLoader.LoadProjectTrustExtensions")
}
func (DefaultResourceLoader) Reload(context.Context, ...ResourceLoaderReloadOptions) error {
	return notImplemented("DefaultResourceLoader.Reload")
}
