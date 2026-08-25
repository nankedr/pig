package agent

import (
	"context"

	"github.com/nankedr/pig/telemetry"
)

type (
	AISpanName            = telemetry.TelemetrySchemaSpanName
	AISpanStartAttributes = telemetry.TelemetrySchemaSpanStartAttributes
	AISpanEndAttributes   = telemetry.TelemetrySchemaSpanEndAttributes
	AISpanAttributes      = telemetry.SpanAttributes
	AISpanEventName       = telemetry.TelemetrySchemaSpanEventName
	AISpanEventAttributes = telemetry.TelemetrySchemaSpanEventAttributes
	AITelemetrySpan       = telemetry.SchemaTelemetrySpan
	AISpan                = telemetry.TelemetrySchemaSpanUnion

	HarnessSpanName            = telemetry.TelemetrySchemaSpanName
	HarnessSpanStartAttributes = telemetry.TelemetrySchemaSpanStartAttributes
	HarnessSpanEndAttributes   = telemetry.TelemetrySchemaSpanEndAttributes
	HarnessSpanAttributes      = telemetry.SpanAttributes
	HarnessSpanEventName       = telemetry.TelemetrySchemaSpanEventName
	HarnessSpanEventAttributes = telemetry.TelemetrySchemaSpanEventAttributes
	HarnessTelemetrySpan       = telemetry.SchemaTelemetrySpan
	HarnessSpan                = telemetry.TelemetrySchemaSpanUnion
)

var AITelemetrySchema = telemetry.TelemetrySchemaDefinition{
	Version: 1,
	Spans: map[string]telemetry.TelemetrySpanDefinition{
		"pi.ai.request": {
			Description: "One logical request to an AI provider",
			Parents:     telemetry.TelemetryParentDefinition{Kind: telemetry.ParentKindAny},
			Status:      telemetry.TelemetrySpanStatusRule{Default: telemetry.SpanStatusOK, ErrorWhen: "The operation throws or returns an error result"},
		},
	},
}

var HarnessTelemetrySchema = telemetry.TelemetrySchemaDefinition{
	Version: 1,
	Spans: map[string]telemetry.TelemetrySpanDefinition{
		"pi.harness.run":           {Parents: telemetry.TelemetryParentDefinition{Kind: telemetry.ParentKindRootOrExternal}},
		"pi.harness.compaction":    {Parents: telemetry.TelemetryParentDefinition{Kind: telemetry.ParentKindRootOrExternal}},
		"pi.harness.navigation":    {Parents: telemetry.TelemetryParentDefinition{Kind: telemetry.ParentKindRootOrExternal}},
		"pi.harness.checkpoint":    {Parents: telemetry.TelemetryParentDefinition{Kind: telemetry.ParentKindSpans, Spans: []string{"pi.harness.run"}}},
		"pi.harness.turn":          {Parents: telemetry.TelemetryParentDefinition{Kind: telemetry.ParentKindSpans, Spans: []string{"pi.harness.run"}}},
		"pi.harness.step":          {Parents: telemetry.TelemetryParentDefinition{Kind: telemetry.ParentKindSpans, Spans: []string{"pi.harness.run"}}},
		"pi.harness.tool":          {Parents: telemetry.TelemetryParentDefinition{Kind: telemetry.ParentKindSpans, Spans: []string{"pi.harness.run"}}},
		"pi.harness.hook":          {Parents: telemetry.TelemetryParentDefinition{Kind: telemetry.ParentKindAny}},
		"pi.harness.sleep":         {Parents: telemetry.TelemetryParentDefinition{Kind: telemetry.ParentKindAny}},
		"pi.harness.event_handler": {Parents: telemetry.TelemetryParentDefinition{Kind: telemetry.ParentKindAny}},
		"pi.session.write":         {Parents: telemetry.TelemetryParentDefinition{Kind: telemetry.ParentKindAny}},
	},
}

var AgentTelemetrySchemas = []telemetry.TelemetrySchemaDefinition{AITelemetrySchema, HarnessTelemetrySchema}

func StartAISpan[T any](
	ctx context.Context,
	telemetryContext telemetry.TelemetryContext,
	name AISpanName,
	attributes AISpanStartAttributes,
	callback func(context.Context, AITelemetrySpan) (T, error),
) (T, error) {
	value, err := telemetryContext.StartSpan(ctx, telemetry.SpanOptions{Name: name, Attributes: attributes}, func(ctx context.Context, span telemetry.TelemetrySpan) (any, error) {
		return callback(ctx, span)
	})
	if err != nil {
		var zero T
		return zero, err
	}
	return value.(T), nil
}

func StartHarnessSpan[T any](
	ctx context.Context,
	telemetryContext telemetry.TelemetryContext,
	name HarnessSpanName,
	attributes HarnessSpanStartAttributes,
	callback func(context.Context, HarnessTelemetrySpan) (T, error),
) (T, error) {
	value, err := telemetryContext.StartSpan(ctx, telemetry.SpanOptions{Name: name, Attributes: attributes}, func(ctx context.Context, span telemetry.TelemetrySpan) (any, error) {
		return callback(ctx, span)
	})
	if err != nil {
		var zero T
		return zero, err
	}
	return value.(T), nil
}
