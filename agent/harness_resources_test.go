package agent_test

import (
	"reflect"
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

func TestTruncateHeadClampsNegativeLimits(t *testing.T) {
	const content = "alpha\nbeta"
	tests := []struct {
		name    string
		options agent.TruncationOptions
		want    agent.TruncationResult
	}{
		{
			name:    "lines",
			options: agent.TruncationOptions{MaxLines: -1, MaxBytes: 100},
			want: agent.TruncationResult{
				Truncated:   true,
				TruncatedBy: agent.TruncationByLines,
				TotalLines:  2,
				TotalBytes:  10,
				MaxLines:    0,
				MaxBytes:    100,
			},
		},
		{
			name:    "bytes",
			options: agent.TruncationOptions{MaxLines: 100, MaxBytes: -1},
			want: agent.TruncationResult{
				Truncated:             true,
				TruncatedBy:           agent.TruncationByBytes,
				TotalLines:            2,
				TotalBytes:            10,
				FirstLineExceedsLimit: true,
				MaxLines:              100,
				MaxBytes:              0,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := agent.TruncateHead(content, test.options); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("TruncateHead() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestTruncateTailClampsNegativeLimits(t *testing.T) {
	const content = "alpha\nbeta"
	tests := []struct {
		name    string
		options agent.TruncationOptions
		want    agent.TruncationResult
	}{
		{
			name:    "lines",
			options: agent.TruncationOptions{MaxLines: -1, MaxBytes: 100},
			want: agent.TruncationResult{
				Truncated:   true,
				TruncatedBy: agent.TruncationByLines,
				TotalLines:  2,
				TotalBytes:  10,
				MaxLines:    0,
				MaxBytes:    100,
			},
		},
		{
			name:    "bytes",
			options: agent.TruncationOptions{MaxLines: 100, MaxBytes: -1},
			want: agent.TruncationResult{
				Truncated:       true,
				TruncatedBy:     agent.TruncationByBytes,
				TotalLines:      2,
				TotalBytes:      10,
				OutputLines:     1,
				LastLinePartial: true,
				MaxLines:        100,
				MaxBytes:        0,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := agent.TruncateTail(content, test.options); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("TruncateTail() = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestTruncateLineClampsNegativeLimit(t *testing.T) {
	want := agent.LineTruncation{Text: "... [truncated]", WasTruncated: true}
	if got := agent.TruncateLine("alpha", -1); !reflect.DeepEqual(got, want) {
		t.Fatalf("TruncateLine() = %#v, want %#v", got, want)
	}
}
