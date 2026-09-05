package codingagent_test

import (
	"flag"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/nankedr/pig/codingagent"
)

var updateIssue74Surface = flag.Bool("update-issue74-surface", false, "regenerate issue #74 API snapshot")

func TestIssue74LockedGoAPISnapshot(t *testing.T) {
	var out strings.Builder
	for _, value := range []any{codingagent.Settings{}, codingagent.SettingsManagerCreateOptions{}, codingagent.CreateHeadlessSessionOptions{}} {
		typ := reflect.TypeOf(value)
		fmt.Fprintf(&out, "type %s\n", typ)
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			if f.IsExported() {
				fmt.Fprintf(&out, "- %s %s %q\n", f.Name, f.Type, f.Tag)
			}
		}
	}
	typ := reflect.TypeOf(codingagent.SettingsManager{})
	for i := 0; i < typ.NumMethod(); i++ {
		m := typ.Method(i)
		fmt.Fprintf(&out, "method %s %s\n", m.Name, m.Type)
	}
	for _, f := range []any{codingagent.NewSettingsManager, codingagent.NewSettingsManagerFromStorage, codingagent.NewInMemorySettingsManager} {
		fmt.Fprintln(&out, reflect.TypeOf(f))
	}
	path := "testdata/issue74_surface_golden.txt"
	if *updateIssue74Surface {
		if err := os.WriteFile(path, []byte(out.String()), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	expected, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(expected) != out.String() {
		t.Fatal("settings API snapshot drifted; use -update-issue74-surface")
	}
}
