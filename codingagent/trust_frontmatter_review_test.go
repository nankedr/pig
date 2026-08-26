package codingagent_test

import (
	"errors"
	"testing"

	"github.com/nankedr/pig/codingagent"
)

func TestProjectTrustDecisionConstructors(t *testing.T) {
	var unset codingagent.ProjectTrustDecision
	if unset != nil {
		t.Fatalf("zero ProjectTrustDecision = %#v, want nil", unset)
	}

	trusted := codingagent.ProjectTrustDecisionTrusted()
	otherTrusted := codingagent.ProjectTrustDecisionTrusted()
	untrusted := codingagent.ProjectTrustDecisionUntrusted()
	otherUntrusted := codingagent.ProjectTrustDecisionUntrusted()
	if trusted == nil || otherTrusted == nil || untrusted == nil || otherUntrusted == nil {
		t.Fatal("explicit trust decision constructor returned nil")
	}
	if !*trusted || !*otherTrusted {
		t.Fatalf("trusted decisions = %v and %v, want true", *trusted, *otherTrusted)
	}
	if *untrusted || *otherUntrusted {
		t.Fatalf("untrusted decisions = %v and %v, want false", *untrusted, *otherUntrusted)
	}
	if trusted == otherTrusted || untrusted == otherUntrusted {
		t.Fatal("trust decision constructors reused mutable storage")
	}

	*trusted = false
	*untrusted = true
	if !*otherTrusted || *otherUntrusted {
		t.Fatalf("mutating one decision changed another: trusted=%v untrusted=%v", *otherTrusted, *otherUntrusted)
	}
	if fresh := codingagent.ProjectTrustDecisionTrusted(); fresh == nil || !*fresh {
		t.Fatalf("fresh trusted decision = %#v, want pointer to true", fresh)
	}
	if fresh := codingagent.ProjectTrustDecisionUntrusted(); fresh == nil || *fresh {
		t.Fatalf("fresh untrusted decision = %#v, want pointer to false", fresh)
	}
}

func TestParseFrontmatterIsExplicitCapabilityStub(t *testing.T) {
	tests := map[string]string{
		"flat YAML":      "---\nname: skill\n---\nBody",
		"malformed YAML": "---\nfoo: [bar\n---\nBody",
		"multiline YAML": "---\ndescription: |\n  Line one\n  Line two\n---\nBody",
		"no frontmatter": "Body only",
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			parsed, err := codingagent.ParseFrontmatter(input)
			assertReviewCapabilityError(t, err, "ParseFrontmatter")
			if parsed.Body != "" || parsed.Frontmatter != nil {
				t.Fatalf("ParseFrontmatter result = %#v, want zero value on unsupported capability", parsed)
			}
		})
	}
}

func TestStripFrontmatterPropagatesCapabilityError(t *testing.T) {
	body, err := codingagent.StripFrontmatter("---\nname: skill\n---\nBody")
	assertReviewCapabilityError(t, err, "ParseFrontmatter")
	if body != "" {
		t.Fatalf("StripFrontmatter body = %q, want empty on unsupported capability", body)
	}
}

func assertReviewCapabilityError(t *testing.T, err error, operation string) {
	t.Helper()
	if !errors.Is(err, codingagent.ErrNotImplemented) {
		t.Fatalf("error = %v, want ErrNotImplemented", err)
	}
	var target *codingagent.NotImplementedError
	if !errors.As(err, &target) {
		t.Fatalf("errors.As(%v, *NotImplementedError) = false", err)
	}
	if target.Module != "codingagent" || target.Operation != operation {
		t.Fatalf("NotImplementedError = %#v, want module codingagent operation %s", target, operation)
	}
}
