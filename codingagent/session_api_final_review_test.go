package codingagent_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/codingagent"
)

var (
	_ []codingagent.ToolDefinition                                                      = codingagent.AgentSessionConfig{}.CustomTools
	_ map[string]agent.ErasedAgentTool                                                  = codingagent.AgentSessionConfig{}.BaseToolsOverride
	_ *codingagent.ExtensionRunner                                                      = codingagent.AgentSessionConfig{}.ExtensionRunnerRef
	_ func(bool)                                                                        = codingagent.PromptOptions{}.PreflightResult
	_ func(*codingagent.AgentSession) bool                                              = (*codingagent.AgentSession).IsIdle
	_ func(*codingagent.AgentSession) bool                                              = (*codingagent.AgentSession).IsStreaming
	_ func(*codingagent.AgentSession) (bool, error)                                     = (*codingagent.AgentSession).IsCompacting
	_ func(*codingagent.AgentSession) (bool, error)                                     = (*codingagent.AgentSession).IsRetrying
	_ func(*codingagent.AgentSession) (bool, error)                                     = (*codingagent.AgentSession).IsBashRunning
	_ func(*codingagent.AgentSession) (bool, error)                                     = (*codingagent.AgentSession).HasPendingBashMessages
	_ func(*codingagent.AgentSession) (int, error)                                      = (*codingagent.AgentSession).PendingMessageCount
	_ func(*codingagent.AgentSession) ([]string, error)                                 = (*codingagent.AgentSession).GetSteeringMessages
	_ func(*codingagent.AgentSession) ([]string, error)                                 = (*codingagent.AgentSession).GetFollowUpMessages
	_ func(*codingagent.AgentSession) ([]agent.ThinkingLevel, error)                    = (*codingagent.AgentSession).GetAvailableThinkingLevels
	_ func(*codingagent.AgentSession) (*codingagent.ContextUsage, error)                = (*codingagent.AgentSession).GetContextUsage
	_ func(*codingagent.AgentSession) (*string, error)                                  = (*codingagent.AgentSession).GetLastAssistantText
	_ func(*codingagent.AgentSession) (codingagent.SessionStats, error)                 = (*codingagent.AgentSession).GetSessionStats
	_ func(*codingagent.AgentSession, string) (codingagent.ToolDefinition, bool, error) = (*codingagent.AgentSession).GetToolDefinition
	_ func(*codingagent.AgentSession) ([]codingagent.ForkMessage, error)                = (*codingagent.AgentSession).GetUserMessagesForForking
	_ func(*codingagent.AgentSession, string) (bool, error)                             = (*codingagent.AgentSession).HasExtensionHandlers
	_ func(*codingagent.AgentSession) ([]codingagent.PromptTemplate, error)             = (*codingagent.AgentSession).PromptTemplates
	_ func(*codingagent.AgentSession) (bool, error)                                     = (*codingagent.AgentSession).SupportsThinking
	_ func(*codingagent.AgentSession) *codingagent.ExtensionRunner                      = (*codingagent.AgentSession).ExtensionRunner
	_ func(*codingagent.AgentSession) (codingagent.ExtensionCommandContext, error)      = (*codingagent.AgentSession).CreateReplacedSessionContext
)

func TestAgentSessionFinalAPICarriers(t *testing.T) {
	configType := reflect.TypeOf(codingagent.AgentSessionConfig{})
	wantFields := map[string]reflect.Type{
		"CustomTools":        reflect.TypeOf([]codingagent.ToolDefinition(nil)),
		"BaseToolsOverride":  reflect.TypeOf(map[string]agent.ErasedAgentTool(nil)),
		"ExtensionRunnerRef": reflect.TypeOf((*codingagent.ExtensionRunner)(nil)),
	}
	for name, want := range wantFields {
		field, ok := configType.FieldByName(name)
		if !ok {
			t.Fatalf("AgentSessionConfig.%s is missing", name)
		}
		if field.Type != want {
			t.Errorf("AgentSessionConfig.%s type = %s, want %s", name, field.Type, want)
		}
	}

	contextUsage, ok := reflect.TypeOf(codingagent.SessionStats{}).FieldByName("ContextUsage")
	wantContextUsage := reflect.TypeOf((*codingagent.ContextUsage)(nil))
	if !ok || contextUsage.Type != wantContextUsage {
		t.Fatalf("SessionStats.ContextUsage = %#v, want %s", contextUsage, wantContextUsage)
	}
}

