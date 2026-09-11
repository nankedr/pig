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

var updateIssue100Surface = flag.Bool("update-issue100-surface", false, "regenerate issue #100 API snapshot")

func TestIssue100ContextAPISnapshot(t *testing.T) {
	var out strings.Builder
	for _, value := range []any{codingagent.DefaultResourceLoaderOptions{}, codingagent.ResourceLoaderReloadOptions{}, codingagent.AgentsFile{}, codingagent.ResourceDiagnostic{}, codingagent.CreateAgentSessionOptions{}} {
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
	path := "testdata/issue100_surface_golden.txt"
	if *updateIssue100Surface {
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
		t.Fatal("credential API snapshot drifted; use -update-issue100-surface")
	}
}
