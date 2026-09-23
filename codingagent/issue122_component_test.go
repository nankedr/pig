package codingagent_test

import (
	"fmt"
	"github.com/nankedr/pig/codingagent"
	"strings"
	"testing"
)

func TestBashComponent122(t *testing.T) {
	c := codingagent.NewBashExecutionComponent("printf demo", true)
	for i := 0; i < 25; i++ {
		_ = c.AppendOutput(fmt.Sprintf("line-%02d\n", i))
	}
	code := 7
	path := "/tmp/full-output"
	_ = c.SetComplete(&code, false, &codingagent.TruncationResult{Truncated: true}, &path)
	code = 0
	path = "changed"
	_ = c.AppendOutput("late callback")
	text, _ := c.GetOutput()
	if strings.Contains(text, "late callback") {
		t.Fatal("late callback overwrote completed output")
	}
	lines, err := c.Render(80)
	rendered := strings.Join(lines, "\n")
	if err != nil || strings.Contains(rendered, "line-00") || !strings.Contains(rendered, "line-24") || !strings.Contains(rendered, "(exit 7)") || !strings.Contains(rendered, "Full output: /tmp/full-output") {
		t.Fatalf("collapsed: %s %v", rendered, err)
	}
	_ = c.SetExpanded(true)
	lines, err = c.Render(80)
	if err != nil || !strings.Contains(strings.Join(lines, "\n"), "line-00") || strings.Contains(strings.Join(lines, "\n"), "more lines") {
		t.Fatal("expand lost output", lines, err)
	}
}
