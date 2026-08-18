package protocol

// Schema is an inert, typed descriptor for a Remote Session Protocol wire
// declaration. Validation and parsing are capability stubs until M9.
type Schema[T any] struct {
	name string
}

func schema[T any](name string) Schema[T] {
	return Schema[T]{name: name}
}

// Name returns the stable declaration name represented by the descriptor.
func (s Schema[T]) Name() string {
	return s.name
}

// Validate is a capability stub.
func (s Schema[T]) Validate(T) error {
	return &NotImplementedError{Module: "protocol", Operation: s.name + "Schema.Validate"}
}

// Parse is a capability stub.
func (s Schema[T]) Parse(any) (T, error) {
	var zero T
	return zero, &NotImplementedError{Module: "protocol", Operation: s.name + "Schema.Parse"}
}

// ModelRef identifies a model within a provider.
type ModelRef struct {
	Provider string `json:"provider"`
	ID       string `json:"id"`
}

var ModelRefSchema = schema[ModelRef]("ModelRef")

var (
	JSONValueSchema     = schema[JSONValue]("JSONValue")
	ThinkingLevelSchema = schema[ThinkingLevel]("ThinkingLevel")
	SessionPhaseSchema  = schema[SessionPhase]("SessionPhase")

	ModelCostSchema     = schema[ModelCost]("ModelCost")
	ModelMetadataSchema = schema[ModelMetadata]("ModelMetadata")

	TextContentSchema      = schema[TextContent]("TextContent")
	ThinkingContentSchema  = schema[ThinkingContent]("ThinkingContent")
	ImageContentSchema     = schema[ImageContent]("ImageContent")
	ToolCallContentSchema  = schema[ToolCallContent]("ToolCallContent")
	UserContentSchema      = schema[UserContent]("UserContent")
	AssistantContentSchema = schema[AssistantContent]("AssistantContent")
	ToolContentSchema      = schema[ToolContent]("ToolContent")
	UsageSchema            = schema[Usage]("Usage")

	UserTranscriptItemSchema      = schema[UserTranscriptItem]("UserTranscriptItem")
	AssistantTranscriptItemSchema = schema[AssistantTranscriptItem]("AssistantTranscriptItem")
	ToolTranscriptItemSchema      = schema[ToolTranscriptItem]("ToolTranscriptItem")
	TranscriptItemSchema          = schema[TranscriptItem]("TranscriptItem")
	TranscriptProgressSchema      = schema[TranscriptProgress]("TranscriptProgress")

	SessionMetadataSchema = schema[SessionMetadata]("SessionMetadata")
	SessionSnapshotSchema = schema[SessionSnapshot]("SessionSnapshot")
	ServerSnapshotSchema  = schema[ServerSnapshot]("ServerSnapshot")

	ProtocolErrorCodeSchema = schema[ProtocolErrorCode]("ProtocolErrorCode")
	ProtocolErrorSchema     = schema[ProtocolError]("ProtocolError")

	ListCommandSchema        = schema[ListCommand]("ListCommand")
	CreateCommandSchema      = schema[CreateCommand]("CreateCommand")
	AttachCommandSchema      = schema[AttachCommand]("AttachCommand")
	DetachCommandSchema      = schema[DetachCommand]("DetachCommand")
	PromptCommandSchema      = schema[PromptCommand]("PromptCommand")
	SteerCommandSchema       = schema[SteerCommand]("SteerCommand")
	AbortCommandSchema       = schema[AbortCommand]("AbortCommand")
	SetModelCommandSchema    = schema[SetModelCommand]("SetModelCommand")
	SetThinkingCommandSchema = schema[SetThinkingCommand]("SetThinkingCommand")
	CommandSchema            = schema[Command]("Command")

	CreateResultSchema      = schema[CreateResult]("CreateResult")
	AttachResultSchema      = schema[AttachResult]("AttachResult")
	PromptResultSchema      = schema[PromptResult]("PromptResult")
	SteerResultSchema       = schema[SteerResult]("SteerResult")
	AbortResultSchema       = schema[AbortResult]("AbortResult")
	SetModelResultSchema    = schema[SetModelResult]("SetModelResult")
	SetThinkingResultSchema = schema[SetThinkingResult]("SetThinkingResult")
	ListResultSchema        = schema[ListResult]("ListResult")
	DetachResultSchema      = schema[DetachResult]("DetachResult")
	CommandResultSchema     = schema[CommandResult]("CommandResult")

	ClientHelloSchema      = schema[ClientHello]("ClientHello")
	RequestEnvelopeSchema  = schema[RequestEnvelope]("RequestEnvelope")
	ClientMessageSchema    = schema[ClientMessage]("ClientMessage")
	ServerEventSchema      = schema[ServerEvent]("ServerEvent")
	ServerHelloSchema      = schema[ServerHello]("ServerHello")
	ServerHelloErrorSchema = schema[ServerHelloError]("ServerHelloError")
	ResponseEnvelopeSchema = schema[ResponseEnvelope]("ResponseEnvelope")
	EventEnvelopeSchema    = schema[EventEnvelope]("EventEnvelope")
	ServerMessageSchema    = schema[ServerMessage]("ServerMessage")
)
