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

var updateIssue102Surface = flag.Bool("update-issue102-surface", false, "regenerate issue #102 API snapshot")

func TestIssue102TemplateAPISnapshot(t *testing.T) {
	var out strings.Builder
	for _, value := range []any{codingagent.PromptTemplate{}, codingagent.ParsedFrontmatter{}, codingagent.PromptTemplateLoadResult{}, codingagent.SourceInfo{}, codingagent.ResourceCollision{}, codingagent.PromptOptions{}, codingagent.CreateHeadlessSessionOptions{}} {
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
	for _, f := range []any{codingagent.ParseFrontmatter, codingagent.StripFrontmatter, (*codingagent.AgentSession).PromptTemplates} {
		fmt.Fprintln(&out, reflect.TypeOf(f))
	}
	path := "testdata/issue102_surface_golden.txt"
	if *updateIssue102Surface {
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
		t.Fatal("prompt template API snapshot drifted; use -update-issue102-surface")
	}
}
