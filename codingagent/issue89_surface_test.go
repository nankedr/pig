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

var updateIssue89Surface = flag.Bool("update-issue89-surface", false, "regenerate issue #89 API snapshot")

func TestIssue89CompactionAPISnapshot(t *testing.T) {
	var out strings.Builder
	typ := reflect.TypeOf((*codingagent.AgentSession)(nil))
	for _, name := range []string{"Compact", "AbortCompaction", "IsCompacting"} {
		method, ok := typ.MethodByName(name)
		if !ok {
			t.Fatalf("missing %s", name)
		}
		fmt.Fprintf(&out, "method %s %s\n", name, method.Type)
	}
	for _, value := range []any{codingagent.SummaryOptions{}, codingagent.CompactionPreparation{}, codingagent.CompactionResult{}, codingagent.SummaryWithUsage{}, codingagent.AgentSessionCompactionEndEvent{}, codingagent.AppendCompactionOptions{}} {
		typ := reflect.TypeOf(value)
		fmt.Fprintln(&out, issue71TypeSnapshot(typ.Name(), typ))
	}
	for _, entry := range []struct {
		name string
		fn   any
	}{{"PrepareCompaction", codingagent.PrepareCompaction}, {"Compact", codingagent.Compact}, {"GenerateSummary", codingagent.GenerateSummary}, {"GenerateSummaryWithUsage", codingagent.GenerateSummaryWithUsage}, {"AppendCompaction", (*codingagent.SessionManager).AppendCompaction}} {
		fmt.Fprintf(&out, "func %s %s\n", entry.name, reflect.TypeOf(entry.fn))
	}
	path := "testdata/issue89_surface_golden.txt"
	if *updateIssue89Surface {
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
