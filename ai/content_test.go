package ai_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/nankedr/pig/ai"
)

func TestContentCodecRoundTripsEveryVariant(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		content  ai.Content
		wantType ai.ContentType
	}{
		{name: "text", content: ai.TextContent{Type: ai.ContentTypeText, Text: "hello", TextSignature: ai.Some("sig")}, wantType: ai.ContentTypeText},
		{name: "thinking", content: ai.ThinkingContent{Type: ai.ContentTypeThinking, Thinking: "hmm", ThinkingSignature: ai.Null[string](), Redacted: ai.Some(false)}, wantType: ai.ContentTypeThinking},
		{name: "image", content: ai.ImageContent{Type: ai.ContentTypeImage, Data: "aGk=", MIMEType: "image/png"}, wantType: ai.ContentTypeImage},
		{name: "tool call", content: ai.ToolCall{Type: ai.ContentTypeToolCall, ID: "call-1", Name: "read", Arguments: map[string]any{"path": "README.md"}, Namespace: ai.Some("builtin")}, wantType: ai.ContentTypeToolCall},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			encoded, err := ai.MarshalContent(test.content)
			if err != nil {
				t.Fatalf("MarshalContent() error = %v", err)
			}
			decoded, err := ai.UnmarshalContent(encoded)
			if err != nil {
				t.Fatalf("UnmarshalContent() error = %v", err)
			}
			if decoded.ContentType() != test.wantType {
				t.Fatalf("ContentType() = %q, want %q", decoded.ContentType(), test.wantType)
			}
			if !reflect.DeepEqual(decoded, test.content) {
				t.Fatalf("round trip = %#v, want %#v", decoded, test.content)
			}
		})
	}
}

func TestContentCodecRejectsUnknownOrMismatchedDiscriminator(t *testing.T) {
	t.Parallel()

	if _, err := ai.MarshalContent(ai.TextContent{Type: ai.ContentTypeImage, Text: "wrong"}); !errors.Is(err, ai.ErrCodec) {
		t.Fatalf("MarshalContent() error = %v, want ErrCodec", err)
	}

	_, err := ai.UnmarshalContent([]byte(`{"type":"audio","data":"..."}`))
	if !errors.Is(err, ai.ErrCodec) {
		t.Fatalf("UnmarshalContent() error = %v, want ErrCodec", err)
	}
	var codecErr *ai.CodecError
	if !errors.As(err, &codecErr) {
		t.Fatalf("errors.As(%v, *CodecError) = false", err)
	}
	if codecErr.Surface != "content" || codecErr.Discriminator != "audio" {
		t.Fatalf("CodecError = %#v", codecErr)
	}
}

func TestContentSetsAreClosedByMessageRole(t *testing.T) {
	t.Parallel()

	var _ ai.UserContent = ai.TextContent{}
	var _ ai.UserContent = ai.ImageContent{}
	var _ ai.AssistantContent = ai.TextContent{}
	var _ ai.AssistantContent = ai.ThinkingContent{}
	var _ ai.AssistantContent = ai.ToolCall{}
	var _ ai.ToolResultContent = ai.TextContent{}
	var _ ai.ToolResultContent = ai.ImageContent{}
}
