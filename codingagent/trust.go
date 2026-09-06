package codingagent

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

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

type ProjectTrustStore struct{ agentDir string }

func NewProjectTrustStore(agentDir string) *ProjectTrustStore {
	return &ProjectTrustStore{agentDir: agentDir}
}

func canonicalTrustPath(path string) (string, error) {
	path, err := resolveSessionPath(path)
	if err != nil {
		return "", err
	}
	if real, err := filepath.EvalSymlinks(path); err == nil {
		return real, nil
	}
	return path, nil
}

func HasTrustRequiringProjectResources(ctx context.Context, cwd string) (bool, error) {
	if ctx == nil {
		return false, fmt.Errorf("trust context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	cwd, err := canonicalTrustPath(cwd)
	if err != nil {
		return false, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return false, err
	}
	home, err = canonicalTrustPath(home)
	if err != nil {
		return false, err
	}
	for _, name := range []string{"settings.json", "extensions", "skills", "prompts", "themes", "SYSTEM.md", "APPEND_SYSTEM.md"} {
		if _, err := os.Stat(filepath.Join(cwd, ".pig", name)); err == nil {
			return true, nil
		}
	}
	for current := cwd; ; current = filepath.Dir(current) {
		if current != home {
			if _, err := os.Stat(filepath.Join(current, ".agents", "skills")); err == nil {
				return true, nil
			}
		}
		if filepath.Dir(current) == current {
			return false, nil
		}
	}
}

func (s ProjectTrustStore) withLock(ctx context.Context, fn func(map[string]*bool) bool) error {
	if ctx == nil {
		return fmt.Errorf("trust context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	dir, err := resolveSessionPath(s.agentDir)
	if err != nil {
		return err
	}
	path := filepath.Join(dir, "trust.json")
	return settingsWithLock(fileSettingsStorage{path: path}, func(current *string) *string {
		if err := ctx.Err(); err != nil {
			panic(err)
		}
		data := map[string]*bool{}
		if current != nil {
			if err := json.Unmarshal([]byte(*current), &data); err != nil {
				panic(fmt.Errorf("invalid trust store: %w", err))
			}
			if data == nil {
				panic(fmt.Errorf("trust store must be an object"))
			}
		}
		if !fn(data) {
			return nil
		}
		if err := os.MkdirAll(dir, 0700); err != nil {
			panic(err)
		}
		if err := os.Chmod(dir, 0700); err != nil {
			panic(err)
		}
		if err := os.Chmod(path, 0600); err != nil && !os.IsNotExist(err) {
			panic(err)
		}
		encoded, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			panic(err)
		}
		next := string(encoded) + "\n"
		return &next
	})
}

func (s ProjectTrustStore) Get(ctx context.Context, cwd string) (ProjectTrustDecision, error) {
	entry, err := s.GetEntry(ctx, cwd)
	if err != nil || entry == nil {
		return nil, err
	}
	return projectTrustDecision(entry.Decision), nil
}

func (s ProjectTrustStore) GetEntry(ctx context.Context, cwd string) (*ProjectTrustStoreEntry, error) {
	cwd, err := canonicalTrustPath(cwd)
	if err != nil {
		return nil, err
	}
	var entry *ProjectTrustStoreEntry
	err = s.withLock(ctx, func(data map[string]*bool) bool {
		for current := cwd; ; current = filepath.Dir(current) {
			if value := data[current]; value != nil {
				entry = &ProjectTrustStoreEntry{Path: current, Decision: *value}
				break
			}
			if filepath.Dir(current) == current {
				break
			}
		}
		return false
	})
	return entry, err
}

func (s ProjectTrustStore) Set(ctx context.Context, cwd string, decision ProjectTrustDecision) error {
	return s.SetMany(ctx, []ProjectTrustUpdate{{Path: cwd, Decision: decision}})
}

func (s ProjectTrustStore) SetMany(ctx context.Context, updates []ProjectTrustUpdate) error {
	normalized := make([]ProjectTrustUpdate, 0, len(updates))
	for _, update := range updates {
		path, err := canonicalTrustPath(update.Path)
		if err != nil {
			return err
		}
		normalized = append(normalized, ProjectTrustUpdate{Path: path, Decision: update.Decision})
	}
	return s.withLock(ctx, func(data map[string]*bool) bool {
		for _, update := range normalized {
			if update.Decision == nil {
				delete(data, update.Path)
			} else {
				data[update.Path] = update.Decision
			}
		}
		return true
	})
}
