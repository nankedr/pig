package protocol_test

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/nankedr/pig/protocol"
)

func requireNotImplemented(t *testing.T, err error, operation string) {
	t.Helper()

	if !errors.Is(err, protocol.ErrNotImplemented) {
		t.Fatalf("errors.Is(%v, protocol.ErrNotImplemented) = false", err)
	}
	var stub *protocol.NotImplementedError
	if !errors.As(err, &stub) {
		t.Fatalf("errors.As(%T, *protocol.NotImplementedError) = false", err)
	}
	if stub.Module != "protocol" || stub.Operation != operation {
		t.Fatalf("stub = %#v, want module protocol operation %q", stub, operation)
	}
}

func TestProtocolVersionSupport(t *testing.T) {
	t.Parallel()

	if protocol.ProtocolVersion != 1 {
		t.Fatalf("ProtocolVersion = %d, want 1", protocol.ProtocolVersion)
	}

	for _, test := range []struct {
		name    string
		version int
		want    bool
	}{
		{name: "current", version: 1, want: true},
		{name: "zero", version: 0, want: false},
		{name: "future", version: 2, want: false},
		{name: "negative", version: -1, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := protocol.IsSupportedProtocolVersion(test.version); got != test.want {
				t.Fatalf("IsSupportedProtocolVersion(%d) = %t, want %t", test.version, got, test.want)
			}
		})
	}
}

func TestSchemaDescriptorIsAnHonestCapabilityStub(t *testing.T) {
	t.Parallel()

	if got := protocol.ModelRefSchema.Name(); got != "ModelRef" {
		t.Fatalf("ModelRefSchema.Name() = %q, want ModelRef", got)
	}

	model := protocol.ModelRef{Provider: "example", ID: "model-1"}
	requireNotImplemented(t, protocol.ModelRefSchema.Validate(model), "ModelRefSchema.Validate")

	parsed, err := protocol.ModelRefSchema.Parse(map[string]any{
		"provider": "example",
		"id":       "model-1",
	})
	if parsed != (protocol.ModelRef{}) {
		t.Fatalf("ModelRefSchema.Parse() value = %#v, want zero value", parsed)
	}
	requireNotImplemented(t, err, "ModelRefSchema.Parse")
}

func TestServerSnapshotPublicShapePreservesWireValues(t *testing.T) {
	t.Parallel()

	updatedAt := protocol.Some[int64](42)
	snapshot := protocol.ServerSnapshot{
		ServerID:        "server-1",
		ProtocolVersion: protocol.ProtocolVersion,
		Revision:        7,
		Sessions: []protocol.SessionMetadata{{
			ID:        "session-1",
			CreatedAt: 1,
			UpdatedAt: updatedAt,
		}},
		Models: []protocol.ModelMetadata{{
			Provider:                "example",
			ID:                      "model-1",
			Name:                    "Example",
			API:                     "example-api",
			Reasoning:               true,
			Input:                   []protocol.ModelInputKind{protocol.ModelInputText, protocol.ModelInputImage},
			ContextWindow:           128_000,
			MaxTokens:               16_000,
			Cost:                    protocol.ModelCost{},
			SupportedThinkingLevels: []protocol.ThinkingLevel{protocol.ThinkingLevelOff, protocol.ThinkingLevelXHigh},
			Authenticated:           true,
		}},
	}

	if snapshot.Sessions[0].UpdatedAt != updatedAt {
		t.Fatalf("UpdatedAt = %#v, want %#v", snapshot.Sessions[0].UpdatedAt, updatedAt)
	}
	if protocol.None[string]().Present {
		t.Fatal("None[string]().Present = true, want false")
	}
	if got := string(snapshot.Models[0].SupportedThinkingLevels[1]); got != "xhigh" {
		t.Fatalf("ThinkingLevelXHigh = %q, want xhigh", got)
	}
	if got := string(snapshot.Models[0].Input[1]); got != "image" {
		t.Fatalf("ModelInputImage = %q, want image", got)
	}
}

func TestSessionSnapshotAndServerEventsPreserveClosedWireShapes(t *testing.T) {
	t.Parallel()

	userItem := protocol.UserTranscriptItem{
		ID:   "message-1",
		Role: protocol.TranscriptRoleUser,
		Content: []protocol.UserContent{protocol.TextContent{
			Type: protocol.ContentTypeText,
			Text: "hello",
		}},
		Timestamp: 10,
	}
	session := protocol.SessionSnapshot{
		ID:               "session-1",
		CWD:              "/work",
		CreatedAt:        1,
		UpdatedAt:        2,
		Phase:            protocol.SessionPhaseIdle,
		Model:            protocol.ModelRef{Provider: "example", ID: "model-1"},
		ThinkingLevel:    protocol.ThinkingLevelMedium,
		Attached:         true,
		Revision:         3,
		Transcript:       []protocol.TranscriptItem{userItem},
		QueuedSteer:      []protocol.UserTranscriptItem{},
		QueuedSteerCount: 0,
	}

	events := []protocol.ServerEvent{
		protocol.ServerSnapshotEvent{
			Type:     protocol.ServerEventTypeServerSnapshot,
			Snapshot: protocol.ServerSnapshot{ServerID: "server-1", ProtocolVersion: 1},
		},
		protocol.SessionSnapshotEvent{
			Type:     protocol.ServerEventTypeSessionSnapshot,
			Snapshot: session,
		},
		protocol.SessionRemovedEvent{
			Type:      protocol.ServerEventTypeSessionRemoved,
			SessionID: session.ID,
		},
	}
	if got := events[1].ServerEventType(); got != protocol.ServerEventTypeSessionSnapshot {
		t.Fatalf("ServerEventType() = %q, want session_snapshot", got)
	}

	wireErr := protocol.ProtocolError{
		Code:    protocol.ProtocolErrorCodeNotImplemented,
		Message: "deferred",
		Details: protocol.Some[protocol.JSONValue](nil),
	}
	if !wireErr.Details.Present || wireErr.Details.Value != nil {
		t.Fatalf("Details = %#v, want explicitly present null", wireErr.Details)
	}
}

