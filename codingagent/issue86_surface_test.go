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

var updateIssue86Surface = flag.Bool("update-issue86-surface", false, "regenerate issue #86 API snapshot")

func TestIssue86RetryAPISnapshot(t *testing.T) {
	var out strings.Builder
	typ := reflect.TypeOf((*codingagent.AgentSession)(nil))
	for _, name := range []string{"Prompt", "AbortRetry", "IsRetrying", "RetryAttempt", "AutoRetryEnabled", "SetAutoRetryEnabled", "WaitForIdle"} {
		method, ok := typ.MethodByName(name)
		if !ok {
			t.Fatalf("missing %s", name)
		}
		fmt.Fprintf(&out, "method %s %s\n", name, method.Type)
	}
	for _, value := range []any{codingagent.RetrySettings{}, codingagent.ProviderRetrySettings{}, codingagent.AgentSessionAutoRetryStartEvent{}, codingagent.AgentSessionAutoRetryEndEvent{}, codingagent.HeadlessOutcome{}} {
		typ := reflect.TypeOf(value)
		fmt.Fprintln(&out, issue71TypeSnapshot(typ.Name(), typ))
	}
	path := "testdata/issue86_surface_golden.txt"
	if *updateIssue86Surface {
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
		t.Fatal("retry API snapshot drift")
	}
}
