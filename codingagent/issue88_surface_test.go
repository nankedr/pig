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

var updateIssue88Surface = flag.Bool("update-issue88-surface", false, "regenerate issue #88 API snapshot")

func TestIssue88StatsAPISnapshot(t *testing.T) {
	var out strings.Builder
	typ := reflect.TypeOf((*codingagent.AgentSession)(nil))
	for _, name := range []string{"GetSessionStats", "GetContextUsage", "GetLastAssistantText", "Messages", "SessionFile", "SessionID"} {
		method, ok := typ.MethodByName(name)
		if !ok {
			t.Fatalf("missing %s", name)
		}
		fmt.Fprintf(&out, "method %s %s\n", name, method.Type)
	}
	for _, value := range []any{codingagent.SessionStats{}, codingagent.ContextUsage{}} {
		typ := reflect.TypeOf(value)
		fmt.Fprintln(&out, issue71TypeSnapshot(typ.Name(), typ))
	}
	path := "testdata/issue88_surface_golden.txt"
	if *updateIssue88Surface {
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
		t.Fatal("stats API snapshot drift")
	}
}
