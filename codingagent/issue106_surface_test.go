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

var updateIssue106Surface = flag.Bool("update-issue106-surface", false, "regenerate issue #106 API snapshot")

func TestIssue106ReloadAPISnapshot(t *testing.T) {
	var out strings.Builder
	for _, value := range []any{(*codingagent.AgentSession)(nil), (*codingagent.DefaultResourceLoader)(nil), (*codingagent.SettingsManager)(nil)} {
		typ := reflect.TypeOf(value)
		for _, name := range []string{"Reload", "Prompt", "PromptTemplates", "ResourceLoader", "SettingsManager", "GetSkills", "GetThemes", "GetSystemPrompt", "SetProjectTrusted"} {
			if m, ok := typ.MethodByName(name); ok {
				fmt.Fprintf(&out, "method %s.%s %s\n", typ, name, m.Type)
			}
		}
	}
	typ := reflect.TypeOf(codingagent.ResourceLoaderReloadOptions{})
	fmt.Fprintf(&out, "type %s\n", typ)
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		fmt.Fprintf(&out, "- %s %s\n", f.Name, f.Type)
	}
	path := "testdata/issue106_surface_golden.txt"
	if *updateIssue106Surface {
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
		t.Fatal("reload API drift; use -update-issue106-surface")
	}
}
