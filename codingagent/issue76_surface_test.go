package codingagent_test

import (
	"flag"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

var updateIssue76Surface = flag.Bool("update-issue76-surface", false, "regenerate issue #76 API snapshot")

func TestIssue76LockedGoAPISnapshot(t *testing.T) {
	var out strings.Builder
	for _, value := range []any{ai.APIKeyCredential{}, codingagent.CreateHeadlessSessionOptions{}} {
		typ := reflect.TypeOf(value)
		fmt.Fprintf(&out, "type %s\n", typ)
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			if f.IsExported() {
				fmt.Fprintf(&out, "- %s %s %q\n", f.Name, f.Type, f.Tag)
			}
		}
	}
	typ := reflect.TypeOf((*codingagent.AuthStorage)(nil))
	for i := 0; i < typ.NumMethod(); i++ {
		m := typ.Method(i)
		fmt.Fprintf(&out, "method %s %s\n", m.Name, m.Type)
	}
	for _, f := range []any{codingagent.NewAuthStorage, codingagent.ResolveAuthPath, codingagent.ReadStoredCredential} {
		fmt.Fprintln(&out, reflect.TypeOf(f))
	}
	path := "testdata/issue76_surface_golden.txt"
	if *updateIssue76Surface {
		if err := os.WriteFile(path, []byte(out.String()), 0644); err != nil {
			t.Fatal(err)
		}
		return
	}
	expected, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(expected) != out.String() {
		t.Fatal("credential API snapshot drifted; use -update-issue76-surface")
	}
}
