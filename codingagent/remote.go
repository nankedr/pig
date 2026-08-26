package codingagent

import (
	"context"
	"encoding/json"
	"math"
	"reflect"
	"strconv"
	"strings"

	"github.com/nankedr/pig/client"
	"github.com/nankedr/pig/protocol"
)

// RemoteSessionOperation identifies an operation that may temporarily make a
// RemoteSession busy.
type RemoteSessionOperation string

const (
	RemoteSessionOperationOpen        RemoteSessionOperation = "open"
	RemoteSessionOperationCreate      RemoteSessionOperation = "create"
	RemoteSessionOperationSubmit      RemoteSessionOperation = "submit"
	RemoteSessionOperationAbort       RemoteSessionOperation = "abort"
	RemoteSessionOperationSetModel    RemoteSessionOperation = "setModel"
	RemoteSessionOperationSetThinking RemoteSessionOperation = "setThinking"
	RemoteSessionOperationReconnect   RemoteSessionOperation = "reconnect"
)

// RemoteSessionLifecycleStatus identifies the local lifecycle state of a
// RemoteSession. Operation is populated only while Status is busy.
type RemoteSessionLifecycleStatus string

const (
	RemoteSessionLifecycleStatusUnbound  RemoteSessionLifecycleStatus = "unbound"
	RemoteSessionLifecycleStatusReady    RemoteSessionLifecycleStatus = "ready"
	RemoteSessionLifecycleStatusBusy     RemoteSessionLifecycleStatus = "busy"
	RemoteSessionLifecycleStatusDisposed RemoteSessionLifecycleStatus = "disposed"
)

// RemoteSessionLifecycle is the Go carrier for the upstream lifecycle union.
type RemoteSessionLifecycle struct {
	Status RemoteSessionLifecycleStatus
	// Operation is present only when Status is busy.
	Operation *RemoteSessionOperation
}

// RemoteSessionState is one projected view of a remote session.
type RemoteSessionState struct {
	Lifecycle  RemoteSessionLifecycle
	Snapshot   *protocol.SessionSnapshot
	Transcript []protocol.TranscriptItem
}

// CreateRemoteSessionOptions configures remote session creation. Optional
// fields preserve absence independently from their Go zero values.
type CreateRemoteSessionOptions struct {
	CWD           string
	Model         protocol.Optional[protocol.ModelRef]
	ThinkingLevel protocol.Optional[protocol.ThinkingLevel]
}

// RemoteSessionOptions configures listener-error reporting.
type RemoteSessionOptions struct {
	OnListenerError client.ListenerErrorHandler
}

// RemoteSession maps the high-level remote-session facade. Transport, request
// correlation, and lease state remain owned by client. Until that lower-layer
// capability is implemented, RemoteSession contains no live remote state.
type RemoteSession struct{}

// OpenRemoteSession maps RemoteSession.open without invoking client or its
// transport factory.
func OpenRemoteSession(context.Context, *client.Client, string, ...RemoteSessionOptions) (*RemoteSession, error) {
	return nil, notImplemented("OpenRemoteSession")
}

// CreateRemoteSession maps RemoteSession.create without invoking client or its
// transport factory.
func CreateRemoteSession(context.Context, *client.Client, CreateRemoteSessionOptions, ...RemoteSessionOptions) (*RemoteSession, error) {
	return nil, notImplemented("CreateRemoteSession")
}

// The getters below require authoritative state owned by a constructed remote
// session. The scaffold has no such state, so it must not present zero values as
// successful observations.
func (*RemoteSession) ID() (*string, error) {
	return nil, notImplemented("RemoteSession.ID")
}

func (*RemoteSession) State() (RemoteSessionState, error) {
	return RemoteSessionState{}, notImplemented("RemoteSession.State")
}

func (*RemoteSession) Snapshot() (*protocol.SessionSnapshot, error) {
	return nil, notImplemented("RemoteSession.Snapshot")
}

func (*RemoteSession) Phase() (*protocol.SessionPhase, error) {
	return nil, notImplemented("RemoteSession.Phase")
}

func (*RemoteSession) Operation() (*RemoteSessionOperation, error) {
	return nil, notImplemented("RemoteSession.Operation")
}

func (*RemoteSession) Models() ([]protocol.ModelMetadata, error) {
	return nil, notImplemented("RemoteSession.Models")
}

