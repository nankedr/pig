package codingagent_test

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func TestBranchSelectionRuntime119(t *testing.T) {
	for _, position := range []int{0, 1} {
		t.Run([]string{"first", "second"}[position], func(t *testing.T) {
			ctx := context.Background()
			runtime, _ := sessionRuntime118(t)
			for _, text := range []string{"first question", "second question", "third question"} {
				if err := runtime.Session().Prompt(ctx, text); err != nil {
					t.Fatal(err)
				}
			}
			source := runtime.Session()
			sourcePath := *source.SessionFile()
			original, _ := os.ReadFile(sourcePath)
			before := tree91Snapshot(t, source)
			if _, err := source.NavigateTree(ctx, "missing"); err == nil {
				t.Fatal("invalid node accepted")
			}
			if _, err := runtime.Fork(ctx, "missing"); err == nil {
				t.Fatal("invalid fork accepted")
			}
			if runtime.Session() != source || before != tree91Snapshot(t, source) {
				t.Fatal("invalid node switched history")
			}
			messages, err := source.GetUserMessagesForForking()
			if err != nil {
				t.Fatal(err)
			}
			selected := ""
			selector := codingagent.NewUserMessageSelectorComponent(messages, func(id string) { selected = id }, nil, messages[position].EntryID)
			if err = selector.HandleInput("\r"); err != nil {
				t.Fatal(err)
			}
			result, err := runtime.Fork(ctx, selected)
			if err != nil {
				t.Fatal(err)
			}
			if result.SelectedText == nil || *result.SelectedText != messages[position].Text {
				t.Fatal(result)
			}
			fork := runtime.Session()
			users := []string{}
			for _, m := range fork.Messages() {
				if m.MessageRole() == ai.MessageRoleUser {
					users = append(users, message85Text(m))
				}
			}
			want := []string{}
			if position == 1 {
				want = append(want, "first question")
			}
			if !reflect.DeepEqual(users, want) {
				t.Fatal(users, want)
			}
			if err = fork.Prompt(ctx, *result.SelectedText+" revised"); err != nil {
				t.Fatal(err)
			}
			reopened, err := codingagent.OpenSessionManager(*fork.SessionFile(), nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			users = nil
			for _, m := range reopened.BuildSessionContext().Messages {
				if m.MessageRole() == ai.MessageRoleUser {
					users = append(users, message85Text(m))
				}
			}
			want = append(want, *result.SelectedText+" revised")
			if !reflect.DeepEqual(users, want) {
				t.Fatal(users, want)
			}
			after, _ := os.ReadFile(sourcePath)
			if string(after) != string(original) {
				t.Fatal("fork modified original")
			}
			old, err := codingagent.OpenSessionManager(sourcePath, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if len(old.BuildSessionContext().Messages) != 6 {
				t.Fatal("original history truncated")
			}
		})
	}
}

func TestTreeSelectorFailedLabelAndSnapshot119(t *testing.T) {
	session, _, _ := tree91Session(t)
	manager := session.SessionManager()
	tree, err := manager.GetTree()
	if err != nil {
		t.Fatal(err)
	}
	before := tree91Snapshot(t, session)
	selector := codingagent.NewTreeSelectorComponent(tree, manager.GetLeafID(), 24, nil, nil, codingagent.TreeSelectorOptions{OnLabelChange: func(id string, label *string) error {
		_, err := manager.AppendLabelChange("missing", label)
		return err
	}})
	for _, key := range []string{"L", "unsaved label", "\r"} {
		if err = selector.HandleInput(key); err != nil {
			t.Fatal(err)
		}
	}
	lines, err := selector.Render(80)
	if err != nil {
		t.Fatal(err)
	}
	if text := strings.Join(lines, "\n"); !strings.Contains(text, "Entry missing not found") || !strings.Contains(text, "unsaved label") {
		t.Fatal(text)
	}
	if tree91Snapshot(t, session) != before {
		t.Fatal("failed label changed history")
	}
	if err = selector.HandleInput("\x1b"); err != nil {
		t.Fatal(err)
	}
	if tree[0].Label != nil {
		t.Fatal("caller tree mutated")
	}
}

func TestTreeLabelWriteFailure119(t *testing.T) {
	session, _, _ := tree91Session(t)
	manager := session.SessionManager()
	path := *manager.GetSessionFile()
	entryCount := len(manager.GetEntries())
	before := tree91Snapshot(t, session)
	tree, err := manager.GetTree()
	if err != nil {
		t.Fatal(err)
	}
	leaf := *manager.GetLeafID()
	target := manager.GetEntries()[1].ID
	selector := codingagent.NewTreeSelectorComponent(tree, &leaf, 24, nil, nil, codingagent.TreeSelectorOptions{InitialSelectedID: target, OnLabelChange: func(id string, label *string) error { _, err := manager.AppendLabelChange(id, label); return err }})
	if err = os.Rename(path, path+".saved"); err != nil {
		t.Fatal(err)
	}
	if err = os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"L", "checkpoint", "\r"} {
		if err = selector.HandleInput(key); err != nil {
			t.Fatal(err)
		}
	}
	if tree91Snapshot(t, session) != before {
		t.Fatal("failed label write changed Session entries or leaf")
	}
	lines, err := selector.Render(80)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(lines, "\n"), "Label (empty to remove)") {
		t.Fatal("failed label write closed editor")
	}
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(path+".saved", path); err != nil {
		t.Fatal(err)
	}
	if err = selector.HandleInput("\r"); err != nil {
		t.Fatal(err)
	}
	reopened, err := codingagent.OpenSessionManager(path, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	label := reopened.GetLabel(target)
	if label == nil || *label != "checkpoint" {
		t.Fatal("label not saved after retry")
	}
	entries := reopened.GetEntries()
	if len(entries) != entryCount+1 {
		t.Fatal("unexpected label entries", len(entries))
	}
	saved := reopened.GetEntry(*reopened.GetLeafID())
	if saved.ParentID == nil || *saved.ParentID != leaf {
		t.Fatal("label linked to failed write")
	}
}
