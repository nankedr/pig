package codingagent_test

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/catalog"
)

var updateIssue73Surface = flag.Bool("update-issue73-surface", false, "regenerate issue #73 API snapshot")

func TestIssue73LockedGoAPISnapshot(t *testing.T) {
	sections := []string{}
	for _, fn := range []struct {
		name  string
		value any
	}{{"ListSessions", codingagent.ListSessions}, {"ListAllSessions", codingagent.ListAllSessions}, {"ContinueRecentSessionManager", codingagent.ContinueRecentSessionManager}, {"ForkSessionManager", codingagent.ForkSessionManager}} {
		sections = append(sections, "func codingagent."+fn.name+" "+reflect.TypeOf(fn.value).String())
	}
	for _, v := range []any{codingagent.SessionInfo{}, codingagent.SessionListOptions{}, codingagent.SessionTreeNode{}, codingagent.SwitchSessionOptions{}, codingagent.NewRuntimeSessionOptions{}, codingagent.ForkSessionOptions{}, codingagent.SessionReplacementResult{}} {
		typ := reflect.TypeOf(v)
		sections = append(sections, issue71TypeSnapshot(typ.Name(), typ))
	}
	for _, typ := range []reflect.Type{reflect.TypeOf((*codingagent.SessionManager)(nil)), reflect.TypeOf((*codingagent.AgentSessionRuntime)(nil))} {
		for i := 0; i < typ.NumMethod(); i++ {
			m := typ.Method(i)
			sections = append(sections, "method "+typ.String()+"."+m.Name+" "+m.Type.String())
		}
	}
	got := strings.Join(sections, "\n\n") + "\n"
	path := filepath.Join("testdata", "issue73_surface_golden.txt")
	if *updateIssue73Surface {
		if err := os.WriteFile(path, []byte(got), 0644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatal("issue #73 API drift; regenerate with -update-issue73-surface")
	}
}

func issue73SessionEvidence(t *testing.T, id string) []issue32ModuleEvidenceDescriptor {
	t.Helper()
	result := []issue32ModuleEvidenceDescriptor{}
	for _, item := range []struct{ name, test string }{{"session-navigation", "TestSessionNavigationParity"}, {"session-tree", "TestSessionTreeParity"}} {
		path := "parity/oracle/fixtures/" + item.name + ".json"
		data, err := os.ReadFile(filepath.Join(issue32RepoRoot(t), path))
		if err != nil {
			t.Fatal(err)
		}
		var fixture struct {
			InputHash       string `json:"input_hash"`
			ObservationHash string `json:"observation_hash"`
		}
		if err := json.Unmarshal(data, &fixture); err != nil {
			t.Fatal(err)
		}
		for _, kind := range []string{"oracle", "go-test"} {
			ref, run := path, "node --experimental-strip-types parity/oracle/"+item.name+".mjs <locked-pi-checkout> --check"
			if kind == "go-test" {
				ref = "internal/parity/" + strings.ReplaceAll(item.name, "-", "_") + "_test.go#" + item.test
				run = "go test ./internal/parity -run '^" + item.test + "$' -count=1"
			}
			result = append(result, issue32ModuleEvidenceDescriptor{InputPath: path, PinnedInputHash: fixture.InputHash, Evidence: catalog.Evidence{Kind: kind, Ref: ref, Baseline: issue32BaselineCommit, CaseID: "go-sdk/codingagent/" + item.name, InputHash: fixture.InputHash, ExecutionMethod: run, Expected: "Issue #73 Session discovery and selected-path fork match the locked Pi public SessionManager boundary without normalizations", Actual: "PASS; observation " + fixture.ObservationHash, Platform: "any", CatalogID: id}})
		}
	}
	for _, item := range []struct{ path, test, run string }{{"codingagent/issue73_runtime_test.go", "TestSessionRuntimeReplacementLifecycle", "go test -race ./codingagent -run '^TestSessionRuntimeReplacementLifecycle$' -count=1"}, {"cmd/pig/issue73_process_test.go", "TestPigContinueSelectAndForkSessions", "go test ./cmd/pig -run '^TestPigContinueSelectAndForkSessions$' -count=1"}} {
		result = append(result, issue32ModuleEvidenceDescriptor{InputPath: item.path, Evidence: catalog.Evidence{Kind: "go-test", Ref: item.path + "#" + item.test, Baseline: issue32BaselineCommit, CaseID: "issue73-" + item.test, ExecutionMethod: item.run, Expected: "Issue #73 public CLI/SDK continues and forks independent Sessions with observable replacement errors and cleanup", Actual: "PASS; history, source isolation and replacement lifecycle assertions passed", Platform: "any", CatalogID: id}})
	}
	return result
}