func (*RemoteSession) Sessions() ([]protocol.SessionMetadata, error) {
	return nil, notImplemented("RemoteSession.Sessions")
}

func (*RemoteSession) ConnectionState() (client.ConnectionState, error) {
	return "", notImplemented("RemoteSession.ConnectionState")
}

func (*RemoteSession) Disposed() (bool, error) {
	return false, notImplemented("RemoteSession.Disposed")
}

// Subscribe is a capability stub and never registers or invokes listener.
func (*RemoteSession) Subscribe(func(RemoteSessionState)) (client.Unsubscribe, error) {
	return nil, notImplemented("RemoteSession.Subscribe")
}

// OnConnectionStateChange is a capability stub and never delegates listener
// registration to client.
func (*RemoteSession) OnConnectionStateChange(func(client.ConnectionStateChange)) (client.Unsubscribe, error) {
	return nil, notImplemented("RemoteSession.OnConnectionStateChange")
}

// Open is an inert remote-operation capability stub.
func (*RemoteSession) Open(context.Context, string) error {
	return notImplemented("RemoteSession.Open")
}

// Create is an inert remote-operation capability stub.
func (*RemoteSession) Create(context.Context, CreateRemoteSessionOptions) error {
	return notImplemented("RemoteSession.Create")
}

// Submit is an inert remote-operation capability stub.
func (*RemoteSession) Submit(context.Context, string) error {
	return notImplemented("RemoteSession.Submit")
}

// Abort is an inert remote-operation capability stub.
func (*RemoteSession) Abort(context.Context) error {
	return notImplemented("RemoteSession.Abort")
}

// SetModel is an inert remote-operation capability stub.
func (*RemoteSession) SetModel(context.Context, protocol.ModelRef) error {
	return notImplemented("RemoteSession.SetModel")
}

// SetThinking is an inert remote-operation capability stub.
func (*RemoteSession) SetThinking(context.Context, protocol.ThinkingLevel) error {
	return notImplemented("RemoteSession.SetThinking")
}

// Reconnect is an inert remote-operation capability stub.
func (*RemoteSession) Reconnect(context.Context) error {
	return notImplemented("RemoteSession.Reconnect")
}

// Dispose is an inert remote-operation capability stub.
func (*RemoteSession) Dispose(context.Context) error {
	return notImplemented("RemoteSession.Dispose")
}

// TranscriptState holds the authoritative snapshot plus transient progress.
// The maps and slices are owned by the value and replaced on every projection.
type TranscriptState struct {
	Snapshot        protocol.SessionSnapshot
	ProgressItems   map[string]protocol.TranscriptItem
	ProgressOrder   []string
	ToolCallBuffers map[string]string
}

// CreateTranscriptState starts a transcript projection from an authoritative
// snapshot and defensively clones all slice- and JSON-backed data.
func CreateTranscriptState(snapshot protocol.SessionSnapshot) TranscriptState {
	return TranscriptState{
		Snapshot:        cloneSessionSnapshot(snapshot),
		ProgressItems:   make(map[string]protocol.TranscriptItem),
		ProgressOrder:   []string{},
		ToolCallBuffers: make(map[string]string),
	}
}

// ApplyTranscriptSnapshot replaces progress with an authoritative snapshot. A
// lower revision is stale only when it belongs to the same session runtime.
func ApplyTranscriptSnapshot(state TranscriptState, snapshot protocol.SessionSnapshot) TranscriptState {
	if state.Snapshot.ID == snapshot.ID && snapshot.Revision < state.Snapshot.Revision {
		return cloneTranscriptState(state)
	}
	return CreateTranscriptState(snapshot)
}

// ApplyTranscriptProgress projects one progress event without mutating state.
func ApplyTranscriptProgress(state TranscriptState, progress protocol.TranscriptProgress) TranscriptState {
	next := cloneTranscriptState(state)
	if progress == nil {
		return next
	}

	switch event := progress.(type) {
	case protocol.ItemStartedProgress:
		return setProgressItem(next, event.Item)
	case *protocol.ItemStartedProgress:
		if event == nil {
			return next
		}
		return setProgressItem(next, event.Item)
	case protocol.ItemUpdatedProgress:
		return setProgressItem(next, event.Item)
	case *protocol.ItemUpdatedProgress:
		if event == nil {
			return next
		}
		return setProgressItem(next, event.Item)
	case protocol.ItemFinishedProgress:
		return finishProgressItem(next, event.Item)
	case *protocol.ItemFinishedProgress:
		if event == nil {
			return next
		}
		return finishProgressItem(next, event.Item)
	case protocol.AssistantDeltaProgress:
		return applyAssistantDelta(next, event)
	case *protocol.AssistantDeltaProgress:
		if event == nil {
			return next
		}
		return applyAssistantDelta(next, *event)
	default:
		return next
	}
}

