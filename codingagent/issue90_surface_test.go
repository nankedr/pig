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

var updateIssue90Surface = flag.Bool("update-issue90-surface", false, "regenerate issue #90 API snapshot")

func TestIssue90CompactionAPISnapshot(t *testing.T) {
	var out strings.Builder
	typ := reflect.TypeOf((*codingagent.AgentSession)(nil))
	for _, name := range []string{"AutoCompactionEnabled", "SetAutoCompactionEnabled", "AbortCompaction", "IsCompacting", "Prompt", "WaitForIdle"} {
		method, ok := typ.MethodByName(name)
		if !ok {
			t.Fatalf("missing %s", name)
		}
		fmt.Fprintf(&out, "method %s %s\n", name, method.Type)
	}
	for _, value := range []any{codingagent.AgentSessionCompactionStartEvent{}, codingagent.AgentSessionCompactionEndEvent{}, codingagent.AgentSessionCompactionRetryAttemptStartEvent{}, codingagent.CompactionSettings{}} {
		typ := reflect.TypeOf(value)
		fmt.Fprintln(&out, issue71TypeSnapshot(typ.Name(), typ))
	}
	path := "testdata/issue90_surface_golden.txt"
	if *updateIssue90Surface {
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
		t.Fatal("compaction API snapshot drift")
	}
}
