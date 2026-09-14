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

var updateIssue101Surface = flag.Bool("update-issue101-surface", false, "regenerate issue #101 API snapshot")

func TestIssue101PromptAPISnapshot(t *testing.T) {
	var out strings.Builder
	for _, value := range []any{codingagent.CreateHeadlessSessionOptions{}, codingagent.ResourcePathSource{}, codingagent.DefaultResourceLoaderOptions{}, codingagent.ResourceLoaderReloadOptions{}, codingagent.AgentsFile{}, codingagent.ResourceDiagnostic{}, codingagent.CreateAgentSessionOptions{}} {
		typ := reflect.TypeOf(value)
		fmt.Fprintf(&out, "type %s\n", typ)
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			if f.IsExported() {
				fmt.Fprintf(&out, "- %s %s %q\n", f.Name, f.Type, f.Tag)
			}
		}
	}
	typ := reflect.TypeOf((*codingagent.DefaultResourceLoader)(nil))
	for i := 0; i < typ.NumMethod(); i++ {
		m := typ.Method(i)
		fmt.Fprintf(&out, "method %s %s\n", m.Name, m.Type)
	}
	for _, f := range []any{codingagent.NewDefaultResourceLoader, codingagent.LoadProjectContextFiles} {
		fmt.Fprintln(&out, reflect.TypeOf(f))
	}
	path := "testdata/issue101_surface_golden.txt"
	if *updateIssue101Surface {
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
		t.Fatal("system prompt API snapshot drifted; use -update-issue101-surface")
	}
}
