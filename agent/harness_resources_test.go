package agent_test

import (
	"strings"
	"testing"

	"github.com/nankedr/pig/agent"
)

func TestHarnessResourcePureHelpers(t *testing.T) {
	args := agent.ParseCommandArgs(`one "two words" 'three words'`)
	if got := agent.SubstituteArgs(`$1|${@:2:2}|$@`, args); got != "one|two words three words|one two words three words" {
		t.Fatalf("SubstituteArgs() = %q", got)
	}

	systemPrompt := agent.FormatSkillsForSystemPrompt([]agent.Skill{{
		Name:        `review<&"'>`,
		Description: "safe",
		FilePath:    "/tmp/SKILL.md",
	}})
	if want := `review&lt;&amp;&quot;&apos;&gt;`; !strings.Contains(systemPrompt, want) {
		t.Fatalf("FormatSkillsForSystemPrompt() = %q; missing %q", systemPrompt, want)
	}

	truncation := agent.TruncateTail("first\nsecond\nthird", agent.TruncationOptions{MaxLines: 2, MaxBytes: 100})
	if !truncation.Truncated || truncation.TruncatedBy != agent.TruncationByLines || truncation.Content != "second\nthird" {
		t.Fatalf("TruncateTail() = %#v", truncation)
	}
	if got := agent.SanitizeBinaryOutput("a\x00\tb\ufffac"); got != "a\tbc" {
		t.Fatalf("SanitizeBinaryOutput() = %q", got)
	}
}
