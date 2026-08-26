package codingagent

import "context"

type SourceScope string

const (
	SourceScopeUser      SourceScope = "user"
	SourceScopeProject   SourceScope = "project"
	SourceScopeTemporary SourceScope = "temporary"
)

type ResourceOrigin string

const (
	ResourceOriginPackage  ResourceOrigin = "package"
	ResourceOriginTopLevel ResourceOrigin = "top-level"
)

type PathMetadata struct {
	Source  string
	Scope   SourceScope
	Origin  ResourceOrigin
	BaseDir string
}

type ResolvedResource struct {
	Path     string
	Enabled  bool
	Metadata PathMetadata
}

type ResolvedPaths struct {
	Extensions []ResolvedResource
	Skills     []ResolvedResource
	Prompts    []ResolvedResource
	Themes     []ResolvedResource
}

type ProgressEventType string
type ProgressAction string

const (
	ProgressEventStart    ProgressEventType = "start"
	ProgressEventProgress ProgressEventType = "progress"
	ProgressEventComplete ProgressEventType = "complete"
	ProgressEventError    ProgressEventType = "error"

	ProgressActionInstall ProgressAction = "install"
	ProgressActionRemove  ProgressAction = "remove"
	ProgressActionUpdate  ProgressAction = "update"
	ProgressActionClone   ProgressAction = "clone"
	ProgressActionPull    ProgressAction = "pull"
)

type ProgressEvent struct {
	Type    ProgressEventType
	Action  ProgressAction
	Source  string
	Message string
}

type ProgressCallback func(ProgressEvent)
type MissingSourceCallback func(context.Context, string) (string, error)

type PackageManager interface {
	Resolve(context.Context, MissingSourceCallback) (ResolvedPaths, error)
	Install(context.Context, string, ...bool) error
	InstallAndPersist(context.Context, string, ...bool) error
	Remove(context.Context, string, ...bool) error
	RemoveAndPersist(context.Context, string, ...bool) (bool, error)
	Update(context.Context, ...string) error
	ListConfiguredPackages(context.Context) ([]PackageSource, error)
	ResolveExtensionSources(context.Context, []string, ...bool) (ResolvedPaths, error)
	AddSourceToSettings(context.Context, string, ...bool) (bool, error)
	RemoveSourceFromSettings(context.Context, string, ...bool) (bool, error)
	SetProgressCallback(ProgressCallback) error
	GetInstalledPath(context.Context, string, SourceScope) (string, error)
}

type DefaultPackageManager struct{}

func NewDefaultPackageManager(string, string, *SettingsManager) *DefaultPackageManager {
	return &DefaultPackageManager{}
}

func (DefaultPackageManager) Resolve(context.Context, MissingSourceCallback) (ResolvedPaths, error) {
	return ResolvedPaths{}, notImplemented("DefaultPackageManager.Resolve")
}
func (DefaultPackageManager) Install(context.Context, string, ...bool) error {
	return notImplemented("DefaultPackageManager.Install")
}
func (DefaultPackageManager) InstallAndPersist(context.Context, string, ...bool) error {
	return notImplemented("DefaultPackageManager.InstallAndPersist")
}
func (DefaultPackageManager) Remove(context.Context, string, ...bool) error {
	return notImplemented("DefaultPackageManager.Remove")
}
func (DefaultPackageManager) RemoveAndPersist(context.Context, string, ...bool) (bool, error) {
	return false, notImplemented("DefaultPackageManager.RemoveAndPersist")
}
func (DefaultPackageManager) Update(context.Context, ...string) error {
	return notImplemented("DefaultPackageManager.Update")
}
func (DefaultPackageManager) ListConfiguredPackages(context.Context) ([]PackageSource, error) {
	return nil, notImplemented("DefaultPackageManager.ListConfiguredPackages")
}
func (DefaultPackageManager) ResolveExtensionSources(context.Context, []string, ...bool) (ResolvedPaths, error) {
	return ResolvedPaths{}, notImplemented("DefaultPackageManager.ResolveExtensionSources")
}
func (DefaultPackageManager) AddSourceToSettings(context.Context, string, ...bool) (bool, error) {
	return false, notImplemented("DefaultPackageManager.AddSourceToSettings")
}
func (DefaultPackageManager) RemoveSourceFromSettings(context.Context, string, ...bool) (bool, error) {
	return false, notImplemented("DefaultPackageManager.RemoveSourceFromSettings")
}
func (DefaultPackageManager) SetProgressCallback(ProgressCallback) error {
	return notImplemented("DefaultPackageManager.SetProgressCallback")
}
func (DefaultPackageManager) GetInstalledPath(context.Context, string, SourceScope) (string, error) {
	return "", notImplemented("DefaultPackageManager.GetInstalledPath")
}
func (DefaultPackageManager) CheckForAvailableUpdates(context.Context) ([]string, error) {
	return nil, notImplemented("DefaultPackageManager.CheckForAvailableUpdates")
}
