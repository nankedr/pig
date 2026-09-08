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

var updateIssue92Surface = flag.Bool("update-issue92-surface", false, "regenerate issue #92 API snapshot")

func TestIssue92BranchSummaryAPISnapshot(t *testing.T) {
	var out strings.Builder
	typ := reflect.TypeOf((*codingagent.AgentSession)(nil))
	for _, name := range []string{"NavigateTree", "AbortBranchSummary", "IsCompacting", "WaitForIdle"} {
		method, ok := typ.MethodByName(name)
		if !ok {
			t.Fatalf("missing %s", name)
		}
		fmt.Fprintf(&out, "method %s %s\n", name, method.Type)
	}
	for _, value := range []any{codingagent.NavigateTreeOptions{}, codingagent.NavigateTreeResult{}, codingagent.GenerateBranchSummaryOptions{}, codingagent.BranchSummaryResult{}, codingagent.BranchSummaryEntry{}, codingagent.AgentSessionBranchSummaryRetryAttemptStartEvent{}} {
		typ := reflect.TypeOf(value)
		fmt.Fprintln(&out, issue71TypeSnapshot(typ.Name(), typ))
	}
	fmt.Fprintf(&out, "function GenerateBranchSummary %s\n", reflect.TypeOf(codingagent.GenerateBranchSummary))
	path := "testdata/issue92_surface_golden.txt"
	if *updateIssue92Surface {
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