func TestNewAgentSessionRetainsExtensionRunnerRef(t *testing.T) {
	runner := &codingagent.ExtensionRunner{}
	session := codingagent.NewAgentSession(codingagent.AgentSessionConfig{ExtensionRunnerRef: runner})

	if got := session.ExtensionRunner(); got != runner {
		t.Fatalf("ExtensionRunner() = %p, want config ref %p", got, runner)
	}
	if got := codingagent.NewAgentSession(codingagent.AgentSessionConfig{}).ExtensionRunner(); got != nil {
		t.Fatalf("zero ExtensionRunnerRef produced %p, want nil", got)
	}
}

func TestCreateReplacedSessionContextReturnsStructuredStub(t *testing.T) {
	got, err := codingagent.NewAgentSession(codingagent.AgentSessionConfig{}).CreateReplacedSessionContext()
	if !reflect.DeepEqual(got, codingagent.ExtensionCommandContext{}) {
		t.Fatalf("CreateReplacedSessionContext() = %#v, want zero ExtensionCommandContext", got)
	}
	if !errors.Is(err, codingagent.ErrNotImplemented) {
		t.Fatalf("error = %v, want ErrNotImplemented", err)
	}
	var target *codingagent.NotImplementedError
	if !errors.As(err, &target) || target.Operation != "AgentSession.CreateReplacedSessionContext" {
		t.Fatalf("error = %#v, want AgentSession.CreateReplacedSessionContext stub", target)
	}
}

func TestAgentSessionRuntimeCreatesInitialSessionOnlyWhenRequested(t *testing.T) {
	factoryCalls := 0
	session := codingagent.NewAgentSession(codingagent.AgentSessionConfig{})
	factory := func(context.Context, codingagent.CreateAgentSessionRuntimeOptions) (codingagent.CreateAgentSessionRuntimeResult, error) {
		factoryCalls++
		return codingagent.CreateAgentSessionRuntimeResult{CreateAgentSessionResult: codingagent.CreateAgentSessionResult{Session: session}}, nil
	}
	runtime := codingagent.NewAgentSessionRuntime(nil, codingagent.AgentSessionServices{}, factory, nil, nil)

	rebindCalls := 0
	invalidateCalls := 0
	runtime.SetRebindSession(func(*codingagent.AgentSession) error {
		rebindCalls++
		return nil
	})
	runtime.SetBeforeSessionInvalidate(func() { invalidateCalls++ })
	if factoryCalls != 0 || rebindCalls != 0 || invalidateCalls != 0 {
		t.Fatalf("constructor/setters invoked callbacks: factory=%d rebind=%d invalidate=%d", factoryCalls, rebindCalls, invalidateCalls)
	}

	created, err := codingagent.CreateAgentSessionRuntime(context.Background(), factory, codingagent.CreateAgentSessionRuntimeOptions{})
	if err != nil || created == nil || created.Session() != session {
		t.Fatalf("CreateAgentSessionRuntime() = (%v, %v), want runtime with factory Session", created, err)
	}
	if factoryCalls != 1 {
		t.Fatalf("CreateAgentSessionRuntime called factory %d times, want one", factoryCalls)
	}
	if err := created.Dispose(context.Background()); err != nil {
		t.Fatalf("Dispose() error = %v", err)
	}
}

