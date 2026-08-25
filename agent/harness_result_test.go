package agent_test

import (
	"errors"
	"testing"

	"github.com/nankedr/pig/agent"
)

func TestTaggedErrorFactoryKeepsTagPropertiesAndCause(t *testing.T) {
	cause := errors.New("disk full")
	factory := agent.TaggedError("WriteFailed")
	err := factory.New("write failed", map[string]any{"path": "notes.md", "cause": cause})

	if !factory.Is(err) {
		t.Fatal("factory does not recognize its own error")
	}
	if errors.Is(err, cause) == false {
		t.Fatal("tagged error does not unwrap its cause")
	}
	payload := err.ToJSON()
	if payload["_tag"] != "WriteFailed" || payload["message"] != "write failed" || payload["path"] != "notes.md" {
		t.Fatalf("ToJSON() = %#v", payload)
	}

	got, ok := agent.MatchError(err, agent.ErrorMatchers[string]{
		"WriteFailed": func(value *agent.TaggedErrorValue) string { return value.Name },
	})
	if !ok || got != "WriteFailed" {
		t.Fatalf("MatchError() = %q, %v", got, ok)
	}
}
