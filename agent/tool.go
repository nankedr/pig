package agent

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/nankedr/pig/ai"
)

// ToolExecutionMode controls a batch of Tool calls. Parallel mode still runs
// every preflight in Assistant source order before starting executions. If a
// batch contains any sequential Tool, the whole batch is sequential.
type ToolExecutionMode string

const (
	ToolExecutionSequential ToolExecutionMode = "sequential"
	ToolExecutionParallel   ToolExecutionMode = "parallel"
)

// QueueMode controls how many messages are removed at one queue drain point.
type QueueMode string

const (
	QueueAll        QueueMode = "all"
	QueueOneAtATime QueueMode = "one-at-a-time"
)

// AgentToolCall is the Tool call block emitted by an Assistant message.
type AgentToolCall = ai.ToolCall

// AgentToolResult is a final or partial Tool result. Optional fields retain
// the distinction between omission and explicit zero/empty values.
type AgentToolResult[TDetails any] struct {
	Content        []ai.ToolResultContent
	Details        TDetails
	Usage          ai.Optional[ai.Usage]
	AddedToolNames ai.Optional[[]string]
	Terminate      ai.Optional[bool]
}

// ErasedAgentToolResult is the heterogeneous runtime form of a Tool result.
type ErasedAgentToolResult = AgentToolResult[ai.JSONValue]

// AgentToolUpdateCallback publishes one partial Tool result. The Tool
// dispatcher admits callbacks only while Execute is active and treats all
// admitted updates as a barrier before the final result event.
type AgentToolUpdateCallback[TDetails any] func(AgentToolResult[TDetails])

// PrepareArgumentsFunc normalizes raw JSON values before authoritative schema
// validation. It must not perform typed decoding or replace schema validation.
type PrepareArgumentsFunc func(ai.JSONValue) (ai.JSONValue, error)

// AgentTool is the strongly typed Tool authoring contract. The raw JSON Schema
// in Tool.Parameters remains authoritative; TParameters is decoded only after
// the dispatcher has prepared, coerced, and validated the value.
type AgentTool[TParameters, TDetails any] struct {
	ai.Tool
	Label            string
	PrepareArguments PrepareArgumentsFunc
	// DecodeValidated is a total mapping from every value admitted by Parameters
	// to TParameters. It must not reject, normalize, or revalidate the value.
	// Tools with Execute must provide this mapping explicitly.
	DecodeValidated func(ai.JSONValue) TParameters
	Execute         func(context.Context, string, TParameters, AgentToolUpdateCallback[TDetails]) (AgentToolResult[TDetails], error)
	ExecutionMode   ToolExecutionMode
}

// ErasedAgentTool is the heterogeneous runtime registry representation. The
// author callbacks and validated execution seam stay package-private: callers
// create this value through EraseAgentTool, and only the Agent dispatcher can
// invoke it after the authoritative schema pipeline has succeeded.
type ErasedAgentTool struct {
	ai.Tool
	Label         string
	ExecutionMode ToolExecutionMode

	prepareArguments PrepareArgumentsFunc
	decodeValidated  func(ai.JSONValue) erasedValidatedAgentToolArguments
	executeValidated func(context.Context, string, ai.JSONValue, AgentToolUpdateCallback[ai.JSONValue]) (ErasedAgentToolResult, error)
	descriptor       *erasedAgentToolDescriptor
}

