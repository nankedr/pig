package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	dir, err := os.MkdirTemp("", "pig-session-interop-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "history.jsonl")
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	// Explicit historical input; OpenSessionManager owns migration and subsequent writes.
	if err := os.WriteFile(path, []byte(`{"type":"session","version":2,"id":"pig-writer","timestamp":"2025-01-01T00:00:00.000Z","cwd":"/project","futureHeader":{"keep":true}}
{"type":"message","id":"historical","parentId":null,"timestamp":"2025-01-01T00:00:00.000Z","message":{"role":"user","content":"hi","timestamp":1},"futureEntry":{"keep":true}}
`), 0600); err != nil {
		return err
	}
	manager, err := codingagent.OpenSessionManager(path, nil, nil)
	if err != nil {
		return err
	}
	provider, err := ai.NewFauxProvider()
	if err != nil {
		return err
	}
	reply, err := ai.FauxAssistantMessage(ai.FauxAssistantText("continued history"), ai.FauxAssistantMessageOptions{Timestamp: ai.Some[int64](3)})
	if err != nil {
		return err
	}
	provider.SetResponses([]ai.FauxResponseStep{reply})
	model, _ := provider.GetModel()
	created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: dir, Model: &model, Provider: provider.Provider, SessionManager: manager})
	if err != nil {
		return err
	}
	if err := created.Session.Prompt(context.Background(), "continue"); err != nil {
		return err
	}
	if err := created.Session.Dispose(); err != nil {
		return err
	}
	for _, message := range []agent.AgentMessage{
		agent.CreateCustomMessage("note", ai.UserText("typed custom"), true, ai.Absent[ai.JSONValue](), 6),
		agent.CreateCustomMessage("null-details", ai.UserText("explicit null"), false, ai.Null[ai.JSONValue](), 7),
		agent.CreateBranchSummaryMessage("typed branch summary", "historical", 8),
		agent.CreateCompactionSummaryMessage("typed compaction summary", 9, 9),
	} {
		if _, err := manager.AppendMessage(message); err != nil {
			return err
		}
	}
	open, err := agent.NewRawAgentMessage(json.RawMessage(`{"role":"futureRole","payload":{"keep":[1,2]},"timestamp":4}`))
	if err != nil {
		return err
	}
	if _, err := manager.AppendMessage(open); err != nil {
		return err
	}
	reopened, err := codingagent.OpenSessionManager(path, nil, nil)
	if err != nil {
		return err
	}
	fmt.Printf("version=%d messages=%d\n", *reopened.GetHeader().Version, len(reopened.BuildSessionContext().Messages))
	return nil
}
