package agent_test

import (
	"context"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/telemetry"
)

func TestStartHarnessSpanDelegatesToTelemetryContext(t *testing.T) {
	got, err := agent.StartHarnessSpan(
		context.Background(),
		telemetry.NOOPTelemetryContext,
		"pi.harness.run",
		telemetry.SpanAttributes{"pi.session.id": "session"},
		func(_ context.Context, span agent.HarnessTelemetrySpan) (string, error) {
			span.SetAttributes(telemetry.SpanAttributes{"pi.operation.outcome": "completed"})
			return "done", nil
		},
	)
	if err != nil || got != "done" {
		t.Fatalf("StartHarnessSpan() = %q, %v", got, err)
	}
	if len(agent.AgentTelemetrySchemas) != 2 {
		t.Fatalf("AgentTelemetrySchemas has %d schemas", len(agent.AgentTelemetrySchemas))
	}
}
