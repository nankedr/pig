package codingagent_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

var (
	_ func(*codingagent.AgentSession, ai.UserMessageContent, ...codingagent.SendUserMessageOptions) error                         = (*codingagent.AgentSession).SendUserMessage
	_ func(*codingagent.AgentSession, codingagent.ExtensionBindings) error                                                        = (*codingagent.AgentSession).BindExtensions
	_ func(*codingagent.AgentSession, context.Context, ...string) (codingagent.CompactionResult, error)                           = (*codingagent.AgentSession).Compact
	_ func(*codingagent.AgentSession, context.Context, ...codingagent.ModelCycleDirection) (*codingagent.ModelCycleResult, error) = (*codingagent.AgentSession).CycleModel
	_ func(*codingagent.AgentSession, context.Context, string, ...codingagent.ExecuteBashOptions) (codingagent.BashResult, error) = (*codingagent.AgentSession).ExecuteBash
	_ func(*codingagent.AgentSession, string, codingagent.BashResult, ...codingagent.RecordBashResultOptions) error               = (*codingagent.AgentSession).RecordBashResult
)

func TestAgentSessionExactOperationCarriers(t *testing.T) {
	t.Run("finite unions", func(t *testing.T) {
		if codingagent.UserMessageDeliverySteer != "steer" || codingagent.UserMessageDeliveryFollowUp != "followUp" {
			t.Fatalf("user-message delivery values = (%q, %q)", codingagent.UserMessageDeliverySteer, codingagent.UserMessageDeliveryFollowUp)
		}
		if codingagent.ModelCycleForward != "forward" || codingagent.ModelCycleBackward != "backward" {
			t.Fatalf("model-cycle direction values = (%q, %q)", codingagent.ModelCycleForward, codingagent.ModelCycleBackward)
		}
	})

	t.Run("extension callbacks stay opaque", func(t *testing.T) {
		opaque := reflect.TypeOf((*codingagent.ExtensionHandler)(nil)).Elem()
		want := map[string]reflect.Type{
			"UIContext":             reflect.TypeOf((*codingagent.ExtensionUIContext)(nil)),
			"Mode":                  reflect.TypeOf((*codingagent.ExtensionMode)(nil)),
			"CommandContextActions": reflect.TypeOf((*codingagent.ExtensionCommandContextActions)(nil)),
			"AbortHandler":          opaque,
			"ShutdownHandler":       opaque,
			"OnError":               opaque,
		}
		typeOf := reflect.TypeOf(codingagent.ExtensionBindings{})
		if typeOf.NumField() != len(want) {
			t.Fatalf("ExtensionBindings has %d fields, want %d", typeOf.NumField(), len(want))
		}
		for name, wantType := range want {
			field, ok := typeOf.FieldByName(name)
			if !ok || field.Type != wantType {
				t.Errorf("ExtensionBindings.%s = %v, want %v", name, field.Type, wantType)
			}
		}
	})

	t.Run("optional operation arguments retain absence", func(t *testing.T) {
		send := codingagent.SendUserMessageOptions{}
		if send.DeliverAs != "" {
			t.Fatalf("zero SendUserMessageOptions.DeliverAs = %q, want absent zero value", send.DeliverAs)
		}

		execute := codingagent.ExecuteBashOptions{}
		if execute.OnChunk != nil || execute.ExcludeFromContext || execute.ID != nil || execute.Operations != nil {
			t.Fatalf("zero ExecuteBashOptions is not absent: %#v", execute)
		}

		record := codingagent.RecordBashResultOptions{}
		if record.ExcludeFromContext {
			t.Fatalf("zero RecordBashResultOptions is not absent: %#v", record)
		}
	})

	t.Run("bash options preserve optional inputs", func(t *testing.T) {
		id := "request-1"
		options := codingagent.ExecuteBashOptions{
			OnChunk:            func(string) {},
			ExcludeFromContext: true,
			ID:                 &id,
			Operations:         panicSessionBashOperations{},
		}
		if options.OnChunk == nil || !options.ExcludeFromContext || options.ID == nil || *options.ID != id || options.Operations == nil {
			t.Fatalf("ExecuteBashOptions lost a pinned field: %#v", options)
		}
	})
}