func TestTranscriptPublicShapeCoversEveryWireVariant(t *testing.T) {
	t.Parallel()

	text := protocol.TextContent{Type: protocol.ContentTypeText, Text: "answer"}
	image := protocol.ImageContent{Type: protocol.ContentTypeImage, Data: "base64", MIMEType: "image/png"}
	thinking := protocol.ThinkingContent{
		Type:     protocol.ContentTypeThinking,
		Thinking: "reasoning",
		Redacted: protocol.Some(true),
	}
	toolCall := protocol.ToolCallContent{
		Type:       protocol.ContentTypeToolCall,
		ToolCallID: "call-1",
		ToolName:   "read",
		Input:      map[string]any{"path": "README.md"},
	}

	var _ protocol.UserContent = image
	var _ protocol.AssistantContent = thinking
	var _ protocol.AssistantContent = toolCall
	var _ protocol.ToolContent = text

	usage := protocol.Usage{
		Input:       1,
		Output:      2,
		CacheRead:   3,
		CacheWrite:  4,
		Reasoning:   protocol.Some[int64](5),
		TotalTokens: 15,
		Cost:        protocol.UsageCost{Total: 0.01},
	}
	streamingAssistant := protocol.StreamingAssistantTranscriptItem{
		ID: "a1", Role: protocol.TranscriptRoleAssistant, Content: []protocol.AssistantContent{text},
		Model: protocol.ModelRef{Provider: "p", ID: "m"}, Timestamp: 1,
		Status: protocol.AssistantTranscriptStatusStreaming,
	}
	completeAssistant := protocol.CompleteAssistantTranscriptItem{
		ID: "a2", Role: protocol.TranscriptRoleAssistant, Content: []protocol.AssistantContent{text},
		Model: protocol.ModelRef{Provider: "p", ID: "m"}, Usage: protocol.Some(usage), Timestamp: 2,
		Status: protocol.AssistantTranscriptStatusComplete, StopReason: protocol.AssistantStopReasonToolUse,
	}
	assistantVariants := []protocol.AssistantTranscriptItem{
		streamingAssistant,
		completeAssistant,
		protocol.ErrorAssistantTranscriptItem{
			ID: "a3", Role: protocol.TranscriptRoleAssistant, Model: protocol.ModelRef{Provider: "p", ID: "m"},
			Timestamp: 3, Status: protocol.AssistantTranscriptStatusError, StopReason: protocol.AssistantStopReasonError,
		},
		protocol.AbortedAssistantTranscriptItem{
			ID: "a4", Role: protocol.TranscriptRoleAssistant, Model: protocol.ModelRef{Provider: "p", ID: "m"},
			Timestamp: 4, Status: protocol.AssistantTranscriptStatusAborted, StopReason: protocol.AssistantStopReasonAborted,
		},
	}
	runningTool := protocol.RunningToolTranscriptItem{
		ID: "t1", Role: protocol.TranscriptRoleTool, ToolCallID: "call-1", ToolName: "read",
		Input: nil, Content: []protocol.ToolContent{text, image}, Timestamp: 5,
		Status: protocol.ToolTranscriptStatusRunning, IsError: false,
	}
	toolVariants := []protocol.ToolTranscriptItem{
		runningTool,
		protocol.CompleteToolTranscriptItem{
			ID: "t2", Role: protocol.TranscriptRoleTool, ToolCallID: "call-2", ToolName: "read",
			Input: nil, Timestamp: 6, Status: protocol.ToolTranscriptStatusComplete, IsError: false,
		},
		protocol.ErrorToolTranscriptItem{
			ID: "t3", Role: protocol.TranscriptRoleTool, ToolCallID: "call-3", ToolName: "read",
			Input: nil, Timestamp: 7, Status: protocol.ToolTranscriptStatusError, IsError: true,
		},
	}
	var _ protocol.TranscriptItem = assistantVariants[0]
	var _ protocol.TranscriptItem = toolVariants[0]

	progress := []protocol.TranscriptProgress{
		protocol.ItemStartedProgress{Type: protocol.TranscriptProgressTypeItemStarted, Item: streamingAssistant},
		protocol.AssistantDeltaProgress{
			Type: protocol.TranscriptProgressTypeAssistantDelta, MessageID: "a1", ContentIndex: 0,
			Kind: protocol.AssistantDeltaKindToolCall, Delta: "{}",
		},
		protocol.ItemUpdatedProgress{Type: protocol.TranscriptProgressTypeItemUpdated, Item: runningTool},
		protocol.ItemFinishedProgress{Type: protocol.TranscriptProgressTypeItemFinished, Item: completeAssistant},
	}
	if got := progress[1].TranscriptProgressType(); got != protocol.TranscriptProgressTypeAssistantDelta {
		t.Fatalf("TranscriptProgressType() = %q, want assistant_delta", got)
	}
}

