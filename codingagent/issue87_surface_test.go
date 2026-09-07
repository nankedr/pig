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

var updateIssue87Surface = flag.Bool("update-issue87-surface", false, "regenerate issue #87 API snapshot")

func TestIssue87ConfigurationAPISnapshot(t *testing.T) {
	var out strings.Builder
	typ := reflect.TypeOf((*codingagent.AgentSession)(nil))
	for _, name := range []string{"SetModel", "CycleModel", "ScopedModels", "SetScopedModels", "SetThinkingLevel", "CycleThinkingLevel", "GetAvailableThinkingLevels", "SupportsThinking", "SetActiveToolsByName", "GetActiveToolNames", "GetAllTools"} {
		method, ok := typ.MethodByName(name)
		if !ok {
			t.Fatalf("missing %s", name)
		}
		fmt.Fprintf(&out, "method %s %s\n", name, method.Type)
	}
	for _, value := range []any{codingagent.ScopedModel{}, codingagent.ModelCycleResult{}, codingagent.AgentSessionThinkingLevelChangedEvent{}} {
		typ := reflect.TypeOf(value)
		fmt.Fprintln(&out, issue71TypeSnapshot(typ.Name(), typ))
	}
	path := "testdata/issue87_surface_golden.txt"
	if *updateIssue87Surface {
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
		t.Fatal("configuration API snapshot drift")
	}
}