func TestAgentSessionUnavailableQueriesReturnStructuredErrors(t *testing.T) {
	session := codingagent.NewAgentSession(codingagent.AgentSessionConfig{})
	tests := []struct {
		name      string
		operation string
		call      func() error
	}{
		{name: "is compacting", operation: "AgentSession.IsCompacting", call: func() error {
			value, err := session.IsCompacting()
			if value {
				t.Error("IsCompacting returned true with an error")
			}
			return err
		}},
		{name: "is retrying", operation: "AgentSession.IsRetrying", call: func() error {
			value, err := session.IsRetrying()
			if value {
				t.Error("IsRetrying returned true with an error")
			}
			return err
		}},
		{name: "is bash running", operation: "AgentSession.IsBashRunning", call: func() error {
			value, err := session.IsBashRunning()
			if value {
				t.Error("IsBashRunning returned true with an error")
			}
			return err
		}},
		{name: "has pending bash messages", operation: "AgentSession.HasPendingBashMessages", call: func() error {
			value, err := session.HasPendingBashMessages()
			if value {
				t.Error("HasPendingBashMessages returned true with an error")
			}
			return err
		}},
		{name: "pending message count", operation: "AgentSession.PendingMessageCount", call: func() error {
			value, err := session.PendingMessageCount()
			if value != 0 {
				t.Errorf("PendingMessageCount = %d, want zero with an error", value)
			}
			return err
		}},
		{name: "steering messages", operation: "AgentSession.GetSteeringMessages", call: func() error {
			value, err := session.GetSteeringMessages()
			if value != nil {
				t.Errorf("GetSteeringMessages = %#v, want nil with an error", value)
			}
			return err
		}},
		{name: "follow-up messages", operation: "AgentSession.GetFollowUpMessages", call: func() error {
			value, err := session.GetFollowUpMessages()
			if value != nil {
				t.Errorf("GetFollowUpMessages = %#v, want nil with an error", value)
			}
			return err
		}},
		{name: "thinking levels", operation: "AgentSession.GetAvailableThinkingLevels", call: func() error {
			value, err := session.GetAvailableThinkingLevels()
			if value != nil {
				t.Errorf("GetAvailableThinkingLevels = %#v, want nil with an error", value)
			}
			return err
		}},
		{name: "context usage", operation: "AgentSession.GetContextUsage", call: func() error {
			value, err := session.GetContextUsage()
			if value != nil {
				t.Errorf("GetContextUsage = %#v, want nil with an error", value)
			}
			return err
		}},
		{name: "last assistant text", operation: "AgentSession.GetLastAssistantText", call: func() error {
			value, err := session.GetLastAssistantText()
			if value != nil {
				t.Errorf("GetLastAssistantText = %#v, want nil with an error", value)
			}
			return err
		}},
		{name: "session stats", operation: "AgentSession.GetSessionStats", call: func() error {
			value, err := session.GetSessionStats()
			if !reflect.DeepEqual(value, codingagent.SessionStats{}) {
				t.Errorf("GetSessionStats = %#v, want zero value with an error", value)
			}
			return err
		}},
		{name: "tool definition", operation: "AgentSession.GetToolDefinition", call: func() error {
			value, ok, err := session.GetToolDefinition("read")
			if ok || !reflect.DeepEqual(value, codingagent.ToolDefinition{}) {
				t.Errorf("GetToolDefinition = (%#v, %t), want (zero, false) with an error", value, ok)
			}
			return err
		}},
		{name: "forking messages", operation: "AgentSession.GetUserMessagesForForking", call: func() error {
			value, err := session.GetUserMessagesForForking()
			if value != nil {
				t.Errorf("GetUserMessagesForForking = %#v, want nil with an error", value)
			}
			return err
		}},
		{name: "extension handlers", operation: "AgentSession.HasExtensionHandlers", call: func() error {
			value, err := session.HasExtensionHandlers("tool_call")
			if value {
				t.Error("HasExtensionHandlers returned true with an error")
			}
			return err
		}},
		{name: "prompt templates", operation: "AgentSession.PromptTemplates", call: func() error {
			value, err := session.PromptTemplates()
			if value != nil {
				t.Errorf("PromptTemplates = %#v, want nil with an error", value)
			}
			return err
		}},
		{name: "supports thinking", operation: "AgentSession.SupportsThinking", call: func() error {
			value, err := session.SupportsThinking()
			if value {
				t.Error("SupportsThinking returned true with an error")
			}
			return err
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := test.call()
			if !errors.Is(err, codingagent.ErrNotImplemented) {
				t.Fatalf("error = %v, want ErrNotImplemented", err)
			}
			var target *codingagent.NotImplementedError
			if !errors.As(err, &target) || target.Module != "codingagent" || target.Operation != test.operation {
				t.Fatalf("error = %#v, want codingagent.%s", target, test.operation)
			}
		})
	}
}