// EraseAgentTool validates metadata and erases a typed authoring descriptor for
// heterogeneous Agent state. A missing Execute function becomes an explicit,
// side-effect-free Capability Stub.
func EraseAgentTool[TParameters, TDetails any](tool AgentTool[TParameters, TDetails]) (ErasedAgentTool, error) {
	tool.Tool = cloneAgentToolMetadata(tool.Tool)
	if tool.Name == "" {
		return ErasedAgentTool{}, fmt.Errorf("Agent Tool name must not be empty")
	}
	if tool.ExecutionMode != "" && tool.ExecutionMode != ToolExecutionParallel && tool.ExecutionMode != ToolExecutionSequential {
		return ErasedAgentTool{}, fmt.Errorf("invalid Tool execution mode %q", tool.ExecutionMode)
	}
	if _, err := json.Marshal(tool.Tool); err != nil {
		return ErasedAgentTool{}, fmt.Errorf("invalid Agent Tool %q: %w", tool.Name, err)
	}
	if _, _, err := compileAgentToolSchema(tool.Name, tool.Parameters); err != nil {
		return ErasedAgentTool{}, err
	}
	if tool.Execute != nil && tool.DecodeValidated == nil {
		return ErasedAgentTool{}, fmt.Errorf("Agent Tool %q with Execute requires DecodeValidated", tool.Name)
	}

	prepare := tool.PrepareArguments
	if prepare == nil {
		prepare = func(value ai.JSONValue) (ai.JSONValue, error) { return value, nil }
	}
	descriptor := &erasedAgentToolDescriptor{authenticityID: nextErasedAgentToolAuthenticityID.Add(1)}
	erased := ErasedAgentTool{
		Tool: tool.Tool, Label: tool.Label, ExecutionMode: tool.ExecutionMode,
		prepareArguments: prepare, descriptor: descriptor,
	}
	if tool.Execute == nil {
		erased.executeValidated = func(context.Context, string, ai.JSONValue, AgentToolUpdateCallback[ai.JSONValue]) (ErasedAgentToolResult, error) {
			return ErasedAgentToolResult{}, newNotImplemented("AgentTool.Execute")
		}
		fingerprint, err := fingerprintErasedAgentTool(erased)
		if err != nil {
			return ErasedAgentTool{}, err
		}
		descriptor.fingerprint = fingerprint
		return erased, nil
	}

	erased.decodeValidated = func(value ai.JSONValue) erasedValidatedAgentToolArguments {
		parameters := tool.DecodeValidated(value)
		return &validatedAgentToolArgumentsValue[TParameters]{value: parameters}
	}
	erased.executeValidated = func(ctx context.Context, toolCallID string, value ai.JSONValue, onUpdate AgentToolUpdateCallback[ai.JSONValue]) (ErasedAgentToolResult, error) {
		parameters, err := unwrapValidatedArguments[TParameters](value, descriptor)
		if err != nil {
			return ErasedAgentToolResult{}, fmt.Errorf("decode validated arguments for Tool %q: %w", tool.Name, err)
		}
		var updateMu sync.Mutex
		updatesOpen := true
		var updateErr error
		result, err := tool.Execute(ctx, toolCallID, parameters, func(update AgentToolResult[TDetails]) {
			updateMu.Lock()
			defer updateMu.Unlock()
			if !updatesOpen || onUpdate == nil || updateErr != nil {
				return
			}
			erasedUpdate, err := eraseAgentToolResult(update)
			if err != nil {
				updateErr = fmt.Errorf("erase progress details for Tool %q: %w", tool.Name, err)
				return
			}
			onUpdate(erasedUpdate)
		})
		updateMu.Lock()
		updatesOpen = false
		progressErr := updateErr
		updateMu.Unlock()
		if err != nil {
			return ErasedAgentToolResult{}, err
		}
		if progressErr != nil {
			return ErasedAgentToolResult{}, progressErr
		}
		erasedResult, err := eraseAgentToolResult(result)
		if err != nil {
			return ErasedAgentToolResult{}, fmt.Errorf("erase final details for Tool %q: %w", tool.Name, err)
		}
		return erasedResult, nil
	}
	fingerprint, err := fingerprintErasedAgentTool(erased)
	if err != nil {
		return ErasedAgentTool{}, err
	}
	descriptor.fingerprint = fingerprint
	return erased, nil
}

func cloneAgentToolMetadata(tool ai.Tool) ai.Tool {
	tool.Parameters = append(json.RawMessage(nil), tool.Parameters...)
	switch constrained := tool.ConstrainedSampling.(type) {
	case *ai.ConstrainedSamplingDisabled:
		if constrained != nil {
			clone := *constrained
			tool.ConstrainedSampling = &clone
		}
	case *ai.JSONSchemaConstrainedSampling:
		if constrained != nil {
			clone := *constrained
			tool.ConstrainedSampling = &clone
		}
	case *ai.GrammarConstrainedSampling:
		if constrained != nil {
			clone := *constrained
			tool.ConstrainedSampling = &clone
		}
	}
	return tool
}

type erasedAgentToolDescriptor struct {
	authenticityID uint64
	fingerprint    [sha256.Size]byte
}

var nextErasedAgentToolAuthenticityID atomic.Uint64

type validatedAgentToolArguments struct {
	authenticityID uint64
	fingerprint    [sha256.Size]byte
	value          ai.JSONValue
	typed          erasedValidatedAgentToolArguments
}

type erasedValidatedAgentToolArguments interface {
	erasedValidatedAgentToolArguments()
}

type validatedAgentToolArgumentsValue[T any] struct {
	value T
}

func (*validatedAgentToolArgumentsValue[T]) erasedValidatedAgentToolArguments() {}