func finishProgressItem(state TranscriptState, item protocol.TranscriptItem) TranscriptState {
	id := transcriptItemID(item)
	for key := range state.ToolCallBuffers {
		if hasToolCallBufferPrefix(key, id) {
			delete(state.ToolCallBuffers, key)
		}
	}
	return setProgressItem(state, item)
}

// SelectTranscript overlays progress on the authoritative transcript, then
// appends transient items and accepted queued steering messages by stable ID.
// Its result is a defensive copy.
func SelectTranscript(state TranscriptState) []protocol.TranscriptItem {
	transcript := make([]protocol.TranscriptItem, 0, len(state.Snapshot.Transcript)+len(state.ProgressOrder)+len(state.Snapshot.QueuedSteer))
	ids := make(map[string]struct{}, cap(transcript))
	for _, item := range state.Snapshot.Transcript {
		id := transcriptItemID(item)
		if progress, ok := state.ProgressItems[id]; ok {
			item = progress
		}
		transcript = append(transcript, cloneTranscriptItem(item))
		ids[id] = struct{}{}
	}
	for _, id := range state.ProgressOrder {
		if _, exists := ids[id]; exists {
			continue
		}
		if item, ok := state.ProgressItems[id]; ok {
			transcript = append(transcript, cloneTranscriptItem(item))
			ids[id] = struct{}{}
		}
	}
	for _, item := range state.Snapshot.QueuedSteer {
		if _, exists := ids[item.ID]; exists {
			continue
		}
		transcript = append(transcript, cloneTranscriptItem(item))
		ids[item.ID] = struct{}{}
	}
	return transcript
}

func applyAssistantDelta(state TranscriptState, progress protocol.AssistantDeltaProgress) TranscriptState {
	item, ok := state.ProgressItems[progress.MessageID]
	if !ok {
		for _, candidate := range state.Snapshot.Transcript {
			if transcriptItemID(candidate) == progress.MessageID {
				item, ok = candidate, true
				break
			}
		}
	}
	if !ok {
		return state
	}

	switch value := item.(type) {
	case protocol.StreamingAssistantTranscriptItem:
		value.Content = applyAssistantContentDelta(state, progress, value.Content)
		return setProgressItem(state, value)
	case *protocol.StreamingAssistantTranscriptItem:
		if value == nil {
			return state
		}
		clone := *value
		clone.Content = applyAssistantContentDelta(state, progress, value.Content)
		return setProgressItem(state, clone)
	case protocol.CompleteAssistantTranscriptItem:
		value.Content = applyAssistantContentDelta(state, progress, value.Content)
		return setProgressItem(state, value)
	case *protocol.CompleteAssistantTranscriptItem:
		if value == nil {
			return state
		}
		clone := *value
		clone.Content = applyAssistantContentDelta(state, progress, value.Content)
		return setProgressItem(state, clone)
	case protocol.ErrorAssistantTranscriptItem:
		value.Content = applyAssistantContentDelta(state, progress, value.Content)
		return setProgressItem(state, value)
	case *protocol.ErrorAssistantTranscriptItem:
		if value == nil {
			return state
		}
		clone := *value
		clone.Content = applyAssistantContentDelta(state, progress, value.Content)
		return setProgressItem(state, clone)
	case protocol.AbortedAssistantTranscriptItem:
		value.Content = applyAssistantContentDelta(state, progress, value.Content)
		return setProgressItem(state, value)
	case *protocol.AbortedAssistantTranscriptItem:
		if value == nil {
			return state
		}
		clone := *value
		clone.Content = applyAssistantContentDelta(state, progress, value.Content)
		return setProgressItem(state, clone)
	default:
		return state
	}
}

