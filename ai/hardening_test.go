package ai_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/nankedr/pig/ai"
)

func TestClosedUnionCodecsRejectTypedNilPointers(t *testing.T) {
	t.Parallel()

	var content *ai.TextContent
	if _, err := ai.MarshalContent(content); !errors.Is(err, ai.ErrCodec) {
		t.Fatalf("MarshalContent(typed nil) error = %v, want ErrCodec", err)
	}

	var message *ai.AssistantMessage
	if _, err := ai.MarshalMessage(message); !errors.Is(err, ai.ErrCodec) {
		t.Fatalf("MarshalMessage(typed nil) error = %v, want ErrCodec", err)
	}

	var event *ai.AssistantMessageDoneEvent
	if _, err := ai.MarshalAssistantMessageEvent(event); !errors.Is(err, ai.ErrCodec) {
		t.Fatalf("MarshalAssistantMessageEvent(typed nil) error = %v, want ErrCodec", err)
	}

	stream := ai.NewAssistantMessageEventStream()
	stream.Push(event)
	if _, err := stream.Result(context.Background()); !errors.Is(err, ai.ErrEventStreamInvariant) {
		t.Fatalf("Result() after typed nil event error = %v, want ErrEventStreamInvariant", err)
	}
}

func TestEventStreamCallbacksCanReenterWithoutMutexDeadlock(t *testing.T) {
	t.Parallel()

	var stream *ai.EventStream[int, int]
	stream = ai.NewEventStream(
		func(int) bool {
			stream.End(7)
			return false
		},
		func(value int) int { return value },
	)
	returned := make(chan struct{})
	go func() {
		stream.Push(1)
		close(returned)
	}()

	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("Push deadlocked when isComplete re-entered End")
	}
	result, err := stream.Result(context.Background())
	if err != nil || result != 7 {
		t.Fatalf("Result() = (%d, %v), want (7, nil)", result, err)
	}
}

func TestEventStreamNilCallbacksBecomeInvariantFailure(t *testing.T) {
	t.Parallel()

	stream := ai.NewEventStream[int, int](nil, nil)
	stream.Push(1)
	if _, err := stream.Result(context.Background()); !errors.Is(err, ai.ErrEventStreamInvariant) {
		t.Fatalf("Result() error = %v, want ErrEventStreamInvariant", err)
	}
}

func TestAssistantMessageEventStreamRejectsForgedDiscriminators(t *testing.T) {
	t.Parallel()

	stream := ai.NewAssistantMessageEventStream()
	stream.Push(ai.AssistantMessageTextDeltaEvent{
		Type:    ai.AssistantMessageEventTypeDone,
		Delta:   "not terminal",
		Partial: ai.AssistantMessage{Role: ai.MessageRoleAssistant},
	})
	if _, err := stream.Result(context.Background()); !errors.Is(err, ai.ErrEventStreamInvariant) || !errors.Is(err, ai.ErrCodec) {
		t.Fatalf("Result() error = %v, want EventStreamInvariant wrapping ErrCodec", err)
	}
}

func TestAssistantMessageSnapshotsDeepCopyDiagnosticCodeAndNamedContainers(t *testing.T) {
	t.Parallel()

	type tags []string
	type codeMap map[string]any
	code := codeMap{"tags": tags{"before"}}
	message := ai.AssistantMessage{
		Diagnostics: ai.Some([]ai.AssistantMessageDiagnostic{{
			Error: ai.Some(ai.DiagnosticErrorInfo{Message: "failed", Code: ai.Some[ai.JSONValue](code)}),
		}}),
	}

	clone := ai.CloneAssistantMessage(message)
	code["tags"].(tags)[0] = "after"
	diagnostics, _ := clone.Diagnostics.Value()
	errorInfo, _ := diagnostics[0].Error.Value()
	clonedCode, _ := errorInfo.Code.Value()
	if got := clonedCode.(codeMap)["tags"].(tags)[0]; got != "before" {
		t.Fatalf("cloned diagnostic code tag = %q, want before", got)
	}
}

func TestUserMessageContentDefensivelyCopiesPointerBlocks(t *testing.T) {
	t.Parallel()

	block := &ai.TextContent{Type: ai.ContentTypeText, Text: "before"}
	content := ai.UserBlocks(block)
	block.Text = "source mutation"
	blocks, ok := content.Blocks()
	if !ok || blocks[0].(*ai.TextContent).Text != "before" {
		t.Fatalf("Blocks() = %#v, want isolated source snapshot", blocks)
	}
	blocks[0].(*ai.TextContent).Text = "return mutation"
	again, _ := content.Blocks()
	if got := again[0].(*ai.TextContent).Text; got != "before" {
		t.Fatalf("second Blocks() text = %q, want before", got)
	}
}

func TestClosedUnionDecodersRejectMissingRequiredFields(t *testing.T) {
	t.Parallel()

	contentInputs := []string{
		`{"type":"image"}`,
		`{"type":"image","data":null,"mimeType":null}`,
		`{"type":"toolCall"}`,
	}
	for _, input := range contentInputs {
		if _, err := ai.UnmarshalContent([]byte(input)); !errors.Is(err, ai.ErrCodec) {
			t.Errorf("UnmarshalContent(%s) error = %v, want ErrCodec", input, err)
		}
	}

	messageInputs := []string{
		`{"role":"user"}`,
		`{"role":"assistant"}`,
		`{"role":"toolResult"}`,
	}
	for _, input := range messageInputs {
		if _, err := ai.UnmarshalMessage([]byte(input)); !errors.Is(err, ai.ErrCodec) {
			t.Errorf("UnmarshalMessage(%s) error = %v, want ErrCodec", input, err)
		}
	}
}

func TestEventDecoderRejectsMissingVariantPayload(t *testing.T) {
	t.Parallel()

	partial, err := ai.MarshalMessage(ai.AssistantMessage{
		Role:       ai.MessageRoleAssistant,
		Content:    []ai.AssistantContent{},
		StopReason: ai.StopReasonPending,
	})
	if err != nil {
		t.Fatalf("MarshalMessage(partial) error = %v", err)
	}
	input := append([]byte(`{"type":"text_delta","partial":`), partial...)
	input = append(input, '}')
	if _, err := ai.UnmarshalAssistantMessageEvent(input); !errors.Is(err, ai.ErrCodec) {
		t.Fatalf("UnmarshalAssistantMessageEvent() error = %v, want ErrCodec", err)
	}
}

func TestToolAcceptsPointerSamplingVariantsAndRejectsInvalidSchemaRoots(t *testing.T) {
	t.Parallel()

	tool := ai.Tool{
		Name:        "search",
		Description: "Search",
		Parameters:  json.RawMessage(`{"type":"object"}`),
		ConstrainedSampling: &ai.JSONSchemaConstrainedSampling{
			Strict: ai.ConstrainedSamplingStrictRequire,
		},
	}
	if _, err := json.Marshal(tool); err != nil {
		t.Fatalf("json.Marshal(pointer constrained sampling) error = %v", err)
	}

	for _, input := range []string{
		`{"name":"x","description":"x","parameters":null}`,
		`{"name":"x","description":"x","parameters":"not a schema"}`,
		`{"description":"x","parameters":{}}`,
	} {
		var decoded ai.Tool
		if err := json.Unmarshal([]byte(input), &decoded); !errors.Is(err, ai.ErrCodec) {
			t.Errorf("json.Unmarshal(%s) error = %v, want ErrCodec", input, err)
		}
	}
}
