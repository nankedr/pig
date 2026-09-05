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

func TestSessionTreeParity(t *testing.T) {
	root := parityRepoRoot(t)
	lock, _, err := baseline.Load(filepath.Join(root, "parity", "baseline"))
	if err != nil {
		t.Fatal(err)
	}
	locked := parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository}
	fixture, err := parity.LoadFixture(filepath.Join(root, "parity", "oracle", "fixtures", "session-tree.json"), locked)
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
			return observeSessionTree(t)
		},
	})
	if err != nil || !result.Match || len(result.Normalizations) != 0 {
		t.Fatalf("session persistence parity = %+v, %v", result, err)
	}
}

func observeSessionTree(t *testing.T) (parity.Observation, error) {
	dir := t.TempDir()
	cwd := filepath.Join(dir, "project")
	m, err := codingagent.NewSessionManager(cwd, &dir, codingagent.NewSessionOptions{ID: "source"})
	if err != nil {
		return parity.Observation{}, err
	}
	mustID := func(id string, err error) string {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	user := func(text string) ai.UserMessage {
		return ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText(text), Timestamp: 1}
	}
	mustID(m.AppendSessionInfo("path name"))
	u := mustID(m.AppendMessage(user("first")))
	label := "checkpoint"
	mustID(m.AppendLabelChange(u, &label))
	reply, err := ai.FauxAssistantMessage(ai.FauxAssistantText("reply"))
	must(err)
	a := mustID(m.AppendMessage(reply))
	mustID(m.AppendMessage(user("abandoned")))
	must(m.Branch(a))
	selected := mustID(m.AppendMessage(user("selected")))
	label = "latest"
	mustID(m.AppendLabelChange(u, &label))
	source := *m.GetSessionFile()
	before, err := os.ReadFile(source)
	must(err)
	re, err := codingagent.OpenSessionManager(source, nil, nil)
	must(err)
	childEntries, err := re.GetChildren(a)
	must(err)
	children := []string{}
	for _, e := range childEntries {
		text, _ := e.Message.(ai.UserMessage).Content.Text()
		children = append(children, text)
	}
	tree, err := re.GetTree()
	must(err)
	labelTime := *tree[0].Children[0].LabelTimestamp
	fork, err := re.CreateBranchedSession(selected)
	must(err)
	opened, err := codingagent.OpenSessionManager(*fork, nil, nil)
	must(err)
	entries := opened.GetEntries()
	ids := map[string]bool{}
	for _, e := range entries {
		ids[e.ID] = true
	}
	parentsValid := true
	for _, e := range entries {
		if e.ParentID != nil && !ids[*e.ParentID] {
			parentsValid = false
		}
	}
	roles := []ai.MessageRole{}
	texts := []string{}
	for _, msg := range opened.BuildSessionContext().Messages {
		roles = append(roles, msg.MessageRole())
		if u, ok := msg.(ai.UserMessage); ok {
			text, _ := u.Content.Text()
			texts = append(texts, text)
		}
	}
	after, err := os.ReadFile(source)
	must(err)
	tree, err = opened.GetTree()
	must(err)
	out := map[string]any{"children": children, "name": opened.GetSessionName(), "label": opened.GetLabel(u), "roles": roles, "texts": texts, "parentsValid": parentsValid, "labelTimePreserved": *tree[0].Children[0].LabelTimestamp == labelTime, "sourceUnchanged": string(after) == string(before), "parentMatches": *opened.GetHeader().ParentSession == source, "idChanged": opened.GetSessionID() != m.GetSessionID(), "leafType": opened.GetLeafEntry().Type}
	mustID(opened.AppendMessage(user("independent")))
	after, err = os.ReadFile(source)
	must(err)
	out["sourceUnchangedAfterAppend"] = string(after) == string(before)
	must(opened.ResetLeaf())
	out["resetEmpty"] = len(opened.BuildSessionContext().Messages) == 0
	failures := []string{}
	for _, action := range []func() error{func() error { return opened.Branch("missing") }, func() error { _, err := opened.AppendLabelChange("missing", &label); return err }, func() error { _, err := opened.CreateBranchedSession("missing"); return err }} {
		if err := action(); err != nil {
			failures = append(failures, err.Error())
		}
	}
	out["errors"] = failures
	outcome, err := json.Marshal(out)
	effects := []parity.SideEffect{}
	return parity.Observation{Outcome: outcome, SideEffects: &effects}, err
}
