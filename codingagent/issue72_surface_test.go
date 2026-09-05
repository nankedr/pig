package codingagent_test

import (
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/codingagent"
)

var updateIssue72Surface = flag.Bool("update-issue72-surface", false, "regenerate issue #72 API snapshot")

func TestIssue72LockedGoAPISnapshot(t *testing.T) {
	sections := []string{
		issue71TypeSnapshot("SessionHeader", reflect.TypeOf(codingagent.SessionHeader{})),
		issue71TypeSnapshot("SessionEntry", reflect.TypeOf(codingagent.SessionEntry{})),
		issue71TypeSnapshot("FileEntry", reflect.TypeOf(codingagent.FileEntry{})),
		issue71TypeSnapshot("SessionContext", reflect.TypeOf(codingagent.SessionContext{})),
		"func codingagent.ConvertToLLM " + reflect.TypeOf(codingagent.ConvertToLLM).String(),
		"func codingagent.OpenSessionManager " + reflect.TypeOf(codingagent.OpenSessionManager).String(),
		"func codingagent.MigrateSessionEntries " + reflect.TypeOf(codingagent.MigrateSessionEntries).String(),
	}
	for _, typ := range []reflect.Type{reflect.TypeOf(agent.CustomMessage{}), reflect.TypeOf(agent.BranchSummaryMessage{}), reflect.TypeOf(agent.CompactionSummaryMessage{})} {
		method, ok := typ.MethodByName("CloneAgentMessage")
		if !ok {
			t.Fatalf("%s.CloneAgentMessage is missing", typ)
		}
		sections = append(sections, "method "+typ.String()+".CloneAgentMessage "+method.Type.String())
	}
	manager := reflect.TypeOf((*codingagent.SessionManager)(nil))
	for _, name := range []string{"AppendMessage", "GetEntries", "GetEntry", "GetLabel", "GetSessionName", "GetBranch", "BuildContextEntries", "BuildSessionContext", "GetHeader", "GetSessionFile", "IsPersisted", "UsesDefaultSessionDir"} {
		method, ok := manager.MethodByName(name)
		if !ok {
			t.Fatalf("SessionManager.%s is missing", name)
		}
		sections = append(sections, "method codingagent.SessionManager."+name+" "+method.Type.String())
	}
	got := strings.Join(sections, "\n\n") + "\n"
	path := filepath.Join("testdata", "issue72_surface_golden.txt")
	if *updateIssue72Surface {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatal("issue #72 API snapshot drifted; regenerate with -update-issue72-surface")
	}
}
