package tui_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
	"github.com/nankedr/pig/tui"
)

func TestKeyParity111(t *testing.T) {
	lock, _, err := baseline.Load("../parity/baseline")
	if err != nil {
		t.Fatal(err)
	}
	f, err := parity.LoadFixture("../parity/oracle/fixtures/keys.json", parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository})
	if err != nil {
		t.Fatal(err)
	}
	var input struct {
		Data []string
		Keys []tui.KeyID
	}
	type result struct {
		Key, Printable, KittyPrintable *string
		Release, Repeat                bool
		Matches                        []tui.KeyID
	}
	var want struct{ Parsed [][]result }
	if err = json.Unmarshal(f.Case.Input, &input); err != nil {
		t.Fatal(err)
	}
	if err = json.Unmarshal(f.Observation.Outcome, &want); err != nil {
		t.Fatal(err)
	}
	defer tui.SetKittyProtocolActive(false)
	for mode, active := range []bool{false, true} {
		if err = tui.SetKittyProtocolActive(active); err != nil {
			t.Fatal(err)
		}
		for i, data := range input.Data {
			got := result{Matches: []tui.KeyID{}}
			k, ok, err := tui.ParseKey(data)
			if err != nil {
				t.Fatal(err)
			}
			if ok {
				s := string(k)
				got.Key = &s
			}
			s, ok, err := tui.DecodePrintableKey(data)
			if err != nil {
				t.Fatal(err)
			}
			if ok {
				text := s
				got.Printable = &text
			}
			s, ok, err = tui.DecodeKittyPrintable(data)
			if err != nil {
				t.Fatal(err)
			}
			if ok {
				text := s
				got.KittyPrintable = &text
			}
			got.Release, err = tui.IsKeyRelease(data)
			if err != nil {
				t.Fatal(err)
			}
			got.Repeat, err = tui.IsKeyRepeat(data)
			if err != nil {
				t.Fatal(err)
			}
			for _, key := range input.Keys {
				match, err := tui.MatchesKey(data, key)
				if err != nil {
					t.Fatal(err)
				}
				if match {
					got.Matches = append(got.Matches, key)
				}
			}
			if !reflect.DeepEqual(got, want.Parsed[mode][i]) {
				a, _ := json.Marshal(got)
				b, _ := json.Marshal(want.Parsed[mode][i])
				t.Errorf("kitty=%t data=%q: got %s want %s", active, data, a, b)
			}
		}
	}
}

func TestTerminalSpecialCases111(t *testing.T) {
	for _, c := range []struct {
		data          string
		detect, shift bool
		want          string
	}{{"\r", true, true, "\x1b[13;2u"}, {"\r", false, true, "\r"}, {"\r", true, false, "\r"}, {"x", true, true, "x"}} {
		got, err := tui.NormalizeNativeShiftEnterInput(c.data, c.detect, c.shift)
		if err != nil || got != c.want {
			t.Fatalf("native %q %v", got, err)
		}
	}
	t.Setenv("WT_SESSION", "test")
	t.Setenv("SSH_CONNECTION", "")
	t.Setenv("SSH_CLIENT", "")
	t.Setenv("SSH_TTY", "")
	key, _, _ := tui.ParseKey("\b")
	if key != "ctrl+backspace" {
		t.Fatal(key)
	}
	t.Setenv("SSH_TTY", "/dev/pts/1")
	key, _, _ = tui.ParseKey("\b")
	if key != "backspace" {
		t.Fatal(key)
	}
	if _, err := tui.IsNativeModifierPressed(tui.ModifierKeyShift); err != nil {
		t.Fatal(err)
	}
}
