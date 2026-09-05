package parity_test

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
)

type settingsStorage struct {
	content *string
	scopes  []codingagent.SettingsScope
	fail    bool
}

func (s *settingsStorage) WithLock(scope codingagent.SettingsScope, fn func(*string) *string) {
	s.scopes = append(s.scopes, scope)
	if scope != codingagent.SettingsScopeGlobal {
		panic("project accessed")
	}
	if next := fn(s.content); next != nil {
		if s.fail {
			panic(errors.New("write failed"))
		}
		s.content = next
	}
}
func settingsJSON(t *testing.T, value any) map[string]any {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err = json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	return result
}
func TestGlobalSettingsParity(t *testing.T) {
	root := parityRepoRoot(t)
	lock, _, err := baseline.Load(filepath.Join(root, "parity/baseline"))
	if err != nil {
		t.Fatal(err)
	}
	locked := parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository}
	fixture, err := parity.LoadFixture(filepath.Join(root, "parity/oracle/fixtures/global-settings.json"), locked)
	if err != nil {
		t.Fatal(err)
	}
	oracle, err := parity.NewFixtureDriver(fixture, locked)
	if err != nil {
		t.Fatal(err)
	}
	result, err := parity.RunCase(context.Background(), fixture.Case, oracle, parity.DriverFunc{SurfaceName: parity.SurfaceGoSDK, ObserveFunc: func(ctx context.Context, c parity.Case) (parity.Observation, error) {
		var input struct {
			Settings  json.RawMessage      `json:"settings"`
			Overrides codingagent.Settings `json:"overrides"`
		}
		if err := json.Unmarshal(c.Input, &input); err != nil {
			return parity.Observation{}, err
		}
		raw := string(input.Settings)
		storage := &settingsStorage{content: &raw}
		trusted := false
		opts := codingagent.SettingsManagerCreateOptions{ProjectTrusted: &trusted}
		manager, err := codingagent.NewSettingsManagerFromStorage(storage, opts)
		if err != nil {
			return parity.Observation{}, err
		}
		defaults, err := codingagent.NewInMemorySettingsManager(codingagent.Settings{}, opts)
		if err != nil {
			return parity.Observation{}, err
		}
		must := func(err error) {
			t.Helper()
			if err != nil {
				t.Fatal(err)
			}
		}
		drain := func(m *codingagent.SettingsManager) []string {
			errs, err := m.DrainErrors()
			must(err)
			scopes := []string{}
			for _, e := range errs {
				if !strings.HasPrefix(e.Error(), "global settings:") {
					t.Fatalf("unscoped error: %v", e)
				}
				scopes = append(scopes, "global")
			}
			return scopes
		}
		migrated, err := manager.GetGlobalSettings()
		must(err)
		must(manager.ApplyOverrides(input.Overrides))
		retry, err := manager.GetProviderRetrySettings()
		must(err)
		packages, err := manager.GetPackages()
		must(err)
		model, err := manager.GetDefaultModel()
		must(err)
		overrides := map[string]any{"retry": retry, "packages": packages, "model": model, "globalModel": *migrated.DefaultModel}
		external := settingsJSON(t, json.RawMessage(*storage.content))
		external["packages"] = []any{}
		external["retry"].(map[string]any)["provider"].(map[string]any)["maxRetries"] = 7
		encoded, err := json.Marshal(external)
		must(err)
		raw = string(encoded)
		storage.content = &raw
		must(manager.SetDefaultThinkingLevel(agent.ThinkingLevel("low")))
		must(manager.SetRetryEnabled(false))
		must(manager.Flush(ctx))
		saved := json.RawMessage(*storage.content)
		must(manager.Reload(ctx))
		retry, err = manager.GetProviderRetrySettings()
		must(err)
		model, err = manager.GetDefaultModel()
		must(err)
		thinking, err := manager.GetDefaultThinkingLevel()
		must(err)
		packages, err = manager.GetPackages()
		must(err)
		reloaded := map[string]any{"retry": retry, "model": model, "thinking": thinking, "packages": packages}
		raw = "{invalid"
		storage.content = &raw
		must(manager.Reload(ctx))
		model, err = manager.GetDefaultModel()
		must(err)
		invalid := map[string]any{"retained": model, "errors": drain(manager), "drained": len(drain(manager))}
		must(manager.SetTheme("light"))
		must(manager.Flush(ctx))
		invalid["preserved"] = *storage.content == "{invalid"
		raw = `{"defaultModel":"repaired"}`
		storage.content = &raw
		must(manager.Reload(ctx))
		must(manager.SetDefaultProvider("deepseek"))
		must(manager.Flush(ctx))
		repaired := json.RawMessage(*storage.content)
		seed := "seed"
		memory, err := codingagent.NewInMemorySettingsManager(codingagent.Settings{DefaultModel: &seed}, opts)
		must(err)
		must(memory.Reload(ctx))
		memoryModel, err := memory.GetDefaultModel()
		must(err)
		failed, err := codingagent.NewSettingsManagerFromStorage(&settingsStorage{fail: true}, opts)
		must(err)
		must(failed.SetDefaultModel("local"))
		must(failed.Flush(ctx))
		failedModel, err := failed.GetDefaultModel()
		must(err)
		compaction, err := defaults.GetCompactionSettings()
		must(err)
		retryDefaults, err := defaults.GetRetrySettings()
		must(err)
		providerRetry, err := defaults.GetProviderRetrySettings()
		must(err)
		transport, err := defaults.GetTransport()
		must(err)
		steering, err := defaults.GetSteeringMode()
		must(err)
		trust, err := defaults.GetDefaultProjectTrust()
		must(err)
		sessionDir, err := defaults.GetSessionDir()
		must(err)
		var sessionValue any
		if sessionDir != "" {
			sessionValue = sessionDir
		}
		globalOnly := true
		for _, scope := range storage.scopes {
			globalOnly = globalOnly && scope == codingagent.SettingsScopeGlobal
		}
		outcome, err := json.Marshal(map[string]any{"defaults": map[string]any{"compaction": map[string]any{"enabled": compaction.Enabled, "reserveTokens": compaction.ReserveTokens, "keepRecentTokens": compaction.KeepRecentTokens}, "retry": retryDefaults, "providerRetry": providerRetry, "transport": transport, "steering": steering, "trust": trust, "sessionDir": sessionValue}, "migrated": migrated, "overrides": overrides, "saved": saved, "reloaded": reloaded, "invalid": invalid, "repaired": repaired, "memory": memoryModel, "writeFailure": map[string]any{"model": failedModel, "errors": drain(failed)}, "globalOnly": globalOnly})
		effects := []parity.SideEffect{}
		return parity.Observation{Outcome: outcome, SideEffects: &effects}, err
	}})
	if err != nil || !result.Match || len(result.Normalizations) != 0 {
		t.Fatalf("global settings parity = %+v, %v", result, err)
	}
}
