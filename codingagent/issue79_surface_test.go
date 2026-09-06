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

var updateIssue79Surface = flag.Bool("update-issue79-surface", false, "regenerate issue #79 API snapshot")

func TestIssue79LockedGoAPISnapshot(t *testing.T) {
	var out strings.Builder
	for _, value := range []any{codingagent.Edit{}, codingagent.EditToolInput{}, codingagent.EditToolDetails{}, codingagent.EditToolOptions{}, codingagent.ToolDefinition{}} {
		typ := reflect.TypeOf(value)
		fmt.Fprintf(&out, "type %s\n", typ)
		for i := 0; i < typ.NumField(); i++ {
			f := typ.Field(i)
			if f.IsExported() {
				fmt.Fprintf(&out, "- %s %s %q\n", f.Name, f.Type, f.Tag)
			}
		}
	}

	for _, f := range []any{codingagent.CreateEditTool, codingagent.CreateEditToolDefinition, codingagent.ToolExecuteFunc(nil), codingagent.WithFileMutationQueue[string]} {
		fmt.Fprintln(&out, reflect.TypeOf(f))
	}
	execute := reflect.TypeOf(codingagent.ToolExecuteFunc(nil))
	for i := 0; i < execute.NumIn(); i++ {
		fmt.Fprintf(&out, "Execute input %d %s\n", i, execute.In(i))
	}
	for i := 0; i < execute.NumOut(); i++ {
		fmt.Fprintf(&out, "Execute output %d %s\n", i, execute.Out(i))
	}
	typ := reflect.TypeOf((*codingagent.EditOperations)(nil)).Elem()
	for i := 0; i < typ.NumMethod(); i++ {
		m := typ.Method(i)
		fmt.Fprintf(&out, "method %s %s\n", m.Name, m.Type)
	}
	path := "testdata/issue79_surface_golden.txt"
	if *updateIssue79Surface {
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
		t.Fatal("edit Tool API snapshot drifted; use -update-issue79-surface")
	}
}
