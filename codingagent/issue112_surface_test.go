package codingagent_test

import (
	"flag"
	"fmt"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/tui"
	"os"
	"reflect"
	"strings"
	"testing"
)

var updateIssue112Surface = flag.Bool("update-issue112-surface", false, "refresh text rendering API snapshot")

func TestTextRenderingAPISnapshot112(t *testing.T) {
	var b strings.Builder
	for _, v := range []any{(*codingagent.Transcript)(nil), (*codingagent.AssistantMessageComponent)(nil), (*codingagent.UserMessageComponent)(nil), (*codingagent.ToolExecutionComponent)(nil), (*tui.Markdown)(nil)} {
		typ := reflect.TypeOf(v)
		fmt.Fprintln(&b, typ)
		for i := 0; i < typ.NumMethod(); i++ {
			m := typ.Method(i)
			fmt.Fprintln(&b, m.Name, m.Type)
		}
	}
	for _, v := range []any{codingagent.NewTranscript, codingagent.NewAssistantMessageComponent, codingagent.NewUserMessageComponent, codingagent.NewToolExecutionComponent, tui.SafeTerminalText, codingagent.RenderDiff} {
		fmt.Fprintln(&b, reflect.TypeOf(v))
	}
	path := "testdata/issue112_surface_golden.txt"
	if *updateIssue112Surface {
		if err := os.WriteFile(path, []byte(b.String()), 0644); err != nil {
			t.Fatal(err)
		}
		return
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != b.String() {
		t.Fatal("text rendering API drift")
	}
}
