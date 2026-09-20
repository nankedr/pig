package tui_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
	"github.com/nankedr/pig/tui"
)

func TestAutocompleteProviderParity115(t *testing.T) {
	lock, _, err := baseline.Load("../parity/baseline")
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := parity.LoadFixture("../parity/oracle/fixtures/autocomplete.json", parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository})
	if err != nil {
		t.Fatal(err)
	}
	var input struct {
		Files    []string
		FDScript string
		Cases    []struct {
			ID, Text string
			Col      *int
			Force    bool
			FD       any
		}
	}
	if err = json.Unmarshal(fixture.Case.Input, &input); err != nil {
		t.Fatal(err)
	}
	var want []struct {
		ID          string
		Suggestions *tui.AutocompleteSuggestions
		Application *tui.AutocompleteResult
		Trigger     bool
	}
	if err = json.Unmarshal(fixture.Observation.Outcome, &want); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	for _, name := range input.Files {
		path := filepath.Join(root, name)
		if err = os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, []byte("test"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	description, hint := "Review changes", "<file>"
	p := tui.NewCombinedAutocompleteProvider([]tui.AutocompleteEntry{tui.SlashCommand{Name: "review", Description: &description, ArgumentHint: &hint}, tui.SlashCommand{Name: "reload"}, tui.SlashCommand{Name: "resume"}, tui.SlashCommand{Name: "skill:中文"}, tui.SlashCommand{Name: "skill:review"}, tui.SlashCommand{Name: "model", GetArgumentCompletions: func(_ context.Context, s string) ([]tui.AutocompleteItem, bool, error) {
		if s == "d" {
			return []tui.AutocompleteItem{{Value: "deepseek/model", Label: "model"}}, true, nil
		}
		return nil, false, nil
	}}}, root, nil)
	for i, c := range input.Cases {
		t.Run(c.ID, func(t *testing.T) {
			text := strings.ReplaceAll(c.Text, "${ROOT}", root)
			col := len(text)
			if c.Col != nil {
				col = *c.Col
			}
			if c.FD != nil && runtime.GOOS == "windows" {
				t.Skip("controlled fd executable requires POSIX shell; six-platform acceptance remains M13")
			}
			provider := p
			if c.FD != nil {
				fd := filepath.Join(t.TempDir(), "fd")
				if c.FD == true {
					if err := os.WriteFile(fd, []byte(input.FDScript), 0755); err != nil {
						t.Fatal(err)
					}
				}
				provider = tui.NewCombinedAutocompleteProvider(nil, root, &fd)
			}
			got, ok, err := provider.GetSuggestions(context.Background(), []string{text}, 0, col, tui.AutocompleteOptions{Force: c.Force})
			if err != nil {
				t.Fatal(err)
			}
			if ok != (want[i].Suggestions != nil) {
				t.Fatalf("suggestions present=%v want %+v", ok, want[i].Suggestions)
			}
			if ok {
				for j := range got.Items {
					got.Items[j].Value = strings.ReplaceAll(got.Items[j].Value, root, "${ROOT}")
				}
				got.Prefix = strings.ReplaceAll(got.Prefix, root, "${ROOT}")
				if !reflect.DeepEqual(got, *want[i].Suggestions) {
					t.Fatalf("suggestions=%+v want %+v", got, *want[i].Suggestions)
				}
				item := got.Items[0]
				item.Value = strings.ReplaceAll(item.Value, "${ROOT}", root)
				applied, err := provider.ApplyCompletion([]string{text}, 0, col, item, strings.ReplaceAll(got.Prefix, "${ROOT}", root))
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(applied.Lines[0], root) {
					applied.CursorCol += 7 - len(root)
					applied.Lines[0] = strings.ReplaceAll(applied.Lines[0], root, "${ROOT}")
				}
				if !reflect.DeepEqual(applied, *want[i].Application) {
					t.Fatalf("application=%+v want %+v", applied, *want[i].Application)
				}
			}
			trigger, err := p.ShouldTriggerFileCompletion([]string{text}, 0, col)
			if err != nil || trigger != want[i].Trigger {
				t.Fatalf("trigger=%v err=%v", trigger, err)
			}
		})
	}
}

func TestAutocompleteEditorParity115(t *testing.T) {
	lock, _, err := baseline.Load("../parity/baseline")
	if err != nil {
		t.Fatal(err)
	}
	fixture, err := parity.LoadFixture("../parity/oracle/fixtures/autocomplete-editor.json", parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository})
	if err != nil {
		t.Fatal(err)
	}
	var input []struct {
		ID, Text string
		Steps    []string
	}
	type state struct {
		Text    string
		Showing bool
		Cursor  tui.EditorCursor
	}
	var want []struct {
		ID      string
		States  []state
		Submits []string
		Changes []string
	}
	if err = json.Unmarshal(fixture.Case.Input, &input); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(fixture.Observation.Outcome, &want); err != nil {
		t.Fatal(err)
	}
	for i, c := range input {
		t.Run(c.ID, func(t *testing.T) {
			e := tui.NewEditor(nil, tui.EditorTheme{})
			if err := e.SetAutocompleteProvider(tui.NewCombinedAutocompleteProvider([]tui.AutocompleteEntry{tui.SlashCommand{Name: "review"}, tui.SlashCommand{Name: "reload"}, tui.SlashCommand{Name: "resume"}, tui.SlashCommand{Name: "skill:中文"}}, "/nonexistent", nil)); err != nil {
				t.Fatal(err)
			}
			if c.Text != "" {
				_ = e.SetText(c.Text)
			}
			submits := []string{}
			changes := []string{}
			e.OnChange = func(s string) { changes = append(changes, s) }
			e.OnSubmit = func(s string) { submits = append(submits, s) }
			for j, input := range c.Steps {
				if err := e.HandleInput(input); err != nil {
					t.Fatal(err)
				}
				time.Sleep(120 * time.Millisecond)
				deadline := time.Now().Add(time.Second)
				var actual state
				for {
					showing := e.IsShowingAutocomplete()
					actual = state{e.GetText(), showing, e.GetCursor()}
					if reflect.DeepEqual(actual, want[i].States[j]) || time.Now().After(deadline) {
						break
					}
					time.Sleep(time.Millisecond)
				}
				if !reflect.DeepEqual(actual, want[i].States[j]) {
					t.Fatalf("step %d %q got %+v want %+v", j, input, actual, want[i].States[j])
				}
			}
			if !reflect.DeepEqual(changes, want[i].Changes) {
				t.Fatalf("changes=%q want %q", changes, want[i].Changes)
			}
			if !reflect.DeepEqual(submits, want[i].Submits) {
				t.Fatalf("submits=%v want %v", submits, want[i].Submits)
			}
		})
	}
}