func applyAssistantContentDelta(state TranscriptState, progress protocol.AssistantDeltaProgress, content []protocol.AssistantContent) []protocol.AssistantContent {
	out := cloneAssistantContent(content)
	for index, item := range out {
		if int64(index) != progress.ContentIndex {
			continue
		}
		switch part := item.(type) {
		case protocol.TextContent:
			if progress.Kind == protocol.AssistantDeltaKindText {
				part.Text += progress.Delta
				out[index] = part
			}
		case protocol.ThinkingContent:
			if progress.Kind == protocol.AssistantDeltaKindThinking {
				part.Thinking += progress.Delta
				out[index] = part
			}
		case protocol.ToolCallContent:
			if progress.Kind == protocol.AssistantDeltaKindToolCall {
				key := toolCallBufferKey(progress.MessageID, progress.ContentIndex)
				existing, buffered := state.ToolCallBuffers[key]
				if !buffered {
					existing, _ = part.Input.(string)
				}
				buffer := existing + progress.Delta
				state.ToolCallBuffers[key] = buffer
				part.Input = parsePartialToolInput(buffer)
				out[index] = part
			}
		case *protocol.TextContent:
			if part != nil && progress.Kind == protocol.AssistantDeltaKindText {
				clone := *part
				clone.Text += progress.Delta
				out[index] = &clone
			}
		case *protocol.ThinkingContent:
			if part != nil && progress.Kind == protocol.AssistantDeltaKindThinking {
				clone := *part
				clone.Thinking += progress.Delta
				out[index] = &clone
			}
		case *protocol.ToolCallContent:
			if part != nil && progress.Kind == protocol.AssistantDeltaKindToolCall {
				key := toolCallBufferKey(progress.MessageID, progress.ContentIndex)
				existing, buffered := state.ToolCallBuffers[key]
				if !buffered {
					existing, _ = part.Input.(string)
				}
				buffer := existing + progress.Delta
				state.ToolCallBuffers[key] = buffer
				clone := *part
				clone.Input = parsePartialToolInput(buffer)
				out[index] = &clone
			}
		}
		break
	}
	return out
}

func parsePartialToolInput(input string) protocol.JSONValue {
	var value any
	if err := json.Unmarshal([]byte(input), &value); err == nil && isJSONValue(value) {
		return value
	}
	return input
}

