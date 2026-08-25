package agent_test

import (
	"context"
	"errors"
	"testing"

	"github.com/nankedr/pig/agent"
)

func TestHarnessToolFactoryDefersExecutionBeforeUsingEnvironment(t *testing.T) {
	tool := agent.CreateBashTool(agent.BashToolOptions{})
	result, err := tool.Execute(context.Background(), "call-1", map[string]any{"command": "echo no"}, nil, agent.ExecutionToolContext{})
	if !errors.Is(err, agent.ErrNotImplemented) {
		t.Fatalf("Execute() error = %v, want ErrNotImplemented", err)
	}
	if result.Content != nil {
		t.Fatalf("Execute() result = %#v, want zero value", result)
	}
	var target *agent.NotImplementedError
	if !errors.As(err, &target) || target.Operation != "HarnessTool.bash.Execute" {
		t.Fatalf("Execute() error = %#v", err)
	}
	if tool.Name != "bash" || tool.Label != "bash" || len(tool.Parameters) == 0 {
		t.Fatalf("CreateBashTool() = %#v", tool)
	}
}
