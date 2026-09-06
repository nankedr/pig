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

var updateIssue75Surface = flag.Bool("update-issue75-surface", false, "regenerate issue #75 API snapshot")

func TestIssue75LockedGoAPISnapshot(t *testing.T) {
	var out strings.Builder
	for _, value := range []any{codingagent.ProjectTrustStoreEntry{}, codingagent.ProjectTrustUpdate{}, codingagent.SettingsManagerCreateOptions{}, codingagent.CreateHeadlessSessionOptions{}} {
		typ := reflect.TypeOf(value)
		fmt.Fprintf(&out, "type %s\n", typ)
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			if f.IsExported() {
				fmt.Fprintf(&out, "- %s %s %q\n", f.Name, f.Type, f.Tag)
			}
		}
	}
	typ := reflect.TypeOf(codingagent.ProjectTrustStore{})
	for i := 0; i < typ.NumMethod(); i++ {
		m := typ.Method(i)
		fmt.Fprintf(&out, "method %s %s\n", m.Name, m.Type)
	}
	for _, f := range []any{codingagent.NewProjectTrustStore, codingagent.HasTrustRequiringProjectResources, codingagent.LoadProjectContextFiles} {
		fmt.Fprintln(&out, reflect.TypeOf(f))
	}
	path := "testdata/issue75_surface_golden.txt"
	if *updateIssue75Surface {
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
		t.Fatal("trust API snapshot drifted; use -update-issue75-surface")
	}
}
