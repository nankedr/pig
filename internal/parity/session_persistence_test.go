package parity_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
)

func TestSessionPersistenceParity(t *testing.T) {
	root := parityRepoRoot(t)
	lock, _, err := baseline.Load(filepath.Join(root, "parity", "baseline"))
	if err != nil {
		t.Fatal(err)
	}
	locked := parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository}
	fixture, err := parity.LoadFixture(filepath.Join(root, "parity", "oracle", "fixtures", "session-persistence.json"), locked)
	if err != nil {
		t.Fatal(err)
	}
	oracle, err := parity.NewFixtureDriver(fixture, locked)
	if err != nil {
		t.Fatal(err)
	}

	result, err := parity.RunCase(context.Background(), fixture.Case, oracle, parity.DriverFunc{
		SurfaceName: parity.SurfaceGoSDK,
		ObserveFunc: func(context.Context, parity.Case) (parity.Observation, error) {
			return observeSessionPersistence(t)
		},
	})
	if err != nil || !result.Match || len(result.Normalizations) != 0 {
		t.Fatalf("session persistence parity = %+v, %v", result, err)
	}
}

func observeSessionPersistence(t *testing.T) (parity.Observation, error) {
	t.Helper()
	dir := t.TempDir()
	cwd := filepath.Join(dir, "project")
	manager, err := codingagent.NewSessionManager(cwd, &dir, codingagent.NewSessionOptions{ID: "m3-session"})
	if err != nil {
		return parity.Observation{}, err
	}
	file := *manager.GetSessionFile()
	existence := []bool{fileExists(file)}
	if _, err = manager.AppendModelChange("fixture", "model-1"); err != nil {
		return parity.Observation{}, err
	}
	existence = append(existence, fileExists(file))
	if _, err = manager.AppendThinkingLevelChange("high"); err != nil {
		return parity.Observation{}, err
	}
	existence = append(existence, fileExists(file))
	if _, err = manager.AppendMessage(ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserBlocks(ai.TextContent{Type: ai.ContentTypeText, Text: "first"}), Timestamp: 1}); err != nil {
		return parity.Observation{}, err
	}
	existence = append(existence, fileExists(file))
	if _, err = manager.AppendMessage(ai.AssistantMessage{
		Role: ai.MessageRoleAssistant, Content: []ai.AssistantContent{ai.TextContent{Type: ai.ContentTypeText, Text: "partial"}},
		API: "fixture", Provider: "fixture", Model: "model-1", Usage: ai.Usage{Input: 1, Output: 1, TotalTokens: 2},
		StopReason: ai.StopReasonError, ErrorMessage: ai.Some("provider failed"), Timestamp: 2,
	}); err != nil {
		return parity.Observation{}, err
	}
	existence = append(existence, fileExists(file))
	if _, err = manager.AppendMessage(ai.ToolResultMessage{
		Role: ai.MessageRoleToolResult, ToolCallID: "call-1", ToolName: "read",
		Content: []ai.ToolResultContent{ai.TextContent{Type: ai.ContentTypeText, Text: "tool output"}}, Timestamp: 3,
	}); err != nil {
		return parity.Observation{}, err
	}

	firstData, err := os.ReadFile(file)
	if err != nil {
		return parity.Observation{}, err
	}
	parsed := codingagent.ParseSessionEntries(string(firstData))
	records := make([]map[string]any, 0, len(parsed)-1)
	indices := map[string]int{}
	for _, item := range parsed[1:] {
		if item.Entry != nil {
			indices[item.Entry.ID] = len(indices)
		}
	}
	for _, item := range parsed[1:] {
		if item.Entry == nil {
			continue
		}
		entry := item.Entry
		projected := map[string]any{"type": entry.Type}
		if entry.ParentID == nil {
			projected["parent"] = nil
		} else {
			projected["parent"] = indices[*entry.ParentID]
		}
		switch entry.Type {
		case "model_change":
			projected["provider"], projected["modelId"] = entry.Provider, entry.ModelID
		case "thinking_level_change":
			projected["thinkingLevel"] = entry.ThinkingLevel
		case "message":
			projected["role"] = entry.Message.MessageRole()
		}
		records = append(records, projected)
	}

	reopened, err := codingagent.OpenSessionManager(file, nil, nil)
	if err != nil {
		return parity.Observation{}, err
	}
	beforeAppend, err := os.ReadFile(file)
	if err != nil {
		return parity.Observation{}, err
	}
	if _, err = reopened.AppendMessage(ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserBlocks(ai.TextContent{Type: ai.ContentTypeText, Text: "second"}), Timestamp: 4}); err != nil {
		return parity.Observation{}, err
	}
	afterAppend, err := os.ReadFile(file)
	if err != nil {
		return parity.Observation{}, err
	}
	roles := []ai.MessageRole{}
	for _, message := range reopened.BuildSessionContext().Messages {
		roles = append(roles, message.MessageRole())
	}

	memory := codingagent.NewInMemorySessionManager(cwd, codingagent.NewSessionOptions{ID: "memory-session"})
	if _, err = memory.AppendMessage(ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("memory"), Timestamp: 5}); err != nil {
		return parity.Observation{}, err
	}
	emptyPath := filepath.Join(dir, "empty.jsonl")
	if err = os.WriteFile(emptyPath, nil, 0o600); err != nil {
		return parity.Observation{}, err
	}
	empty, err := codingagent.OpenSessionManager(emptyPath, nil, nil)
	if err != nil {
		return parity.Observation{}, err
	}
	emptyData, err := os.ReadFile(emptyPath)
	if err != nil {
		return parity.Observation{}, err
	}
	invalidPath := filepath.Join(dir, "invalid.jsonl")
	invalidContent := []byte("{\"type\":\"message\",\"id\":\"orphan\"}\n")
	if err = os.WriteFile(invalidPath, invalidContent, 0o600); err != nil {
		return parity.Observation{}, err
	}
	_, invalidErr := codingagent.OpenSessionManager(invalidPath, nil, nil)
	invalidAfter, err := os.ReadFile(invalidPath)
	if err != nil {
		return parity.Observation{}, err
	}

	header := manager.GetHeader()
	outcome, err := json.Marshal(map[string]any{
		"created": map[string]any{
			"id": manager.GetSessionID(), "fileHasID": strings.HasSuffix(filepath.Base(file), "_m3-session.jsonl"),
			"existence": existence, "records": records,
			"header": map[string]any{"type": header.Type, "version": *header.Version, "id": header.ID, "cwdMatches": header.CWD == cwd},
		},
		"reopened": map[string]any{
			"id": reopened.GetSessionID(), "roles": roles, "unchangedBeforeAppend": strings.HasPrefix(string(afterAppend), string(beforeAppend)),
			"appendedLines": len(strings.Split(strings.TrimSpace(string(afterAppend)), "\n")) - len(strings.Split(strings.TrimSpace(string(beforeAppend)), "\n")),
		},
		"memory":  map[string]any{"persisted": memory.IsPersisted(), "file": memory.GetSessionFile()},
		"empty":   map[string]any{"idMatchesHeader": empty.GetSessionID() == empty.GetHeader().ID, "lines": len(strings.Split(strings.TrimSpace(string(emptyData)), "\n"))},
		"invalid": map[string]any{"failed": invalidErr != nil && strings.Contains(invalidErr.Error(), "not a valid pi session"), "preserved": string(invalidAfter) == string(invalidContent)},
	})
	if err != nil {
		return parity.Observation{}, err
	}
	effects := []parity.SideEffect{
		{Kind: "file-create", Target: "session", Detail: json.RawMessage(`"after-first-assistant"`)},
		{Kind: "file-write", Target: "explicit-empty", Detail: json.RawMessage(`"header"`)},
		{Kind: "file-preserve", Target: "explicit-invalid", Detail: json.RawMessage(`"unchanged"`)},
	}
	return parity.Observation{Outcome: outcome, SideEffects: &effects}, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
