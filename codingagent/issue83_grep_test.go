package codingagent_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
)

func TestGrepToolSessionParity(t *testing.T) {
	root, _ := filepath.Abs("..")
	lock, _, err := baseline.Load(filepath.Join(root, "parity/baseline"))
	if err != nil {
		t.Fatal(err)
	}
	locked := parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository}
	fixture, err := parity.LoadFixture(filepath.Join(root, "parity/oracle/fixtures/grep-tool.json"), locked)
	if err != nil {
		t.Fatal(err)
	}
	oracle, err := parity.NewFixtureDriver(fixture, locked)
	if err != nil {
		t.Fatal(err)
	}
	result, err := parity.RunCase(context.Background(), fixture.Case, oracle, parity.DriverFunc{SurfaceName: parity.SurfaceGoSDK, ObserveFunc: func(ctx context.Context, c parity.Case) (parity.Observation, error) {
		var input struct {
			Files    map[string]string
			Searches []map[string]any
		}
		if err := json.Unmarshal(c.Input, &input); err != nil {
			return parity.Observation{}, err
		}
		cwd := t.TempDir()
		t.Setenv("PIG_CODING_AGENT_DIR", t.TempDir())
		t.Setenv("PIG_OFFLINE", "1")
		for name, content := range input.Files {
			path := filepath.Join(cwd, name)
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
		}
		tool, err := codingagent.CreateGrepTool(cwd)
		if err != nil {
			return parity.Observation{}, err
		}
		definition, err := codingagent.CreateGrepToolDefinition(cwd)
		if err != nil {
			return parity.Observation{}, err
		}
		var expected struct{ Results []map[string]any }
		if err := json.Unmarshal(fixture.Observation.Outcome, &expected); err != nil {
			t.Fatal(err)
		}
		results := []map[string]any{}
		for _, args := range input.Searches {
			if mode, _ := args["readMode"].(string); mode != "" {
				options := codingagent.GrepToolOptions{Operations: grepReadOperations{mode: mode}}
				tool, err = codingagent.CreateGrepTool(cwd, options)
				if err != nil {
					t.Fatal(err)
				}
				definition, err = codingagent.CreateGrepToolDefinition(cwd, options)
				if err != nil {
					t.Fatal(err)
				}
			} else {
				tool, _ = codingagent.CreateGrepTool(cwd)
				definition, _ = codingagent.CreateGrepToolDefinition(cwd)
			}
			messages := runFileToolSession(t, ctx, cwd, []agent.ErasedAgentTool{tool}, []ai.ToolCall{{Type: "toolCall", ID: "grep", Name: "grep", Arguments: args}})
			message := messages[0]
			details, _ := message.Details.Value()
			if object, ok := details.(map[string]any); ok && len(object) == 0 {
				details = nil
			}
			text := strings.ReplaceAll(readToolResultText(t, message), cwd, "<cwd>")
			result := map[string]any{"text": text, "details": details, "isError": message.IsError}
			direct, directErr := definition.Execute(ctx, "grep", args, nil)
			if (directErr != nil) != message.IsError {
				t.Fatalf("definition disagrees: %v / %s", directErr, text)
			}
			if directErr == nil {
				got, _ := json.Marshal(map[string]any{"text": direct.Content[0].(ai.TextContent).Text, "details": direct.Details, "isError": false})
				want, _ := json.Marshal(result)
				if string(got) != string(want) {
					t.Fatalf("definition result differs: %s / %s", got, want)
				}
			}
			if !reflect.DeepEqual(result, expected.Results[len(results)]) {
				t.Errorf("case %d args=%v got=%v want=%v", len(results), args, result, expected.Results[len(results)])
			}
			results = append(results, result)
		}
		metadata := map[string]any{"name": definition.Name, "label": definition.Label, "description": definition.Description, "parameters": definition.Parameters, "promptSnippet": definition.PromptSnippet}
		outcome, err := json.Marshal(map[string]any{"results": results, "metadata": metadata})
		return parity.Observation{Outcome: outcome, SideEffects: &[]parity.SideEffect{}}, err
	}})
	if err != nil || !result.Match {
		t.Fatalf("parity: %v %+v", err, result.Differences)
	}
}

func TestGrepToolSelectionAndValidation(t *testing.T) {
	for _, tc := range []struct {
		names, excluded []string
		noTools         codingagent.NoToolsMode
		want            bool
	}{
		{names: []string{"grep"}, want: true}, {names: []string{"grep"}, excluded: []string{"grep"}}, {names: []string{"grep"}, noTools: codingagent.NoToolsBuiltin, want: true}, {},
	} {
		settings, _ := codingagent.NewInMemorySettingsManager(codingagent.Settings{})
		runtime, err := codingagent.CreateHeadlessSession(context.Background(), codingagent.CreateHeadlessSessionOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), SettingsManager: settings, SessionManager: codingagent.NewInMemorySessionManager(""), Provider: "deepseek", Model: "deepseek-v4-flash", Tools: tc.names, ExcludeTools: tc.excluded, NoTools: tc.noTools})
		if err != nil {
			t.Fatal(err)
		}
		if got := strings.Contains(runtime.Session().SystemPrompt(), "- grep: Search file contents for patterns"); got != tc.want {
			t.Fatalf("grep prompt=%v want=%v", got, tc.want)
		}
		runtime.Dispose(context.Background())
	}
	cwd := t.TempDir()
	tool, err := codingagent.CreateGrepTool(cwd)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", t.TempDir())
	t.Setenv("PIG_CODING_AGENT_DIR", t.TempDir())
	for _, args := range []map[string]any{{}, {"pattern": map[string]any{}}, {"pattern": "x", "context": "bad"}, {"pattern": "x", "ignoreCase": map[string]any{}}, {"pattern": "x", "path": map[string]any{}}} {
		messages := runFileToolSession(t, context.Background(), cwd, []agent.ErasedAgentTool{tool}, []ai.ToolCall{{Type: "toolCall", ID: "invalid", Name: "grep", Arguments: args}})
		if !messages[0].IsError || strings.Contains(readToolResultText(t, messages[0]), "ripgrep") {
			t.Fatalf("invalid args %v reached process: %+v", args, messages)
		}
	}
}

type grepReadOperations struct{ mode string }

func (o grepReadOperations) IsDirectory(ctx context.Context, path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return info.IsDir(), nil
}
func (o grepReadOperations) ReadFile(context.Context, string) (string, error) {
	if o.mode == "denied" {
		return "", os.ErrPermission
	}
	return "remote before\nremote match\nremote after", nil
}
