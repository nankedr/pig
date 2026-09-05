package codingagent_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func TestSessionBranchSummaryAndForkRawMetadataReopen(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "source.jsonl")
	raw := `{"type":"session","version":3,"id":"source","timestamp":"2026-01-01T00:00:00Z","cwd":"` + filepath.ToSlash(dir) + `"}` + "\n" + `{"type":"message","id":"u","parentId":null,"timestamp":"2026-01-01T00:00:00Z","extra":{"keep":true},"message":{"role":"user","content":"hello","timestamp":1}}` + "\n"
	if err := os.WriteFile(source, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	fork, err := codingagent.ForkSessionManager(source, dir, &dir)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(*fork.GetSessionFile())
	if err != nil || !strings.Contains(string(data), `"extra":{"keep":true}`) {
		t.Fatalf("raw lost: %s %v", data, err)
	}
	id := "u"
	usage := ai.Usage{Input: 3, Output: 4, TotalTokens: 7}
	hook := true
	summary, err := fork.BranchWithSummary(&id, "summary", codingagent.BranchSummaryOptions{Details: json.RawMessage(`{"key":"value"}`), Usage: &usage, FromHook: &hook})
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := codingagent.OpenSessionManager(*fork.GetSessionFile(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	entry := reopened.GetEntry(summary)
	if entry == nil || entry.Usage == nil || entry.Usage.TotalTokens != 7 || entry.FromID != "u" || entry.FromHook == nil || !*entry.FromHook {
		t.Fatalf("summary=%+v", entry)
	}
	if err := reopened.SetSessionFile(source); err != nil {
		t.Fatal(err)
	}
	if reopened.GetSessionID() != "source" {
		t.Fatal("SetSessionFile failed")
	}
	if _, err := reopened.NewSession(codingagent.NewSessionOptions{ID: "next", ParentSession: source}); err != nil {
		t.Fatal(err)
	}
	if reopened.GetSessionID() != "next" || len(reopened.GetEntries()) != 0 || *reopened.GetHeader().ParentSession != source {
		t.Fatal("new Session retained old state")
	}
}

func TestSessionDiscoveryDefaultDirectoriesAndCancellation(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PIG_CODING_AGENT_DIR", dir)
	cwd := filepath.Join(dir, "project")
	manager, err := codingagent.NewSessionManager(cwd, nil)
	if err != nil {
		t.Fatal(err)
	}
	reply, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("reply"))
	if _, err := manager.AppendMessage(reply); err != nil {
		t.Fatal(err)
	}
	recentDefault, err := codingagent.ContinueRecentSessionManager(cwd, nil)
	if err != nil || !recentDefault.UsesDefaultSessionDir() {
		t.Fatal("continue lost default directory", err)
	}
	alias := filepath.Join(dir, "sessions", "alias")
	if err := os.Symlink(manager.GetSessionDir(), alias); err != nil {
		t.Fatal(err)
	}
	all, err := codingagent.ListAllSessions(context.Background())
	if err != nil || len(all) != 2 {
		t.Fatalf("symlink discovery: %+v %v", all, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := codingagent.ListAllSessions(ctx); err != context.Canceled {
		t.Fatal(err)
	}
	empty := filepath.Join(dir, "empty")
	recent, err := codingagent.ContinueRecentSessionManager(empty, nil)
	if err != nil || len(recent.GetEntries()) != 0 {
		t.Fatal(recent, err)
	}
	if _, err := os.Stat(*recent.GetSessionFile()); !os.IsNotExist(err) {
		t.Fatal("empty continue wrote a file")
	}
}

func TestSessionDiscoveryKeepsLiteralNoMessagesFirstText(t *testing.T) {
	dir := t.TempDir()
	manager, err := codingagent.NewSessionManager(dir, &dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"(no messages)", "second"} {
		if _, err := manager.AppendMessage(ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText(text), Timestamp: 1}); err != nil {
			t.Fatal(err)
		}
	}
	reply, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("reply"))
	if _, err := manager.AppendMessage(reply); err != nil {
		t.Fatal(err)
	}
	list, err := codingagent.ListSessions(context.Background(), dir, codingagent.SessionListOptions{SessionDir: &dir})
	if err != nil || len(list) != 1 || list[0].FirstMessage != "(no messages)" {
		t.Fatal(list, err)
	}
}
