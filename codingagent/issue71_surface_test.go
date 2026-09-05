package codingagent_test

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nankedr/pig/codingagent"
)

var updateIssue71Surface = flag.Bool("update-issue71-surface", false, "regenerate issue #71 API snapshot")

func TestIssue71LockedGoAPISnapshot(t *testing.T) {
	sections := []string{
		issue71TypeSnapshot("CreateHeadlessSessionOptions", reflect.TypeOf(codingagent.CreateHeadlessSessionOptions{})),
		issue71TypeSnapshot("NewSessionOptions", reflect.TypeOf(codingagent.NewSessionOptions{})),
		"func codingagent.GetAgentDir " + reflect.TypeOf(codingagent.GetAgentDir).String(),
		"func codingagent.NewSessionManager " + reflect.TypeOf(codingagent.NewSessionManager).String(),
		"func codingagent.OpenSessionManager " + reflect.TypeOf(codingagent.OpenSessionManager).String(),
	}
	manager := reflect.TypeOf((*codingagent.SessionManager)(nil))
	for _, name := range []string{"AppendMessage", "AppendModelChange", "AppendThinkingLevelChange", "BuildSessionContext", "GetHeader", "GetSessionFile", "IsPersisted", "UsesDefaultSessionDir"} {
		method, ok := manager.MethodByName(name)
		if !ok {
			t.Fatalf("SessionManager.%s is missing", name)
		}
		sections = append(sections, "method codingagent.SessionManager."+name+" "+method.Type.String())
	}
	got := strings.Join(sections, "\n\n") + "\n"
	path := filepath.Join("testdata", "issue71_surface_golden.txt")
	if *updateIssue71Surface {
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
		t.Fatal("issue #71 API snapshot drifted; regenerate with -update-issue71-surface")
	}
}

func issue71TypeSnapshot(name string, typeOf reflect.Type) string {
	var out strings.Builder
	fmt.Fprintf(&out, "type codingagent.%s struct\nfields:\n", name)
	for i := 0; i < typeOf.NumField(); i++ {
		field := typeOf.Field(i)
		fmt.Fprintf(&out, "- %s %s tag=%q\n", field.Name, field.Type, field.Tag)
	}
	return strings.TrimSuffix(out.String(), "\n")
}