func TestCommandAndMessagePublicShapeCoversEveryWireVariant(t *testing.T) {
	t.Parallel()

	model := protocol.ModelRef{Provider: "example", ID: "model-1"}
	commands := []protocol.Command{
		protocol.ListCommand{Command: protocol.CommandNameList},
		protocol.CreateCommand{
			Command: protocol.CommandNameCreate, CWD: protocol.Some("/work"),
			Name: protocol.Some("demo"), Model: protocol.Some(model),
			ThinkingLevel: protocol.Some(protocol.ThinkingLevelHigh),
		},
		protocol.AttachCommand{Command: protocol.CommandNameAttach, SessionID: "s1"},
		protocol.DetachCommand{Command: protocol.CommandNameDetach, SessionID: "s1"},
		protocol.PromptCommand{Command: protocol.CommandNamePrompt, SessionID: "s1", Text: "hello"},
		protocol.SteerCommand{Command: protocol.CommandNameSteer, SessionID: "s1", Text: "change course"},
		protocol.AbortCommand{Command: protocol.CommandNameAbort, SessionID: "s1"},
		protocol.SetModelCommand{Command: protocol.CommandNameSetModel, SessionID: "s1", Model: model},
		protocol.SetThinkingCommand{
			Command: protocol.CommandNameSetThinking, SessionID: "s1", ThinkingLevel: protocol.ThinkingLevelMax,
		},
	}
	wantCommandNames := []protocol.CommandName{
		protocol.CommandNameList, protocol.CommandNameCreate, protocol.CommandNameAttach,
		protocol.CommandNameDetach, protocol.CommandNamePrompt, protocol.CommandNameSteer,
		protocol.CommandNameAbort, protocol.CommandNameSetModel, protocol.CommandNameSetThinking,
	}
	for i, command := range commands {
		if got := command.CommandName(); got != wantCommandNames[i] {
			t.Fatalf("commands[%d].CommandName() = %q, want %q", i, got, wantCommandNames[i])
		}
	}

	session := protocol.SessionSnapshot{ID: "s1"}
	results := []protocol.CommandResult{
		protocol.ListResult{Command: protocol.CommandNameList, Sessions: []protocol.SessionMetadata{}},
		protocol.CreateResult{Command: protocol.CommandNameCreate, Session: session},
		protocol.AttachResult{Command: protocol.CommandNameAttach, Session: session},
		protocol.DetachResult{Command: protocol.CommandNameDetach, SessionID: "s1"},
		protocol.PromptResult{Command: protocol.CommandNamePrompt, Session: session},
		protocol.SteerResult{Command: protocol.CommandNameSteer, Session: session},
		protocol.AbortResult{Command: protocol.CommandNameAbort, Session: session},
		protocol.SetModelResult{Command: protocol.CommandNameSetModel, Session: session},
		protocol.SetThinkingResult{Command: protocol.CommandNameSetThinking, Session: session},
	}
	for i, result := range results {
		if got := result.ResultCommandName(); got != wantCommandNames[i] {
			t.Fatalf("results[%d].ResultCommandName() = %q, want %q", i, got, wantCommandNames[i])
		}
	}
	var _ protocol.ResultForCommand[protocol.ListCommand] = protocol.ListResult{}

	clientMessages := []protocol.ClientMessage{
		protocol.ClientHello{Type: protocol.MessageTypeHello, Version: protocol.ProtocolVersion},
		protocol.RequestEnvelope{Type: protocol.MessageTypeRequest, ID: "r1", Request: commands[0]},
	}
	if got := clientMessages[1].ClientMessageType(); got != protocol.MessageTypeRequest {
		t.Fatalf("ClientMessageType() = %q, want request", got)
	}

	wireErr := protocol.ProtocolError{Code: protocol.ProtocolErrorCodeInvalidRequest, Message: "invalid"}
	serverMessages := []protocol.ServerMessage{
		protocol.ServerHello{
			Type: protocol.MessageTypeHello, Version: protocol.ProtocolVersion, ConnectionID: "c1",
			Snapshot: protocol.ServerSnapshot{ServerID: "server-1", ProtocolVersion: protocol.ProtocolVersion},
		},
		protocol.ServerHelloError{Type: protocol.MessageTypeHelloError, Error: wireErr},
		protocol.SuccessResponseEnvelope{
			Type: protocol.MessageTypeResponse, ID: "r1", OK: true, Result: results[0],
		},
		protocol.ErrorResponseEnvelope{Type: protocol.MessageTypeResponse, ID: "r2", OK: false, Error: wireErr},
		protocol.EventEnvelope{
			Type: protocol.MessageTypeEvent,
			Event: protocol.SessionRemovedEvent{
				Type: protocol.ServerEventTypeSessionRemoved, SessionID: "s1",
			},
		},
	}
	var _ protocol.ResponseEnvelope = serverMessages[2].(protocol.SuccessResponseEnvelope)
	if got := serverMessages[4].ServerMessageType(); got != protocol.MessageTypeEvent {
		t.Fatalf("ServerMessageType() = %q, want event", got)
	}
}