func validateErasedAgentTool(tool ErasedAgentTool) error {
	if tool.Name == "" {
		return fmt.Errorf("Agent Tool name must not be empty")
	}
	if tool.ExecutionMode != "" && tool.ExecutionMode != ToolExecutionParallel && tool.ExecutionMode != ToolExecutionSequential {
		return fmt.Errorf("invalid Tool execution mode %q", tool.ExecutionMode)
	}
	if _, err := json.Marshal(tool.Tool); err != nil {
		return fmt.Errorf("invalid Agent Tool %q: %w", tool.Name, err)
	}
	if tool.prepareArguments == nil {
		return fmt.Errorf("Agent Tool %q is missing PrepareArguments", tool.Name)
	}
	if tool.executeValidated == nil {
		return fmt.Errorf("Agent Tool %q is missing ExecuteValidated", tool.Name)
	}
	if tool.descriptor == nil {
		return fmt.Errorf("Agent Tool %q must come from EraseAgentTool", tool.Name)
	}
	fingerprint, err := fingerprintErasedAgentTool(tool)
	if err != nil {
		return err
	}
	if fingerprint != tool.descriptor.fingerprint {
		return fmt.Errorf("Agent Tool %q metadata does not match its erased descriptor", tool.Name)
	}
	return nil
}

func sealValidatedAgentToolArguments(tool ErasedAgentTool, value ai.JSONValue) (validatedAgentToolArguments, error) {
	if err := validateErasedAgentTool(tool); err != nil {
		return validatedAgentToolArguments{}, err
	}
	var typed erasedValidatedAgentToolArguments
	if tool.decodeValidated != nil {
		typed = tool.decodeValidated(value)
	}
	return validatedAgentToolArguments{
		authenticityID: tool.descriptor.authenticityID,
		fingerprint:    tool.descriptor.fingerprint,
		value:          value,
		typed:          typed,
	}, nil
}

func unwrapValidatedArguments[T any](value ai.JSONValue, descriptor *erasedAgentToolDescriptor) (T, error) {
	var zero T
	var sealed *validatedAgentToolArguments
	switch typed := value.(type) {
	case validatedAgentToolArguments:
		sealed = &typed
	case *validatedAgentToolArguments:
		sealed = typed
	default:
		return zero, fmt.Errorf("arguments must come from the validated Tool pipeline")
	}
	if sealed == nil || descriptor == nil || sealed.authenticityID != descriptor.authenticityID || sealed.fingerprint != descriptor.fingerprint {
		return zero, fmt.Errorf("arguments were not sealed for this Tool")
	}
	typed, ok := sealed.typed.(*validatedAgentToolArgumentsValue[T])
	if !ok {
		return zero, fmt.Errorf("validated arguments have the wrong type for this Tool")
	}
	return typed.value, nil
}

func fingerprintErasedAgentTool(tool ErasedAgentTool) ([sha256.Size]byte, error) {
	var empty [sha256.Size]byte
	rawTool, err := json.Marshal(tool.Tool)
	if err != nil {
		return empty, fmt.Errorf("invalid Agent Tool %q: %w", tool.Name, err)
	}
	payload, err := json.Marshal(struct {
		Tool          json.RawMessage   `json:"tool"`
		Label         string            `json:"label"`
		ExecutionMode ToolExecutionMode `json:"executionMode"`
	}{
		Tool: rawTool, Label: tool.Label, ExecutionMode: tool.ExecutionMode,
	})
	if err != nil {
		return empty, err
	}
	return sha256.Sum256(payload), nil
}

func eraseAgentToolResult[T any](result AgentToolResult[T]) (ErasedAgentToolResult, error) {
	details, err := toJSONValue(result.Details)
	if err != nil {
		return ErasedAgentToolResult{}, err
	}
	return ErasedAgentToolResult{
		Content:        append([]ai.ToolResultContent(nil), result.Content...),
		Details:        details,
		Usage:          result.Usage,
		AddedToolNames: cloneOptionalStrings(result.AddedToolNames),
		Terminate:      result.Terminate,
	}, nil
}

func toJSONValue(value any) (ai.JSONValue, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var erased ai.JSONValue
	if err := json.Unmarshal(raw, &erased); err != nil {
		return nil, err
	}
	return erased, nil
}

func cloneOptionalStrings(value ai.Optional[[]string]) ai.Optional[[]string] {
	if !value.IsSet() {
		return ai.Absent[[]string]()
	}
	if value.IsNull() {
		return ai.Null[[]string]()
	}
	strings, _ := value.Value()
	return ai.Some(append([]string(nil), strings...))
}