func TestAgentSessionExactOperationStubsAreInert(t *testing.T) {
	legacy := newLegacyAgentWithQueuedSteering(t, agent.AgentInitialState{
		SystemPrompt:  "system",
		Model:         ai.Model{ID: "model-1", Name: "Model One", API: ai.API("api"), Provider: ai.ProviderID("provider")},
		ThinkingLevel: ai.ModelThinkingLevelHigh,
		Messages:      []agent.AgentMessage{ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("existing"), Timestamp: 1}},
	})
	manager := codingagent.NewInMemorySessionManager("/project", codingagent.NewSessionOptions{ID: "session-1"})
	runner := &codingagent.ExtensionRunner{}
	session := codingagent.NewAgentSession(codingagent.AgentSessionConfig{
		Agent:                  legacy,
		SessionManager:         manager,
		ExtensionRunnerRef:     runner,
		InitialActiveToolNames: []string{"read", "bash"},
		ScopedModels:           []codingagent.ScopedModel{{Model: legacy.State().Model, ThinkingLevel: ai.ModelThinkingLevelHigh}},
	})

	beforeAgent := legacy.State()
	beforeHeader := manager.GetHeader()
	beforeEntries := manager.GetEntries()
	beforeTools := session.GetActiveToolNames()
	beforeScoped := session.ScopedModels()
	chunkCalls := 0
	listenerCalls := 0
	unsubscribe := legacy.Subscribe(func(context.Context, agent.AgentEvent) error {
		listenerCalls++
		return nil
	})
	defer unsubscribe()

	tests := []struct {
		name      string
		operation string
		call      func() error
	}{
		{name: "send user message", operation: "AgentSession.SendUserMessage", call: func() error {
			return session.SendUserMessage(ai.UserBlocks(
				ai.TextContent{Type: ai.ContentTypeText, Text: "hello"},
				ai.ImageContent{Type: ai.ContentTypeImage, Data: "aGk=", MIMEType: "image/png"},
			), codingagent.SendUserMessageOptions{DeliverAs: codingagent.UserMessageDeliveryFollowUp})
		}},
		{name: "bind extensions", operation: "AgentSession.BindExtensions", call: func() error {
			mode := codingagent.ExtensionModeRPC
			return session.BindExtensions(codingagent.ExtensionBindings{
				UIContext:             &codingagent.ExtensionUIContext{},
				Mode:                  &mode,
				CommandContextActions: &codingagent.ExtensionCommandContextActions{},
			})
		}},
		{name: "compact", operation: "AgentSession.Compact", call: func() error {
			result, err := session.Compact(context.Background(), "preserve decisions")
			if !reflect.DeepEqual(result, codingagent.CompactionResult{}) {
				t.Errorf("Compact result = %#v, want zero CompactionResult", result)
			}
			return err
		}},
		{name: "cycle model", operation: "AgentSession.CycleModel", call: func() error {
			result, err := session.CycleModel(context.Background(), codingagent.ModelCycleBackward)
			if result != nil {
				t.Errorf("CycleModel result = %#v, want nil", result)
			}
			return err
		}},
		{name: "execute bash", operation: "AgentSession.ExecuteBash", call: func() error {
			result, err := session.ExecuteBash(context.Background(), "must not run", codingagent.ExecuteBashOptions{
				OnChunk:    func(string) { chunkCalls++ },
				ID:         pointerTo("bash-1"),
				Operations: panicSessionBashOperations{},
			})
			if result != (codingagent.BashResult{}) {
				t.Errorf("ExecuteBash result = %#v, want zero BashResult", result)
			}
			return err
		}},
		{name: "record bash result", operation: "AgentSession.RecordBashResult", call: func() error {
			exitCode := 9
			fullOutputPath := "/must/not/be/retained"
			return session.RecordBashResult("must not persist", codingagent.BashResult{
				Output:         "output",
				ExitCode:       &exitCode,
				Cancelled:      true,
				Truncated:      true,
				FullOutputPath: &fullOutputPath,
			}, codingagent.RecordBashResultOptions{ExcludeFromContext: true})
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assertSessionOperationNotImplemented(t, test.call(), test.operation)
			if got := legacy.State(); !reflect.DeepEqual(got, beforeAgent) {
				t.Fatalf("%s changed Agent state: got %#v, want %#v", test.operation, got, beforeAgent)
			}
			if got := manager.GetHeader(); !reflect.DeepEqual(got, beforeHeader) {
				t.Fatalf("%s changed session header: got %#v, want %#v", test.operation, got, beforeHeader)
			}
			if got := manager.GetEntries(); !reflect.DeepEqual(got, beforeEntries) {
				t.Fatalf("%s changed session entries: got %#v, want %#v", test.operation, got, beforeEntries)
			}
			if got := session.GetActiveToolNames(); !reflect.DeepEqual(got, beforeTools) {
				t.Fatalf("%s changed active tools: got %v, want %v", test.operation, got, beforeTools)
			}
			if got := session.ScopedModels(); !reflect.DeepEqual(got, beforeScoped) {
				t.Fatalf("%s changed scoped models: got %#v, want %#v", test.operation, got, beforeScoped)
			}
			if session.ExtensionRunner() != runner {
				t.Fatalf("%s changed extension runner", test.operation)
			}
			if !legacy.HasQueuedMessages() {
				t.Fatalf("%s drained the Agent queue", test.operation)
			}
		})
	}

	if chunkCalls != 0 {
		t.Fatalf("ExecuteBash invoked OnChunk %d times, want zero", chunkCalls)
	}
	if listenerCalls != 0 {
		t.Fatalf("AgentSession stubs published %d Agent events, want zero", listenerCalls)
	}
}

type panicSessionBashOperations struct{}

func (panicSessionBashOperations) Exec(context.Context, string, string, codingagent.BashExecOptions) (codingagent.BashExecResult, error) {
	panic("AgentSession.ExecuteBash invoked BashOperations.Exec")
}

func assertSessionOperationNotImplemented(t *testing.T, err error, operation string) {
	t.Helper()
	if !errors.Is(err, codingagent.ErrNotImplemented) {
		t.Fatalf("%s error = %v, want ErrNotImplemented", operation, err)
	}
	var unavailable *codingagent.NotImplementedError
	if !errors.As(err, &unavailable) {
		t.Fatalf("%s error = %T, want *NotImplementedError", operation, err)
	}
	if unavailable.Module != "codingagent" || unavailable.Operation != operation {
		t.Fatalf("%s error = %#v, want module codingagent and exact operation", operation, unavailable)
	}
}

func pointerTo[T any](value T) *T { return &value }