func TestEverySchemaDescriptorHasStableIdentity(t *testing.T) {
	t.Parallel()

	descriptors := []interface{ Name() string }{
		protocol.JSONValueSchema,
		protocol.ThinkingLevelSchema,
		protocol.SessionPhaseSchema,
		protocol.ModelRefSchema,
		protocol.ModelCostSchema,
		protocol.ModelMetadataSchema,
		protocol.TextContentSchema,
		protocol.ThinkingContentSchema,
		protocol.ImageContentSchema,
		protocol.ToolCallContentSchema,
		protocol.UserContentSchema,
		protocol.AssistantContentSchema,
		protocol.ToolContentSchema,
		protocol.UsageSchema,
		protocol.UserTranscriptItemSchema,
		protocol.AssistantTranscriptItemSchema,
		protocol.ToolTranscriptItemSchema,
		protocol.TranscriptItemSchema,
		protocol.TranscriptProgressSchema,
		protocol.SessionMetadataSchema,
		protocol.SessionSnapshotSchema,
		protocol.ServerSnapshotSchema,
		protocol.ProtocolErrorCodeSchema,
		protocol.ProtocolErrorSchema,
		protocol.ListCommandSchema,
		protocol.CreateCommandSchema,
		protocol.AttachCommandSchema,
		protocol.DetachCommandSchema,
		protocol.PromptCommandSchema,
		protocol.SteerCommandSchema,
		protocol.AbortCommandSchema,
		protocol.SetModelCommandSchema,
		protocol.SetThinkingCommandSchema,
		protocol.CommandSchema,
		protocol.CreateResultSchema,
		protocol.AttachResultSchema,
		protocol.PromptResultSchema,
		protocol.SteerResultSchema,
		protocol.AbortResultSchema,
		protocol.SetModelResultSchema,
		protocol.SetThinkingResultSchema,
		protocol.ListResultSchema,
		protocol.DetachResultSchema,
		protocol.CommandResultSchema,
		protocol.ClientHelloSchema,
		protocol.RequestEnvelopeSchema,
		protocol.ClientMessageSchema,
		protocol.ServerEventSchema,
		protocol.ServerHelloSchema,
		protocol.ServerHelloErrorSchema,
		protocol.ResponseEnvelopeSchema,
		protocol.EventEnvelopeSchema,
		protocol.ServerMessageSchema,
	}
	want := []string{
		"JSONValue", "ThinkingLevel", "SessionPhase", "ModelRef", "ModelCost", "ModelMetadata",
		"TextContent", "ThinkingContent", "ImageContent", "ToolCallContent", "UserContent",
		"AssistantContent", "ToolContent", "Usage", "UserTranscriptItem", "AssistantTranscriptItem",
		"ToolTranscriptItem", "TranscriptItem", "TranscriptProgress", "SessionMetadata", "SessionSnapshot",
		"ServerSnapshot", "ProtocolErrorCode", "ProtocolError", "ListCommand", "CreateCommand",
		"AttachCommand", "DetachCommand", "PromptCommand", "SteerCommand", "AbortCommand",
		"SetModelCommand", "SetThinkingCommand", "Command", "CreateResult", "AttachResult",
		"PromptResult", "SteerResult", "AbortResult", "SetModelResult", "SetThinkingResult",
		"ListResult", "DetachResult", "CommandResult", "ClientHello", "RequestEnvelope",
		"ClientMessage", "ServerEvent", "ServerHello", "ServerHelloError", "ResponseEnvelope",
		"EventEnvelope", "ServerMessage",
	}
	if len(descriptors) != len(want) {
		t.Fatalf("descriptor count = %d, want %d", len(descriptors), len(want))
	}
	for i, descriptor := range descriptors {
		if got := descriptor.Name(); got != want[i] {
			t.Errorf("descriptor[%d].Name() = %q, want %q", i, got, want[i])
		}
	}

	parsed, err := protocol.ServerMessageSchema.Parse(protocol.ServerHello{})
	if parsed != nil {
		t.Fatalf("ServerMessageSchema.Parse() = %#v, want nil", parsed)
	}
	requireNotImplemented(t, err, "ServerMessageSchema.Parse")
}

func TestCBORAndFramingAreInertCapabilityStubs(t *testing.T) {
	if protocol.DefaultMaxCBORByteLength != 16*1024*1024 {
		t.Fatalf("DefaultMaxCBORByteLength = %d, want 16777216", protocol.DefaultMaxCBORByteLength)
	}
	if protocol.DefaultMaxCBORContainerLength != 1_000_000 {
		t.Fatalf("DefaultMaxCBORContainerLength = %d, want 1000000", protocol.DefaultMaxCBORContainerLength)
	}
	if protocol.DefaultMaxCBORDepth != 64 {
		t.Fatalf("DefaultMaxCBORDepth = %d, want 64", protocol.DefaultMaxCBORDepth)
	}
	if protocol.DefaultMaxFrameLength != 16*1024*1024 {
		t.Fatalf("DefaultMaxFrameLength = %d, want 16777216", protocol.DefaultMaxFrameLength)
	}

	zero := uint32(0)
	cborOptions := &protocol.CBOROptions{
		MaxByteLength:      &zero,
		MaxContainerLength: &zero,
		MaxDepth:           &zero,
	}
	value := map[string]any{"nested": []any{1.0, true, nil}}
	wantValue := map[string]any{"nested": []any{1.0, true, nil}}
	encoded, err := protocol.EncodeCBOR(value, cborOptions)
	if encoded != nil {
		t.Fatalf("EncodeCBOR() = %v, want nil", encoded)
	}
	requireNotImplemented(t, err, "EncodeCBOR")
	if !reflect.DeepEqual(value, wantValue) {
		t.Fatalf("EncodeCBOR mutated input: got %#v, want %#v", value, wantValue)
	}

	input := []byte{0xa1, 0x61, 0x61, 0x01}
	wantInput := bytes.Clone(input)
	decoded, err := protocol.DecodeCBOR(input, cborOptions)
	if decoded != nil {
		t.Fatalf("DecodeCBOR() = %#v, want nil", decoded)
	}
	requireNotImplemented(t, err, "DecodeCBOR")
	if !bytes.Equal(input, wantInput) {
		t.Fatalf("DecodeCBOR mutated input: got %x, want %x", input, wantInput)
	}

	payload := []byte{1, 2, 3}
	wantPayload := bytes.Clone(payload)
	frame, err := protocol.EncodeFrame(payload)
	if frame != nil {
		t.Fatalf("EncodeFrame() = %x, want nil", frame)
	}
	requireNotImplemented(t, err, "EncodeFrame")
	if !bytes.Equal(payload, wantPayload) {
		t.Fatalf("EncodeFrame mutated payload: got %x, want %x", payload, wantPayload)
	}

	frameOptions := &protocol.FrameDecoderOptions{MaxFrameLength: &zero}
	requireNotImplemented(t, protocol.AssertCompleteFrame(input, frameOptions), "AssertCompleteFrame")
	if !bytes.Equal(input, wantInput) {
		t.Fatalf("AssertCompleteFrame mutated frame: got %x, want %x", input, wantInput)
	}

	decoder, err := protocol.NewFrameDecoder(frameOptions)
	if decoder != nil {
		t.Fatalf("NewFrameDecoder() = %#v, want nil", decoder)
	}
	requireNotImplemented(t, err, "NewFrameDecoder")
	if fields := reflect.TypeOf(protocol.FrameDecoder{}).NumField(); fields != 0 {
		t.Fatalf("FrameDecoder has %d fields, want zero-state stub", fields)
	}
	var zeroDecoder protocol.FrameDecoder
	frames, err := zeroDecoder.Push(input)
	if frames != nil {
		t.Fatalf("FrameDecoder.Push() = %v, want nil", frames)
	}
	requireNotImplemented(t, err, "FrameDecoder.Push")
	requireNotImplemented(t, zeroDecoder.End(), "FrameDecoder.End")
	if !bytes.Equal(input, wantInput) {
		t.Fatalf("FrameDecoder.Push mutated chunk: got %x, want %x", input, wantInput)
	}
}

