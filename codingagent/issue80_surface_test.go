package codingagent_test

import (
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nankedr/pig/codingagent"
)

var updateIssue80Surface = flag.Bool("update-issue80-surface", false, "regenerate issue #80 API snapshot")

func TestIssue80BashAPISnapshot(t *testing.T) {
	var sections []string
	for _, item := range []struct {
		name  string
		value any
	}{{"CreateBashTool", codingagent.CreateBashTool}, {"CreateBashToolDefinition", codingagent.CreateBashToolDefinition}, {"CreateLocalBashOperations", codingagent.CreateLocalBashOperations}, {"GetShellConfig", codingagent.GetShellConfig}} {
		sections = append(sections, "func codingagent."+item.name+" "+reflect.TypeOf(item.value).String())
	}
	for _, item := range []struct {
		name  string
		value any
	}{{"BashToolOptions", codingagent.BashToolOptions{}}, {"BashToolInput", codingagent.BashToolInput{}}, {"BashToolDetails", codingagent.BashToolDetails{}}, {"BashExecOptions", codingagent.BashExecOptions{}}, {"BashExecResult", codingagent.BashExecResult{}}, {"BashSpawnContext", codingagent.BashSpawnContext{}}, {"ShellConfig", codingagent.ShellConfig{}}} {
		sections = append(sections, issue71TypeSnapshot(item.name, reflect.TypeOf(item.value)))
	}
	got := strings.Join(sections, "\n\n") + "\n"
	path := filepath.Join("testdata", "issue80_surface_golden.txt")
	if *updateIssue80Surface {
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
		t.Fatal("bash API snapshot drift")
	}
}
