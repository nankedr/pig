package parity_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
)

func TestModelRuntimeParity(t *testing.T) {
	t.Setenv("DEEPSEEK_API_KEY", "")
	root := parityRepoRoot(t)
	lock, _, err := baseline.Load(filepath.Join(root, "parity/baseline"))
	if err != nil {
		t.Fatal(err)
	}
	locked := parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository}
	fixture, err := parity.LoadFixture(filepath.Join(root, "parity/oracle/fixtures/model-runtime.json"), locked)
	if err != nil {
		t.Fatal(err)
	}
	oracle, err := parity.NewFixtureDriver(fixture, locked)
	if err != nil {
		t.Fatal(err)
	}
	result, err := parity.RunCase(context.Background(), fixture.Case, oracle, parity.DriverFunc{SurfaceName: parity.SurfaceGoSDK, ObserveFunc: func(ctx context.Context, c parity.Case) (parity.Observation, error) {
		store := ai.NewInMemoryCredentialStore()
		_, err := store.Modify(ctx, "deepseek", func(context.Context, ai.Credential) (ai.Credential, error) {
			return ai.APIKeyCredential{Type: ai.AuthTypeAPIKey, Key: ai.Some("fixture-key")}, nil
		}, ai.AuthOperationOptions{})
		if err != nil {
			return parity.Observation{}, err
		}
		runtime, err := codingagent.NewModelRuntime(ctx, codingagent.CreateModelRuntimeOptions{Credentials: store})
		if err != nil {
			return parity.Observation{}, err
		}
		var input struct {
			Queries  []codingagent.ResolveCliModelOptions
			Patterns []string
		}
		if err = json.Unmarshal(c.Input, &input); err != nil {
			return parity.Observation{}, err
		}
		queries := []any{}
		for _, q := range input.Queries {
			q.ModelRuntime = runtime
			r, err := codingagent.ResolveCLIModel(q)
			if err != nil {
				return parity.Observation{}, err
			}
			var model any
			if r.Model != nil {
				model = string(r.Model.Provider) + "/" + r.Model.ID
			}
			queries = append(queries, map[string]any{"model": model, "thinking": r.ThinkingLevel, "warning": r.Warning, "error": r.Error})
		}
		scope, err := codingagent.ResolveModelScopeWithDiagnostics(ctx, input.Patterns, runtime)
		if err != nil {
			return parity.Observation{}, err
		}
		scoped := []any{}
		for _, m := range scope.ScopedModels {
			scoped = append(scoped, map[string]any{"model": string(m.Model.Provider) + "/" + m.Model.ID, "thinking": m.ThinkingLevel})
		}
		diagnostics := []any{}
		for _, d := range scope.Diagnostics {
			diagnostics = append(diagnostics, map[string]any{"type": d.Type, "code": d.Code, "message": d.Message, "pattern": d.Pattern})
		}
		status, err := runtime.GetProviderAuthStatus("deepseek")
		if err != nil {
			return parity.Observation{}, err
		}
		return parity.Observation{Outcome: mustJSON(t, map[string]any{"queries": queries, "scope": scoped, "diagnostics": diagnostics, "status": map[string]any{"configured": status.Configured, "source": status.Source}}), SideEffects: &[]parity.SideEffect{}}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Match {
		t.Fatalf("parity: %+v", result)
	}
}
