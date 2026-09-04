package ai_test

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/nankedr/pig/ai"
)

func TestIssue67LockedGoAPISnapshot(t *testing.T) {
	sections := []string{issue60TypeSnapshot("ContextUsageEstimate", reflect.TypeOf(ai.ContextUsageEstimate{}))}
	for _, function := range []struct {
		name  string
		value any
	}{
		{"EstimateContextTokens", ai.EstimateContextTokens},
		{"GetOverflowPatterns", ai.GetOverflowPatterns},
		{"IsContextOverflow", ai.IsContextOverflow},
		{"IsRecoverableLength", ai.IsRecoverableLength},
	} {
		sections = append(sections, "func ai."+function.name+" "+reflect.TypeOf(function.value).String())
	}
	want, err := os.ReadFile("testdata/issue67_surface_golden.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(sections, "\n\n") + "\n"; got != string(want) {
		t.Fatalf("issue #67 API changed:\n%s", got)
	}
}
