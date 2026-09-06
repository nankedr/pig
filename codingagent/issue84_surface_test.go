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

var updateIssue84Surface = flag.Bool("update-issue84-surface", false, "regenerate issue #84 API snapshot")

func TestIssue84FindLsAPISnapshot(t *testing.T) {
	var sections []string
	for _, item := range []struct {
		name  string
		value any
	}{{"CreateFindTool", codingagent.CreateFindTool}, {"CreateFindToolDefinition", codingagent.CreateFindToolDefinition}, {"CreateLsTool", codingagent.CreateLsTool}, {"CreateLsToolDefinition", codingagent.CreateLsToolDefinition}} {
		sections = append(sections, "func codingagent."+item.name+" "+reflect.TypeOf(item.value).String())
	}
	for _, item := range []struct {
		name  string
		value any
	}{{"FindToolOptions", codingagent.FindToolOptions{}}, {"FindToolInput", codingagent.FindToolInput{}}, {"FindToolDetails", codingagent.FindToolDetails{}}, {"FindGlobOptions", codingagent.FindGlobOptions{}}, {"FindOperations", (*codingagent.FindOperations)(nil)}, {"LsToolOptions", codingagent.LsToolOptions{}}, {"LsToolInput", codingagent.LsToolInput{}}, {"LsToolDetails", codingagent.LsToolDetails{}}, {"LsOperations", (*codingagent.LsOperations)(nil)}, {"LsFileInfo", (*codingagent.LsFileInfo)(nil)}} {
		typ := reflect.TypeOf(item.value)
		if typ.Kind() == reflect.Pointer {
			typ = typ.Elem()
		}
		if typ.Kind() == reflect.Interface {
			section := "type codingagent." + item.name + " interface"
			for i := 0; i < typ.NumMethod(); i++ {
				method := typ.Method(i)
				section += "\n  " + method.Name + " " + method.Type.String()
			}
			sections = append(sections, section)
		} else {
			sections = append(sections, issue71TypeSnapshot(item.name, typ))
		}
	}
	got := strings.Join(sections, "\n\n") + "\n"
	path := filepath.Join("testdata", "issue84_surface_golden.txt")
	if *updateIssue84Surface {
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
		t.Fatal("find/ls API snapshot drift")
	}
}
