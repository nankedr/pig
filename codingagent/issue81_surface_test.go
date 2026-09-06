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

var updateIssue81Surface = flag.Bool("update-issue81-surface", false, "regenerate issue #81 API snapshot")

func TestIssue81CodingToolsAPISnapshot(t *testing.T) {
	var sections []string
	for _, item := range []struct {
		name  string
		value any
	}{{"CreateCodingTools", codingagent.CreateCodingTools}, {"CreateAgentSession", codingagent.CreateAgentSession}, {"CreateHeadlessSession", codingagent.CreateHeadlessSession}, {"CreateAgentSessionFromServices", codingagent.CreateAgentSessionFromServices}} {
		sections = append(sections, "func codingagent."+item.name+" "+reflect.TypeOf(item.value).String())
	}
	for _, item := range []struct {
		name  string
		value any
	}{{"ToolsOptions", codingagent.ToolsOptions{}}, {"CreateAgentSessionOptions", codingagent.CreateAgentSessionOptions{}}, {"CreateHeadlessSessionOptions", codingagent.CreateHeadlessSessionOptions{}}} {
		sections = append(sections, issue71TypeSnapshot(item.name, reflect.TypeOf(item.value)))
	}
	got := strings.Join(sections, "\n\n") + "\n"
	path := filepath.Join("testdata", "issue81_surface_golden.txt")
	if *updateIssue81Surface {
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
		t.Fatal("coding tools API snapshot drift")
	}
}