func isJSONValue(value any) bool {
	switch value := value.(type) {
	case nil, bool, string:
		return true
	case float64:
		return !math.IsNaN(value) && !math.IsInf(value, 0)
	case []any:
		for _, element := range value {
			if !isJSONValue(element) {
				return false
			}
		}
		return true
	case map[string]any:
		for _, element := range value {
			if !isJSONValue(element) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func setProgressItem(state TranscriptState, item protocol.TranscriptItem) TranscriptState {
	if item == nil {
		return state
	}
	id := transcriptItemID(item)
	if _, exists := state.ProgressItems[id]; !exists {
		state.ProgressOrder = append(state.ProgressOrder, id)
	}
	state.ProgressItems[id] = cloneTranscriptItem(item)
	return state
}

func cloneTranscriptState(state TranscriptState) TranscriptState {
	progressItems := make(map[string]protocol.TranscriptItem, len(state.ProgressItems))
	for id, item := range state.ProgressItems {
		progressItems[id] = cloneTranscriptItem(item)
	}
	toolCallBuffers := make(map[string]string, len(state.ToolCallBuffers))
	for key, value := range state.ToolCallBuffers {
		toolCallBuffers[key] = value
	}
	return TranscriptState{
		Snapshot:        cloneSessionSnapshot(state.Snapshot),
		ProgressItems:   progressItems,
		ProgressOrder:   append([]string(nil), state.ProgressOrder...),
		ToolCallBuffers: toolCallBuffers,
	}
}

func cloneSessionSnapshot(snapshot protocol.SessionSnapshot) protocol.SessionSnapshot {
	snapshot.Transcript = cloneTranscriptItems(snapshot.Transcript)
	if snapshot.QueuedSteer == nil {
		return snapshot
	}
	snapshot.QueuedSteer = append([]protocol.UserTranscriptItem(nil), snapshot.QueuedSteer...)
	for index := range snapshot.QueuedSteer {
		snapshot.QueuedSteer[index].Content = cloneUserContent(snapshot.QueuedSteer[index].Content)
	}
	return snapshot
}

func cloneTranscriptItems(items []protocol.TranscriptItem) []protocol.TranscriptItem {
	if items == nil {
		return nil
	}
	out := make([]protocol.TranscriptItem, len(items))
	for index, item := range items {
		out[index] = cloneTranscriptItem(item)
	}
	return out
}

func cloneTranscriptItem(item protocol.TranscriptItem) protocol.TranscriptItem {
	switch value := item.(type) {
	case protocol.UserTranscriptItem:
		value.Content = cloneUserContent(value.Content)
		return value
	case *protocol.UserTranscriptItem:
		if value == nil {
			return item
		}
		clone := *value
		clone.Content = cloneUserContent(value.Content)
		return &clone
	case protocol.StreamingAssistantTranscriptItem:
		value.Content = cloneAssistantContent(value.Content)
		return value
	case *protocol.StreamingAssistantTranscriptItem:
		if value == nil {
			return item
		}
		clone := *value
		clone.Content = cloneAssistantContent(value.Content)
		return &clone
	case protocol.CompleteAssistantTranscriptItem:
		value.Content = cloneAssistantContent(value.Content)
		return value
	case *protocol.CompleteAssistantTranscriptItem:
		if value == nil {
			return item
		}
		clone := *value
		clone.Content = cloneAssistantContent(value.Content)
		return &clone
	case protocol.ErrorAssistantTranscriptItem:
		value.Content = cloneAssistantContent(value.Content)
		return value
	case *protocol.ErrorAssistantTranscriptItem:
		if value == nil {
			return item
		}
		clone := *value
		clone.Content = cloneAssistantContent(value.Content)
		return &clone
	case protocol.AbortedAssistantTranscriptItem:
		value.Content = cloneAssistantContent(value.Content)
		return value
	case *protocol.AbortedAssistantTranscriptItem:
		if value == nil {
			return item
		}
		clone := *value
		clone.Content = cloneAssistantContent(value.Content)
		return &clone
	case protocol.RunningToolTranscriptItem:
		value.Input = cloneJSONValue(value.Input)
		value.Content = cloneToolContent(value.Content)
		value.Details = cloneOptionalJSONValue(value.Details)
		return value
	case *protocol.RunningToolTranscriptItem:
		if value == nil {
			return item
		}
		clone := *value
		clone.Input = cloneJSONValue(value.Input)
		clone.Content = cloneToolContent(value.Content)
		clone.Details = cloneOptionalJSONValue(value.Details)
		return &clone
	case protocol.CompleteToolTranscriptItem:
		value.Input = cloneJSONValue(value.Input)
		value.Content = cloneToolContent(value.Content)
		value.Details = cloneOptionalJSONValue(value.Details)
		return value
	case *protocol.CompleteToolTranscriptItem:
		if value == nil {
			return item
		}
		clone := *value
		clone.Input = cloneJSONValue(value.Input)
		clone.Content = cloneToolContent(value.Content)
		clone.Details = cloneOptionalJSONValue(value.Details)
		return &clone
	case protocol.ErrorToolTranscriptItem:
		value.Input = cloneJSONValue(value.Input)
		value.Content = cloneToolContent(value.Content)
		value.Details = cloneOptionalJSONValue(value.Details)
		return value
	case *protocol.ErrorToolTranscriptItem:
		if value == nil {
			return item
		}
		clone := *value
		clone.Input = cloneJSONValue(value.Input)
		clone.Content = cloneToolContent(value.Content)
		clone.Details = cloneOptionalJSONValue(value.Details)
		return &clone
	default:
		return item
	}
}

func cloneUserContent(content []protocol.UserContent) []protocol.UserContent {
	if content == nil {
		return nil
	}
	out := make([]protocol.UserContent, len(content))
	for index, item := range content {
		switch value := item.(type) {
		case *protocol.TextContent:
			if value != nil {
				clone := *value
				item = &clone
			}
		case *protocol.ImageContent:
			if value != nil {
				clone := *value
				item = &clone
			}
		}
		out[index] = item
	}
	return out
}

func cloneAssistantContent(content []protocol.AssistantContent) []protocol.AssistantContent {
	if content == nil {
		return nil
	}
	out := make([]protocol.AssistantContent, len(content))
	for index, item := range content {
		switch value := item.(type) {
		case protocol.ToolCallContent:
			value.Input = cloneJSONValue(value.Input)
			item = value
		case *protocol.TextContent:
			if value != nil {
				clone := *value
				item = &clone
			}
		case *protocol.ThinkingContent:
			if value != nil {
				clone := *value
				item = &clone
			}
		case *protocol.ToolCallContent:
			if value != nil {
				clone := *value
				clone.Input = cloneJSONValue(value.Input)
				item = &clone
			}
		}
		out[index] = item
	}
	return out
}

func cloneToolContent(content []protocol.ToolContent) []protocol.ToolContent {
	if content == nil {
		return nil
	}
	out := make([]protocol.ToolContent, len(content))
	for index, item := range content {
		switch value := item.(type) {
		case *protocol.TextContent:
			if value != nil {
				clone := *value
				item = &clone
			}
		case *protocol.ImageContent:
			if value != nil {
				clone := *value
				item = &clone
			}
		}
		out[index] = item
	}
	return out
}

func cloneOptionalJSONValue(value protocol.Optional[protocol.JSONValue]) protocol.Optional[protocol.JSONValue] {
	if !value.Present {
		return value
	}
	value.Value = cloneJSONValue(value.Value)
	return value
}

func cloneJSONValue(value protocol.JSONValue) protocol.JSONValue {
	cloned := cloneJSONReflect(reflect.ValueOf(value))
	if !cloned.IsValid() {
		return nil
	}
	return cloned.Interface()
}

// cloneJSONReflect preserves named JSON-compatible containers while breaking
// every mutable map, slice, pointer, and array reference. JSON values cannot
// contain cycles, so cycle detection is intentionally unnecessary here.
func cloneJSONReflect(value reflect.Value) reflect.Value {
	if !value.IsValid() {
		return value
	}

	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		cloned := cloneJSONReflect(value.Elem())
		out := reflect.New(value.Type()).Elem()
		out.Set(cloned)
		return out
	case reflect.Map:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		out := reflect.MakeMapWithSize(value.Type(), value.Len())
		iterator := value.MapRange()
		for iterator.Next() {
			out.SetMapIndex(iterator.Key(), cloneJSONReflect(iterator.Value()))
		}
		return out
	case reflect.Slice:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		out := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		for index := 0; index < value.Len(); index++ {
			out.Index(index).Set(cloneJSONReflect(value.Index(index)))
		}
		return out
	case reflect.Array:
		out := reflect.New(value.Type()).Elem()
		for index := 0; index < value.Len(); index++ {
			out.Index(index).Set(cloneJSONReflect(value.Index(index)))
		}
		return out
	case reflect.Pointer:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		out := reflect.New(value.Type().Elem())
		out.Elem().Set(cloneJSONReflect(value.Elem()))
		return out
	default:
		return value
	}
}

func transcriptItemID(item protocol.TranscriptItem) string {
	switch value := item.(type) {
	case protocol.UserTranscriptItem:
		return value.ID
	case *protocol.UserTranscriptItem:
		if value != nil {
			return value.ID
		}
	case protocol.StreamingAssistantTranscriptItem:
		return value.ID
	case *protocol.StreamingAssistantTranscriptItem:
		if value != nil {
			return value.ID
		}
	case protocol.CompleteAssistantTranscriptItem:
		return value.ID
	case *protocol.CompleteAssistantTranscriptItem:
		if value != nil {
			return value.ID
		}
	case protocol.ErrorAssistantTranscriptItem:
		return value.ID
	case *protocol.ErrorAssistantTranscriptItem:
		if value != nil {
			return value.ID
		}
	case protocol.AbortedAssistantTranscriptItem:
		return value.ID
	case *protocol.AbortedAssistantTranscriptItem:
		if value != nil {
			return value.ID
		}
	case protocol.RunningToolTranscriptItem:
		return value.ID
	case *protocol.RunningToolTranscriptItem:
		if value != nil {
			return value.ID
		}
	case protocol.CompleteToolTranscriptItem:
		return value.ID
	case *protocol.CompleteToolTranscriptItem:
		if value != nil {
			return value.ID
		}
	case protocol.ErrorToolTranscriptItem:
		return value.ID
	case *protocol.ErrorToolTranscriptItem:
		if value != nil {
			return value.ID
		}
	}
	return ""
}

func toolCallBufferKey(messageID string, contentIndex int64) string {
	return messageID + ":" + formatDecimal(contentIndex)
}

func hasToolCallBufferPrefix(key, itemID string) bool {
	prefix := itemID + ":"
	return strings.HasPrefix(key, prefix)
}

func formatDecimal(value int64) string {
	return strconv.FormatInt(value, 10)
}