func TestCodecAndMessageDecodersAreInertCapabilityStubs(t *testing.T) {
	clientInput := map[string]any{
		"type":    "request",
		"id":      "request-1",
		"request": map[string]any{"command": "list"},
	}
	wantClientInput := map[string]any{
		"type":    "request",
		"id":      "request-1",
		"request": map[string]any{"command": "list"},
	}
	clientMessage, err := protocol.ParseClientMessage(clientInput)
	if clientMessage != nil {
		t.Fatalf("ParseClientMessage() = %#v, want nil", clientMessage)
	}
	requireNotImplemented(t, err, "ParseClientMessage")
	if !reflect.DeepEqual(clientInput, wantClientInput) {
		t.Fatalf("ParseClientMessage mutated input: got %#v, want %#v", clientInput, wantClientInput)
	}

	serverInput := map[string]any{"type": "hello_error"}
	wantServerInput := map[string]any{"type": "hello_error"}
	serverMessage, err := protocol.ParseServerMessage(serverInput)
	if serverMessage != nil {
		t.Fatalf("ParseServerMessage() = %#v, want nil", serverMessage)
	}
	requireNotImplemented(t, err, "ParseServerMessage")
	if !reflect.DeepEqual(serverInput, wantServerInput) {
		t.Fatalf("ParseServerMessage mutated input: got %#v, want %#v", serverInput, wantServerInput)
	}

	client := protocol.RequestEnvelope{
		Type: protocol.MessageTypeRequest, ID: "request-1",
		Request: protocol.ListCommand{Command: protocol.CommandNameList},
	}
	wantClient := client
	encoded, err := protocol.EncodeClientMessage(client, nil)
	if encoded != nil {
		t.Fatalf("EncodeClientMessage() = %x, want nil", encoded)
	}
	requireNotImplemented(t, err, "EncodeClientMessage")
	if !reflect.DeepEqual(client, wantClient) {
		t.Fatalf("EncodeClientMessage mutated input: got %#v, want %#v", client, wantClient)
	}

	server := protocol.ServerHello{
		Type: protocol.MessageTypeHello, Version: protocol.ProtocolVersion, ConnectionID: "connection-1",
		Snapshot: protocol.ServerSnapshot{
			ServerID: "server-1", ProtocolVersion: protocol.ProtocolVersion,
			Sessions: []protocol.SessionMetadata{{ID: "session-1"}},
		},
	}
	wantServer := server
	encoded, err = protocol.EncodeServerMessage(server, nil)
	if encoded != nil {
		t.Fatalf("EncodeServerMessage() = %x, want nil", encoded)
	}
	requireNotImplemented(t, err, "EncodeServerMessage")
	if !reflect.DeepEqual(server, wantServer) {
		t.Fatalf("EncodeServerMessage mutated input: got %#v, want %#v", server, wantServer)
	}

	clientDecoder, err := protocol.NewClientMessageDecoder(nil)
	if clientDecoder != nil {
		t.Fatalf("NewClientMessageDecoder() = %#v, want nil", clientDecoder)
	}
	requireNotImplemented(t, err, "NewClientMessageDecoder")
	serverDecoder, err := protocol.NewServerMessageDecoder(nil)
	if serverDecoder != nil {
		t.Fatalf("NewServerMessageDecoder() = %#v, want nil", serverDecoder)
	}
	requireNotImplemented(t, err, "NewServerMessageDecoder")

	if fields := reflect.TypeOf(protocol.ClientMessageDecoder{}).NumField(); fields != 0 {
		t.Fatalf("ClientMessageDecoder has %d fields, want zero-state stub", fields)
	}
	if fields := reflect.TypeOf(protocol.ServerMessageDecoder{}).NumField(); fields != 0 {
		t.Fatalf("ServerMessageDecoder has %d fields, want zero-state stub", fields)
	}
	chunk := []byte{0, 0, 0, 0}
	wantChunk := bytes.Clone(chunk)
	var zeroClientDecoder protocol.ClientMessageDecoder
	clientMessages, err := zeroClientDecoder.Push(chunk)
	if clientMessages != nil {
		t.Fatalf("ClientMessageDecoder.Push() = %#v, want nil", clientMessages)
	}
	requireNotImplemented(t, err, "ClientMessageDecoder.Push")
	requireNotImplemented(t, zeroClientDecoder.End(), "ClientMessageDecoder.End")
	var zeroServerDecoder protocol.ServerMessageDecoder
	serverMessages, err := zeroServerDecoder.Push(chunk)
	if serverMessages != nil {
		t.Fatalf("ServerMessageDecoder.Push() = %#v, want nil", serverMessages)
	}
	requireNotImplemented(t, err, "ServerMessageDecoder.Push")
	requireNotImplemented(t, zeroServerDecoder.End(), "ServerMessageDecoder.End")
	if !bytes.Equal(chunk, wantChunk) {
		t.Fatalf("message decoder mutated chunk: got %x, want %x", chunk, wantChunk)
	}
}

