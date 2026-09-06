package parity_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
)

func TestCredentialStorageParity(t *testing.T) {
	root := parityRepoRoot(t)
	lock, _, err := baseline.Load(filepath.Join(root, "parity/baseline"))
	if err != nil {
		t.Fatal(err)
	}
	locked := parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository}
	fixture, err := parity.LoadFixture(filepath.Join(root, "parity/oracle/fixtures/credentials.json"), locked)
	if err != nil {
		t.Fatal(err)
	}
	oracle, err := parity.NewFixtureDriver(fixture, locked)
	if err != nil {
		t.Fatal(err)
	}
	result, err := parity.RunCase(context.Background(), fixture.Case, oracle, parity.DriverFunc{SurfaceName: parity.SurfaceGoSDK, ObserveFunc: func(ctx context.Context, c parity.Case) (parity.Observation, error) {
		must := func(err error) {
			t.Helper()
			if err != nil {
				t.Fatal(err)
			}
		}
		var input struct {
			Values []string
			Env    ai.ProviderEnv
		}
		must(json.Unmarshal(c.Input, &input))
		path := filepath.Join(t.TempDir(), "agent", "auth.json")
		store, err := codingagent.NewAuthStorage(path)
		must(err)
		keys := []string{}
		for _, key := range input.Values {
			_, err = store.Modify(ctx, "deepseek", func(context.Context, ai.Credential) (ai.Credential, error) {
				return ai.UnmarshalCredential([]byte(`{"type":"api_key","key":` + string(mustJSON(t, key)) + `,"env":{"PIG_PARITY_LEFT":"left","PIG_PARITY_RIGHT":"right"},"custom":{"preserved":17}}`))
			}, ai.AuthOperationOptions{})
			must(err)
			credential, err := store.Read(ctx, "deepseek", ai.AuthOperationOptions{})
			must(err)
			keys = append(keys, credentialKey(credential))
		}
		raw, err := codingagent.ReadStoredCredential(ctx, "deepseek", path)
		must(err)
		unchanged, err := store.Modify(ctx, "deepseek", func(context.Context, ai.Credential) (ai.Credential, error) { return nil, nil }, ai.AuthOperationOptions{})
		must(err)
		_, err = store.Modify(ctx, "openai", func(context.Context, ai.Credential) (ai.Credential, error) {
			return ai.APIKeyCredential{Type: ai.AuthTypeAPIKey, Key: ai.Some("other")}, nil
		}, ai.AuthOperationOptions{})
		must(err)
		infos, err := store.List(ctx, ai.AuthOperationOptions{})
		must(err)
		listed := []string{}
		for _, info := range infos {
			listed = append(listed, string(info.ProviderID))
		}
		must(store.Delete(ctx, "deepseek", ai.AuthOperationOptions{}))
		deleted, err := store.Read(ctx, "deepseek", ai.AuthOperationOptions{})
		must(err)
		other, err := store.Read(ctx, "openai", ai.AuthOperationOptions{})
		must(err)
		data, err := ai.MarshalCredential(raw)
		must(err)
		var fields map[string]any
		must(json.Unmarshal(data, &fields))
		stat, err := os.Stat(path)
		must(err)
		rawKey := credentialKey(raw)
		outcome, err := json.Marshal(map[string]any{"keys": keys, "rawKey": rawKey, "unchanged": credentialKey(unchanged) == rawKey, "extra": fields["custom"], "listed": listed, "deleted": deleted == nil, "other": credentialKey(other), "fileMode": stat.Mode().Perm()})
		effects := []parity.SideEffect{}
		return parity.Observation{Outcome: outcome, SideEffects: &effects}, err
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Match {
		t.Fatalf("credential parity: %+v", result)
	}
}
func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func credentialKey(c ai.Credential) string { key, _ := c.(ai.APIKeyCredential).Key.Value(); return key }
