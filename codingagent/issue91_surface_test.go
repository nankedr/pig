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

var updateIssue91Surface = flag.Bool("update-issue91-surface", false, "regenerate issue #91 API snapshot")

func TestIssue91TreeNavigationAPISnapshot(t *testing.T) {
	var out strings.Builder
	typ := reflect.TypeOf((*codingagent.AgentSession)(nil))
	for _, name := range []string{"NavigateTree"} {
		method, ok := typ.MethodByName(name)
		if !ok {
			t.Fatalf("missing %s", name)
		}
		fmt.Fprintf(&out, "method %s %s\n", name, method.Type)
	}
	for _, value := range []any{codingagent.NavigateTreeOptions{}, codingagent.NavigateTreeResult{}} {
		typ := reflect.TypeOf(value)
		fmt.Fprintln(&out, issue71TypeSnapshot(typ.Name(), typ))
	}
	path := "testdata/issue91_surface_golden.txt"
	if *updateIssue91Surface {
		if err := os.WriteFile(path, []byte(out.String()), 0644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != out.String() {
		t.Fatal("tree navigation API snapshot drift")
	}
}