func TestNamedEnumsPreserveEveryWireIdentifier(t *testing.T) {
	t.Parallel()

	thinkingLevels := []protocol.ThinkingLevel{
		protocol.ThinkingLevelOff, protocol.ThinkingLevelMinimal, protocol.ThinkingLevelLow,
		protocol.ThinkingLevelMedium, protocol.ThinkingLevelHigh, protocol.ThinkingLevelXHigh, protocol.ThinkingLevelMax,
	}
	if got, want := fmt.Sprint(thinkingLevels), "[off minimal low medium high xhigh max]"; got != want {
		t.Fatalf("thinking levels = %s, want %s", got, want)
	}

	phases := []protocol.SessionPhase{
		protocol.SessionPhaseIdle, protocol.SessionPhaseTurn, protocol.SessionPhaseCompaction,
		protocol.SessionPhaseBranchSummary, protocol.SessionPhaseRetry,
	}
	if got, want := fmt.Sprint(phases), "[idle turn compaction branch_summary retry]"; got != want {
		t.Fatalf("session phases = %s, want %s", got, want)
	}

	errorCodes := []protocol.ProtocolErrorCode{
		protocol.ProtocolErrorCodeVersion, protocol.ProtocolErrorCodeBusy, protocol.ProtocolErrorCodeSessionLocked,
		protocol.ProtocolErrorCodeNotFound, protocol.ProtocolErrorCodeInvalidRequest,
		protocol.ProtocolErrorCodeNotImplemented, protocol.ProtocolErrorCodeInternalError,
	}
	if got, want := fmt.Sprint(errorCodes), "[version busy session_locked not_found invalid_request not_implemented internal_error]"; got != want {
		t.Fatalf("protocol error codes = %s, want %s", got, want)
	}

	commandNames := []protocol.CommandName{
		protocol.CommandNameList, protocol.CommandNameCreate, protocol.CommandNameAttach, protocol.CommandNameDetach,
		protocol.CommandNamePrompt, protocol.CommandNameSteer, protocol.CommandNameAbort,
		protocol.CommandNameSetModel, protocol.CommandNameSetThinking,
	}
	if got, want := fmt.Sprint(commandNames), "[list create attach detach prompt steer abort set_model set_thinking]"; got != want {
		t.Fatalf("command names = %s, want %s", got, want)
	}
}

