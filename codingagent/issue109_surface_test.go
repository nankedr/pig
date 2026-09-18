package codingagent_test

import (
	"flag"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/tui"
)

var updateIssue109Surface = flag.Bool("update-issue109-surface", false, "regenerate Interactive API snapshot")

func TestIssue109InteractiveAPISnapshot(t *testing.T) {
	var out strings.Builder
	for _, value := range []any{codingagent.InteractiveModeOptions{}, tui.TextUIOptions{}, tui.TextInput{}} {
		typ := reflect.TypeOf(value)
		fmt.Fprintln(&out, "type", typ)
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			fmt.Fprintln(&out, field.Name, field.Type)
		}
	}

	for _, value := range []any{(*codingagent.InteractiveMode)(nil), (*tui.ProcessTerminal)(nil), (*tui.TextUI)(nil)} {
		typ := reflect.TypeOf(value)
		fmt.Fprintln(&out, "type", typ)
		for i := 0; i < typ.NumMethod(); i++ {
			method := typ.Method(i)
			fmt.Fprintln(&out, method.Name, method.Type)
		}
	}
	path := "testdata/issue109_surface_golden.txt"
	if *updateIssue109Surface {
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
		t.Fatal("Interactive API drift")
	}
}
