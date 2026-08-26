package codingagent

import "context"

// ProjectTrustDecision is tri-state: nil means no saved decision, while a
// non-nil value is an explicit trusted or untrusted decision.
type ProjectTrustDecision = *bool

// ProjectTrustDecisionTrusted returns a fresh explicit trusted decision.
func ProjectTrustDecisionTrusted() ProjectTrustDecision { return projectTrustDecision(true) }

// ProjectTrustDecisionUntrusted returns a fresh explicit untrusted decision.
func ProjectTrustDecisionUntrusted() ProjectTrustDecision { return projectTrustDecision(false) }

func projectTrustDecision(value bool) ProjectTrustDecision { return &value }

type ProjectTrustStoreEntry struct {
	Path     string
	Decision bool
}

type ProjectTrustUpdate struct {
	Path     string
	Decision ProjectTrustDecision
}

// ProjectTrustStore retains only the configured path. No constructor or method
// stats, reads, creates, locks, or writes the trust store in this scaffold.
type ProjectTrustStore struct {
	agentDir string
}

func NewProjectTrustStore(agentDir string) *ProjectTrustStore {
	return &ProjectTrustStore{agentDir: agentDir}
}

func HasTrustRequiringProjectResources(context.Context, string) (bool, error) {
	return false, notImplemented("HasTrustRequiringProjectResources")
}

func (ProjectTrustStore) Get(context.Context, string) (ProjectTrustDecision, error) {
	return nil, notImplemented("ProjectTrustStore.Get")
}

func (ProjectTrustStore) GetEntry(context.Context, string) (*ProjectTrustStoreEntry, error) {
	return nil, notImplemented("ProjectTrustStore.GetEntry")
}

func (ProjectTrustStore) Set(context.Context, string, ProjectTrustDecision) error {
	return notImplemented("ProjectTrustStore.Set")
}

func (ProjectTrustStore) SetMany(context.Context, []ProjectTrustUpdate) error {
	return notImplemented("ProjectTrustStore.SetMany")
}
