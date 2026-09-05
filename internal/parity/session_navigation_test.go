package parity_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
)

func TestSessionNavigationParity(t *testing.T) {
	root := parityRepoRoot(t)
	lock, _, err := baseline.Load(filepath.Join(root, "parity", "baseline"))
	if err != nil {
		t.Fatal(err)
	}
	locked := parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository}
	fixture, err := parity.LoadFixture(filepath.Join(root, "parity", "oracle", "fixtures", "session-navigation.json"), locked)
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
			return observeSessionNavigation(t)
		},
	})
	if err != nil || !result.Match || len(result.Normalizations) != 0 {
		t.Fatalf("session persistence parity = %+v, %v", result, err)
	}
}

func observeSessionNavigation(t *testing.T) (parity.Observation, error) {
	t.Helper()
	dir := t.TempDir()
	cwd := filepath.Join(dir, "project")
	makeSession := func(id, project string, timestamp int64) *codingagent.SessionManager {
		m, err := codingagent.NewSessionManager(project, &dir, codingagent.NewSessionOptions{ID: id})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = m.AppendMessage(ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("hello"), Timestamp: timestamp - 1}); err != nil {
			t.Fatal(err)
		}
		reply, err := ai.FauxAssistantMessage(ai.FauxAssistantText("reply"), ai.FauxAssistantMessageOptions{Timestamp: ai.Some(timestamp)})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = m.AppendMessage(reply); err != nil {
			t.Fatal(err)
		}
		return m
	}
	a, b, c := makeSession("local-a", cwd, 2001), makeSession("local-b", cwd, 3001), makeSession("other", filepath.Join(dir, "other"), 4001)
	if _, err := a.AppendSessionInfo("  named\r\n session  "); err != nil {
		return parity.Observation{}, err
	}
	if _, err := b.AppendSessionInfo("cleared"); err != nil {
		return parity.Observation{}, err
	}
	if _, err := b.AppendSessionInfo(" "); err != nil {
		return parity.Observation{}, err
	}
	if err := os.WriteFile(filepath.Join(dir, "invalid.jsonl"), []byte("invalid"), 0600); err != nil {
		t.Fatal(err)
	}
	for m, seconds := range map[*codingagent.SessionManager]int64{a: 10, b: 5, c: 20} {
		stamp := time.Unix(seconds, 0)
		if err := os.Chtimes(*m.GetSessionFile(), stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}
	progress := [][2]int{}
	local, err := codingagent.ListSessions(context.Background(), cwd, codingagent.SessionListOptions{SessionDir: &dir, OnProgress: func(n, total int) { progress = append(progress, [2]int{n, total}) }})
	if err != nil {
		return parity.Observation{}, err
	}
	all, err := codingagent.ListAllSessions(context.Background(), codingagent.SessionListOptions{SessionDir: &dir})
	if err != nil {
		return parity.Observation{}, err
	}
	recent, err := codingagent.ContinueRecentSessionManager(cwd, &dir)
	if err != nil {
		return parity.Observation{}, err
	}
	rows := []map[string]any{}
	ids := []string{}
	for _, s := range local {
		rows = append(rows, map[string]any{"id": s.ID, "name": s.Name, "modified": s.Modified.UnixMilli(), "count": s.MessageCount, "first": s.FirstMessage, "text": s.AllMessagesText})
	}
	for _, s := range all {
		ids = append(ids, s.ID)
	}
	outcome, err := json.Marshal(map[string]any{"local": rows, "all": ids, "recent": recent.GetSessionID(), "progress": progress})
	effects := []parity.SideEffect{}
	return parity.Observation{Outcome: outcome, SideEffects: &effects}, err
}
