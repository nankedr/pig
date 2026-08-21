package ai_test

import (
	"errors"
	"testing"

	"github.com/nankedr/pig/ai"
)

var (
	_ ai.PiMessagesEvent       = ai.PiMessagesStartEvent{}         // upstream: PiMessagesEvent
	_ ai.PiMessagesEvent       = ai.PiMessagesTextStartEvent{}     // upstream: PiMessagesEvent
	_ ai.PiMessagesEvent       = ai.PiMessagesTextDeltaEvent{}     // upstream: PiMessagesEvent
	_ ai.PiMessagesEvent       = ai.PiMessagesTextEndEvent{}       // upstream: PiMessagesEvent
	_ ai.PiMessagesEvent       = ai.PiMessagesThinkingStartEvent{} // upstream: PiMessagesEvent
	_ ai.PiMessagesEvent       = ai.PiMessagesThinkingDeltaEvent{} // upstream: PiMessagesEvent
	_ ai.PiMessagesEvent       = ai.PiMessagesThinkingEndEvent{}   // upstream: PiMessagesEvent
	_ ai.PiMessagesEvent       = ai.PiMessagesToolCallStartEvent{} // upstream: PiMessagesEvent
	_ ai.PiMessagesEvent       = ai.PiMessagesToolCallDeltaEvent{} // upstream: PiMessagesEvent
	_ ai.PiMessagesEvent       = ai.PiMessagesToolCallEndEvent{}   // upstream: PiMessagesEvent
	_ ai.PiMessagesEvent       = ai.PiMessagesDoneEvent{}          // upstream: PiMessagesEvent
	_ ai.PiMessagesEvent       = ai.PiMessagesErrorEvent{}         // upstream: PiMessagesEvent
	_ ai.PiMessagesDoneReason  = ai.PiMessagesDoneReasonStop
	_ ai.PiMessagesErrorReason = ai.PiMessagesErrorReasonError

	_ = ai.PiMessagesRewriteImpact{} // upstream: PiMessagesRewriteImpact

	_ = ai.OpenAICodexWebSocketDebugStats{} // upstream: OpenAICodexWebSocketDebugStats

	_ ai.FauxContentBlock    = ai.TextContent{}                 // upstream: FauxContentBlock
	_                        = ai.FauxModelDefinition{}         // upstream: FauxModelDefinition
	_                        = ai.FauxProviderHandle{}          // upstream: FauxProviderHandle
	_                        = ai.FauxProviderRegistration{}    // upstream: FauxProviderRegistration
	_                        = ai.FauxProviderState{}           // upstream: FauxProviderState
	_ ai.FauxResponseFactory = nil                              // upstream: FauxResponseFactory
	_ ai.FauxResponseStep    = ai.AssistantMessage{}            // upstream: FauxResponseStep
	_ ai.FauxResponseStep    = ai.FauxResponseFactory(nil)      // upstream: FauxResponseStep
	_                        = ai.RegisterFauxProviderOptions{} // upstream: RegisterFauxProviderOptions
	_                        = ai.CreateFauxCore                // upstream: createFauxCore
	_                        = ai.FauxAssistantMessage          // upstream: fauxAssistantMessage
	_                        = ai.NewFauxProvider               // upstream: fauxProvider
	_                        = ai.FauxText                      // upstream: fauxText
	_                        = ai.FauxThinking                  // upstream: fauxThinking
	_                        = ai.FauxToolCall                  // upstream: fauxToolCall
)

func TestFauxPureValuesAndRuntimeStubsStaySeparated(t *testing.T) {
	t.Parallel()

	if text := ai.FauxText("hello"); text.Type != ai.ContentTypeText || text.Text != "hello" {
		t.Fatalf("FauxText = %#v", text)
	}
	if thinking := ai.FauxThinking("hmm"); thinking.Type != ai.ContentTypeThinking || thinking.Thinking != "hmm" {
		t.Fatalf("FauxThinking = %#v", thinking)
	}
	if _, err := ai.FauxToolCall("tool", nil); !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("FauxToolCall error = %v", err)
	}
	if _, err := ai.FauxAssistantMessage(ai.FauxAssistantText("response")); !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("FauxAssistantMessage error = %v", err)
	}
	if core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{}); core != nil || !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("CreateFauxCore = (%#v, %v)", core, err)
	}
	if provider, err := ai.NewFauxProvider(); provider != nil || !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("NewFauxProvider = (%#v, %v)", provider, err)
	}
}