type completionRequest115 struct {
	ctx      context.Context
	prefix   string
	reply    chan tui.AutocompleteSuggestions
	returned chan struct{}
}
type completionProvider115 struct {
	*tui.CombinedAutocompleteProvider
	requests chan completionRequest115
}

func (p completionProvider115) GetSuggestions(ctx context.Context, lines []string, line, col int, _ tui.AutocompleteOptions) (tui.AutocompleteSuggestions, bool, error) {
	req := completionRequest115{ctx, lines[line][:col], make(chan tui.AutocompleteSuggestions, 1), make(chan struct{})}
	p.requests <- req
	reply := <-req.reply
	close(req.returned)
	return reply, true, nil
}
func TestAutocompleteStaleAndCancel115(t *testing.T) {
	requests := make(chan completionRequest115, 5)
	e := tui.NewEditor(nil, tui.EditorTheme{})
	_ = e.SetAutocompleteProvider(completionProvider115{tui.NewCombinedAutocompleteProvider(nil, t.TempDir(), nil), requests})
	defer e.SetAutocompleteProvider(nil)
	next := func() completionRequest115 {
		t.Helper()
		select {
		case r := <-requests:
			return r
		case <-time.After(time.Second):
			t.Fatal("provider not requested")
			return completionRequest115{}
		}
	}
	reply := func(r completionRequest115, name string) {
		r.reply <- tui.AutocompleteSuggestions{Prefix: r.prefix, Items: []tui.AutocompleteItem{{Value: name, Label: name}}}
		<-r.returned
	}
	_ = e.HandleInput("/old")
	old := next()
	_ = e.SetText("")
	_ = e.HandleInput("/new")
	fresh := next()
	if old.ctx.Err() == nil {
		t.Fatal("old request not canceled")
	}
	reply(fresh, "new-command")
	deadline := time.Now().Add(time.Second)
	for !e.IsShowingAutocomplete() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !e.IsShowingAutocomplete() {
		t.Fatal("new suggestions missing")
	}
	reply(old, "old-command")
	_ = e.HandleInput("\t")
	if e.GetText() != "/new-command " {
		t.Fatalf("stale result won: %q", e.GetText())
	}
	_ = e.SetText("")
	_ = e.HandleInput("/cancel")
	pending := next()
	_ = e.HandleInput("\x1b")
	if pending.ctx.Err() == nil {
		t.Fatal("Escape did not cancel provider")
	}
	reply(pending, "wrong")
	if e.IsShowingAutocomplete() || e.GetText() != "/cancel" {
		t.Fatal("cancellation changed draft or reopened picker")
	}
	_ = e.SetText("")
	_ = e.HandleInput("/replace")
	replaced := next()
	_ = e.SetText("中文 draft")
	reply(replaced, "wrong")
	if e.IsShowingAutocomplete() || e.GetText() != "中文 draft" {
		t.Fatal("SetText accepted stale completion")
	}
}
