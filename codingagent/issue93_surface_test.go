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

var updateIssue93Surface = flag.Bool("update-issue93-surface", false, "regenerate issue #93 API snapshot")

func TestIssue93BashAPISnapshot(t *testing.T) {
	var out strings.Builder
	typ := reflect.TypeOf((*codingagent.AgentSession)(nil))
	for _, name := range []string{"ExecuteBash", "AbortBash", "RecordBashResult", "IsBashRunning", "HasPendingBashMessages"} {
		method, ok := typ.MethodByName(name)
		if !ok {
			t.Fatalf("missing %s", name)
		}
		fmt.Fprintf(&out, "method %s %s\n", name, method.Type)
	}
	for _, value := range []any{codingagent.ExecuteBashOptions{}, codingagent.RecordBashResultOptions{}, codingagent.BashResult{}, codingagent.AgentSessionBashExecutionUpdateEvent{}} {
		typ := reflect.TypeOf(value)
		fmt.Fprintln(&out, issue71TypeSnapshot(typ.Name(), typ))
	}
	path := "testdata/issue93_surface_golden.txt"
	if *updateIssue93Surface {
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
		t.Fatal("Bash API snapshot drift")
	}
}
