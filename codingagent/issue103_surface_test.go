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

var updateIssue103Surface = flag.Bool("update-issue103-surface", false, "regenerate issue #103 API snapshot")

func TestIssue103SkillAPISnapshot(t *testing.T) {
	var out strings.Builder
	for _, value := range []any{codingagent.SkillFrontmatter{}, codingagent.Skill{}, codingagent.LoadSkillsFromDirOptions{}, codingagent.LoadSkillsResult{}, codingagent.SkillLoadResult{}, codingagent.SourceInfo{}, codingagent.ResourceDiagnostic{}, codingagent.ResourceCollision{}, codingagent.PromptOptions{}, codingagent.CreateHeadlessSessionOptions{}} {
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
	for _, f := range []any{codingagent.LoadSkills, codingagent.LoadSkillsFromDir, codingagent.FormatSkillsForPrompt, codingagent.ParseSkillBlock} {
		fmt.Fprintln(&out, reflect.TypeOf(f))
	}
	path := "testdata/issue103_surface_golden.txt"
	if *updateIssue103Surface {
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
		t.Fatal("skill API snapshot drifted; use -update-issue103-surface")
	}
}