// Compile-time root surface parity: every one of the 110 exports on Pi's
// protocol root (`.`) export subpath maps to a compile-usable Pig declaration.
// Go spellings use exported identifiers and idiomatic initialisms (CBOR, JSON),
// while constructors use New. Each marker preserves the exact upstream name in
// parity/surface/symbols.jsonl for machine-checkable set comparison.
var (
	_ func([]byte, *protocol.CBOROptions) (any, error)                            = protocol.DecodeCBOR                         // upstream: decodeCbor
	_ func(any, *protocol.CBOROptions) ([]byte, error)                            = protocol.EncodeCBOR                         // upstream: encodeCbor
	_ error                                                                       = (*protocol.CBORError)(nil)                  // upstream: CborError
	_ protocol.CBOROptions                                                                                                      // upstream: CborOptions
	_ uint32                                                                      = protocol.DefaultMaxCBORByteLength           // upstream: DEFAULT_MAX_CBOR_BYTE_LENGTH
	_ uint32                                                                      = protocol.DefaultMaxCBORContainerLength      // upstream: DEFAULT_MAX_CBOR_CONTAINER_LENGTH
	_ uint32                                                                      = protocol.DefaultMaxCBORDepth                // upstream: DEFAULT_MAX_CBOR_DEPTH
	_ protocol.ClientMessageDecoder                                                                                             // upstream: ClientMessageDecoder
	_ error                                                                       = (*protocol.ProtocolValidationError)(nil)    // upstream: ProtocolValidationError
	_ protocol.ServerMessageDecoder                                                                                             // upstream: ServerMessageDecoder
	_ func(*protocol.FrameDecoderOptions) (*protocol.ClientMessageDecoder, error) = protocol.NewClientMessageDecoder            // upstream: createClientMessageDecoder
	_ func(*protocol.FrameDecoderOptions) (*protocol.ServerMessageDecoder, error) = protocol.NewServerMessageDecoder            // upstream: createServerMessageDecoder
	_ func(protocol.ClientMessage, *protocol.FrameDecoderOptions) ([]byte, error) = protocol.EncodeClientMessage                // upstream: encodeClientMessage
	_ func(protocol.ServerMessage, *protocol.FrameDecoderOptions) ([]byte, error) = protocol.EncodeServerMessage                // upstream: encodeServerMessage
	_ func(int) bool                                                              = protocol.IsSupportedProtocolVersion         // upstream: isSupportedProtocolVersion
	_ func(any) (protocol.ClientMessage, error)                                   = protocol.ParseClientMessage                 // upstream: parseClientMessage
	_ func(any) (protocol.ServerMessage, error)                                   = protocol.ParseServerMessage                 // upstream: parseServerMessage
	_ uint32                                                                      = protocol.DefaultMaxFrameLength              // upstream: DEFAULT_MAX_FRAME_LENGTH
	_ protocol.FrameDecoder                                                                                                     // upstream: FrameDecoder
	_ protocol.FrameDecoderOptions                                                                                              // upstream: FrameDecoderOptions
	_ error                                                                       = (*protocol.FrameError)(nil)                 // upstream: FrameError
	_ func([]byte, *protocol.FrameDecoderOptions) error                           = protocol.AssertCompleteFrame                // upstream: assertCompleteFrame
	_ func([]byte) ([]byte, error)                                                = protocol.EncodeFrame                        // upstream: encodeFrame
	_ protocol.Schema[protocol.AbortCommand]                                      = protocol.AbortCommandSchema                 // upstream: AbortCommandSchema
	_ protocol.Schema[protocol.AbortResult]                                       = protocol.AbortResultSchema                  // upstream: AbortResultSchema
	_ protocol.Schema[protocol.AssistantContent]                                  = protocol.AssistantContentSchema             // upstream: AssistantContentSchema
	_ protocol.AssistantTranscriptItem                                            = protocol.StreamingAssistantTranscriptItem{} // upstream: AssistantTranscriptItem
	_ protocol.Schema[protocol.AssistantTranscriptItem]                           = protocol.AssistantTranscriptItemSchema      // upstream: AssistantTranscriptItemSchema
	_ protocol.Schema[protocol.AttachCommand]                                     = protocol.AttachCommandSchema                // upstream: AttachCommandSchema
	_ protocol.Schema[protocol.AttachResult]                                      = protocol.AttachResultSchema                 // upstream: AttachResultSchema
	_ protocol.ClientHello                                                                                                      // upstream: ClientHello
	_ protocol.Schema[protocol.ClientHello]                                       = protocol.ClientHelloSchema                  // upstream: ClientHelloSchema
	_ protocol.ClientMessage                                                      = protocol.ClientHello{}                      // upstream: ClientMessage
	_ protocol.Schema[protocol.ClientMessage]                                     = protocol.ClientMessageSchema                // upstream: ClientMessageSchema
	_ protocol.Command                                                            = protocol.ListCommand{}                      // upstream: Command
	_ protocol.CommandName                                                                                                      // upstream: CommandName
	_ protocol.CommandResult                                                      = protocol.ListResult{}                       // upstream: CommandResult
	_ protocol.Schema[protocol.CommandResult]                                     = protocol.CommandResultSchema                // upstream: CommandResultSchema
	_ protocol.Schema[protocol.Command]                                           = protocol.CommandSchema                      // upstream: CommandSchema
	_ protocol.Schema[protocol.CreateCommand]                                     = protocol.CreateCommandSchema                // upstream: CreateCommandSchema
	_ protocol.Schema[protocol.CreateResult]                                      = protocol.CreateResultSchema                 // upstream: CreateResultSchema
	_ protocol.Schema[protocol.DetachCommand]                                     = protocol.DetachCommandSchema                // upstream: DetachCommandSchema
	_ protocol.Schema[protocol.DetachResult]                                      = protocol.DetachResultSchema                 // upstream: DetachResultSchema
	_ protocol.EventEnvelope                                                                                                    // upstream: EventEnvelope
	_ protocol.Schema[protocol.EventEnvelope]                                     = protocol.EventEnvelopeSchema                // upstream: EventEnvelopeSchema
	_ protocol.ImageContent                                                                                                     // upstream: ImageContent
	_ protocol.Schema[protocol.ImageContent]                                      = protocol.ImageContentSchema                 // upstream: ImageContentSchema
	_ protocol.JSONValue                                                          = nil                                         // upstream: JsonValue
	_ protocol.Schema[protocol.JSONValue]                                         = protocol.JSONValueSchema                    // upstream: JsonValueSchema
	_ protocol.Schema[protocol.ListCommand]                                       = protocol.ListCommandSchema                  // upstream: ListCommandSchema
	_ protocol.Schema[protocol.ListResult]                                        = protocol.ListResultSchema                   // upstream: ListResultSchema
	_ protocol.Schema[protocol.ModelCost]                                         = protocol.ModelCostSchema                    // upstream: ModelCostSchema
	_ protocol.ModelMetadata                                                                                                    // upstream: ModelMetadata
	_ protocol.Schema[protocol.ModelMetadata]                                     = protocol.ModelMetadataSchema                // upstream: ModelMetadataSchema
	_ protocol.ModelRef                                                                                                         // upstream: ModelRef
	_ protocol.Schema[protocol.ModelRef]                                          = protocol.ModelRefSchema                     // upstream: ModelRefSchema
	_ int                                                                         = protocol.ProtocolVersion                    // upstream: PROTOCOL_VERSION
	_ protocol.Schema[protocol.PromptCommand]                                     = protocol.PromptCommandSchema                // upstream: PromptCommandSchema
	_ protocol.Schema[protocol.PromptResult]                                      = protocol.PromptResultSchema                 // upstream: PromptResultSchema
	_ protocol.ProtocolError                                                                                                    // upstream: ProtocolError
	_ protocol.ProtocolErrorCode                                                                                                // upstream: ProtocolErrorCode
	_ protocol.Schema[protocol.ProtocolErrorCode]                                 = protocol.ProtocolErrorCodeSchema            // upstream: ProtocolErrorCodeSchema
	_ protocol.Schema[protocol.ProtocolError]                                     = protocol.ProtocolErrorSchema                // upstream: ProtocolErrorSchema
	_ protocol.RequestEnvelope                                                                                                  // upstream: RequestEnvelope
	_ protocol.Schema[protocol.RequestEnvelope]                                   = protocol.RequestEnvelopeSchema              // upstream: RequestEnvelopeSchema
	_ protocol.ResponseEnvelope                                                   = protocol.SuccessResponseEnvelope{}          // upstream: ResponseEnvelope
	_ protocol.Schema[protocol.ResponseEnvelope]                                  = protocol.ResponseEnvelopeSchema             // upstream: ResponseEnvelopeSchema
	_ protocol.ResultForCommand[protocol.ListCommand]                             = protocol.ListResult{}                       // upstream: ResultForCommand
	_ protocol.ServerEvent                                                        = protocol.ServerSnapshotEvent{}              // upstream: ServerEvent
	_ protocol.Schema[protocol.ServerEvent]                                       = protocol.ServerEventSchema                  // upstream: ServerEventSchema
	_ protocol.ServerHello                                                                                                      // upstream: ServerHello
	_ protocol.ServerHelloError                                                                                                 // upstream: ServerHelloError
	_ protocol.Schema[protocol.ServerHelloError]                                  = protocol.ServerHelloErrorSchema             // upstream: ServerHelloErrorSchema
	_ protocol.Schema[protocol.ServerHello]                                       = protocol.ServerHelloSchema                  // upstream: ServerHelloSchema
	_ protocol.ServerMessage                                                      = protocol.ServerHello{}                      // upstream: ServerMessage
	_ protocol.Schema[protocol.ServerMessage]                                     = protocol.ServerMessageSchema                // upstream: ServerMessageSchema
	_ protocol.ServerSnapshot                                                                                                   // upstream: ServerSnapshot
	_ protocol.Schema[protocol.ServerSnapshot]                                    = protocol.ServerSnapshotSchema               // upstream: ServerSnapshotSchema
	_ protocol.SessionMetadata                                                                                                  // upstream: SessionMetadata
	_ protocol.Schema[protocol.SessionMetadata]                                   = protocol.SessionMetadataSchema              // upstream: SessionMetadataSchema
	_ protocol.SessionPhase                                                                                                     // upstream: SessionPhase
	_ protocol.Schema[protocol.SessionPhase]                                      = protocol.SessionPhaseSchema                 // upstream: SessionPhaseSchema
	_ protocol.SessionSnapshot                                                                                                  // upstream: SessionSnapshot
	_ protocol.Schema[protocol.SessionSnapshot]                                   = protocol.SessionSnapshotSchema              // upstream: SessionSnapshotSchema
	_ protocol.Schema[protocol.SetModelCommand]                                   = protocol.SetModelCommandSchema              // upstream: SetModelCommandSchema
	_ protocol.Schema[protocol.SetModelResult]                                    = protocol.SetModelResultSchema               // upstream: SetModelResultSchema
	_ protocol.Schema[protocol.SetThinkingCommand]                                = protocol.SetThinkingCommandSchema           // upstream: SetThinkingCommandSchema
	_ protocol.Schema[protocol.SetThinkingResult]                                 = protocol.SetThinkingResultSchema            // upstream: SetThinkingResultSchema
	_ protocol.Schema[protocol.SteerCommand]                                      = protocol.SteerCommandSchema                 // upstream: SteerCommandSchema
	_ protocol.Schema[protocol.SteerResult]                                       = protocol.SteerResultSchema                  // upstream: SteerResultSchema
	_ protocol.TextContent                                                                                                      // upstream: TextContent
	_ protocol.Schema[protocol.TextContent]                                       = protocol.TextContentSchema                  // upstream: TextContentSchema
	_ protocol.ThinkingContent                                                                                                  // upstream: ThinkingContent
	_ protocol.Schema[protocol.ThinkingContent]                                   = protocol.ThinkingContentSchema              // upstream: ThinkingContentSchema
	_ protocol.ThinkingLevel                                                                                                    // upstream: ThinkingLevel
	_ protocol.Schema[protocol.ThinkingLevel]                                     = protocol.ThinkingLevelSchema                // upstream: ThinkingLevelSchema
	_ protocol.ToolCallContent                                                                                                  // upstream: ToolCallContent
	_ protocol.Schema[protocol.ToolCallContent]                                   = protocol.ToolCallContentSchema              // upstream: ToolCallContentSchema
	_ protocol.Schema[protocol.ToolContent]                                       = protocol.ToolContentSchema                  // upstream: ToolContentSchema
	_ protocol.ToolTranscriptItem                                                 = protocol.RunningToolTranscriptItem{}        // upstream: ToolTranscriptItem
	_ protocol.Schema[protocol.ToolTranscriptItem]                                = protocol.ToolTranscriptItemSchema           // upstream: ToolTranscriptItemSchema
	_ protocol.TranscriptItem                                                     = protocol.UserTranscriptItem{}               // upstream: TranscriptItem
	_ protocol.Schema[protocol.TranscriptItem]                                    = protocol.TranscriptItemSchema               // upstream: TranscriptItemSchema
	_ protocol.TranscriptProgress                                                 = protocol.ItemStartedProgress{}              // upstream: TranscriptProgress
	_ protocol.Schema[protocol.TranscriptProgress]                                = protocol.TranscriptProgressSchema           // upstream: TranscriptProgressSchema
	_ protocol.Usage                                                                                                            // upstream: Usage
	_ protocol.Schema[protocol.Usage]                                             = protocol.UsageSchema                        // upstream: UsageSchema
	_ protocol.Schema[protocol.UserContent]                                       = protocol.UserContentSchema                  // upstream: UserContentSchema
	_ protocol.UserTranscriptItem                                                                                               // upstream: UserTranscriptItem
	_ protocol.Schema[protocol.UserTranscriptItem]                                = protocol.UserTranscriptItemSchema           // upstream: UserTranscriptItemSchema
)

// Compile-time command/result pairing parity for ResultForCommand. These
// assertions strengthen the single upstream surface mapping above without
// adding duplicate upstream-name markers.
var (
	_ protocol.ResultForCommand[protocol.ListCommand]        = protocol.ListResult{}
	_ protocol.ResultForCommand[protocol.CreateCommand]      = protocol.CreateResult{}
	_ protocol.ResultForCommand[protocol.AttachCommand]      = protocol.AttachResult{}
	_ protocol.ResultForCommand[protocol.DetachCommand]      = protocol.DetachResult{}
	_ protocol.ResultForCommand[protocol.PromptCommand]      = protocol.PromptResult{}
	_ protocol.ResultForCommand[protocol.SteerCommand]       = protocol.SteerResult{}
	_ protocol.ResultForCommand[protocol.AbortCommand]       = protocol.AbortResult{}
	_ protocol.ResultForCommand[protocol.SetModelCommand]    = protocol.SetModelResult{}
	_ protocol.ResultForCommand[protocol.SetThinkingCommand] = protocol.SetThinkingResult{}
)
