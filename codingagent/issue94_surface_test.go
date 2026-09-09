package codingagent_test

import (
	"flag"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
	"path/filepath"
)

var updateIssue94Surface = flag.Bool("update-issue94-surface", false, "regenerate issue #94 RPC API snapshot")

func TestIssue94RPCAPISnapshot(t *testing.T) {
	var out strings.Builder
	typ := reflect.TypeOf((*codingagent.RPCClient)(nil))
	for _, name := range []string{"Start", "Stop", "Prompt", "PromptAndWait", "Abort", "GetState", "GetMessages", "GetLastAssistantText", "GetStderr", "OnEvent", "CollectEvents", "WaitForIdle"} {
		m, ok := typ.MethodByName(name)
		if !ok {
			t.Fatal(name)
		}
		fmt.Fprintf(&out, "method %s %s\n", name, m.Type)
	}
	for _, v := range []any{codingagent.RPCClientOptions{}, codingagent.RPCSessionState{}, codingagent.RPCResponse{}} {
		typ := reflect.TypeOf(v)
		fmt.Fprintln(&out, issue71TypeSnapshot(typ.Name(), typ))
	}
	fmt.Fprintf(&out, "RunRPCMode %T\n", codingagent.RunRPCMode)
	path := "testdata/issue94_surface_golden.txt"
	if *updateIssue94Surface {
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
		t.Fatal("RPC API snapshot drift")
	}
}
func TestIssue94RPCFixtureIntegrity(t *testing.T) {
	root := issue32RepoRoot(t)
	lock, _, err := baseline.Load(filepath.Join(root, "parity/baseline"))
	if err != nil {
		t.Fatal(err)
	}
	_, err = parity.LoadFixture(filepath.Join(root, "parity/oracle/fixtures/rpc.json"), parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository})
	if err != nil {
		t.Fatal(err)
	}
}
