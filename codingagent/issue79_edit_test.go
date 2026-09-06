package codingagent_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
)

func TestEditToolSessionParity(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	lock, _, err := baseline.Load(filepath.Join(root, "parity/baseline"))
	if err != nil {
		t.Fatal(err)
	}
	locked := parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository}
	fixture, err := parity.LoadFixture(filepath.Join(root, "parity/oracle/fixtures/edit-tool.json"), locked)
	if err != nil {
		t.Fatal(err)
	}
	oracle, err := parity.NewFixtureDriver(fixture, locked)
	if err != nil {
		t.Fatal(err)
	}
	result, err := parity.RunCase(context.Background(), fixture.Case, oracle, parity.DriverFunc{SurfaceName: parity.SurfaceGoSDK, ObserveFunc: func(ctx context.Context, c parity.Case) (parity.Observation, error) {
		var input struct {
			Cases []struct {
				Name, Content string
				Bytes         []byte
				Args          map[string]any
			}
		}
		if err := json.Unmarshal(c.Input, &input); err != nil {
			return parity.Observation{}, err
		}
		cwd := t.TempDir()
		edit, err := codingagent.CreateEditTool(cwd)
		if err != nil {
			return parity.Observation{}, err
		}
		read, err := codingagent.CreateReadTool(cwd)
		if err != nil {
			return parity.Observation{}, err
		}
		results := []map[string]any{}
		for _, item := range input.Cases {
			data := []byte(item.Content)
			if item.Bytes != nil {
				data = item.Bytes
			}
			if err := os.WriteFile(filepath.Join(cwd, "target.txt"), data, 0600); err != nil {
				return parity.Observation{}, err
			}
			item.Args["path"] = "target.txt"
			messages := runFileToolSession(t, ctx, cwd, []agent.ErasedAgentTool{edit, read}, []ai.ToolCall{
				{Type: "toolCall", ID: "edit", Name: "edit", Arguments: item.Args},
				{Type: "toolCall", ID: "read", Name: "read", Arguments: map[string]any{"path": "target.txt"}},
			})
			content, err := os.ReadFile(filepath.Join(cwd, "target.txt"))
			if err != nil {
				return parity.Observation{}, err
			}
			details, _ := messages[0].Details.Value()
			results = append(results, map[string]any{"text": readToolResultText(t, messages[0]), "details": details, "isError": messages[0].IsError, "read": readToolResultText(t, messages[1]), "content": string(content)})
		}
		definition, err := codingagent.CreateEditToolDefinition(cwd)
		if err != nil {
			return parity.Observation{}, err
		}
		metadata := map[string]any{"name": definition.Name, "label": definition.Label, "description": definition.Description, "parameters": definition.Parameters, "promptSnippet": definition.PromptSnippet, "promptGuidelines": definition.PromptGuidelines}
		outcome, err := json.Marshal(map[string]any{"results": results, "metadata": metadata})
		return parity.Observation{Outcome: outcome, SideEffects: &[]parity.SideEffect{}}, err
	}})
	if err != nil {
		var expected, actual struct{ Results []map[string]any }
		json.Unmarshal(result.Oracle.Outcome, &expected)
		json.Unmarshal(result.Pig.Outcome, &actual)
		for i, want := range expected.Results {
			if i < len(actual.Results) && !reflect.DeepEqual(want, actual.Results[i]) {
				t.Logf("case %d: got=%v want=%v", i, actual.Results[i], want)
			}
		}
		t.Fatalf("%v: %+v", err, result.Differences)
	}
	if !result.Match {
		t.Fatalf("parity: %#v", result)
	}
}
