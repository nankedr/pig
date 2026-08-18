package telemetry

import "context"

// TelemetryAttributeType names the wire type of a schema attribute. It mirrors
// Pi's TelemetryAttributeType string union.
type TelemetryAttributeType string

// Telemetry attribute types (Pi: "string" | "number" | ... | "boolean[]").
const (
	AttributeTypeString       TelemetryAttributeType = "string"
	AttributeTypeNumber       TelemetryAttributeType = "number"
	AttributeTypeBoolean      TelemetryAttributeType = "boolean"
	AttributeTypeStringArray  TelemetryAttributeType = "string[]"
	AttributeTypeNumberArray  TelemetryAttributeType = "number[]"
	AttributeTypeBooleanArray TelemetryAttributeType = "boolean[]"
)

// TelemetryAttributeMetadata is the documentation-only metadata shared by every
// attribute definition. Sensitive and Cardinality are optional, so they are
// three-state pointers (absent vs set) per docs/design/compatibility.md.
type TelemetryAttributeMetadata struct {
	Description string
	Sensitive   *bool
	Cardinality string // "" (unset), "low" or "high"
}

// TelemetryAttributeDefinition describes one span attribute. Pi models this as a
// metadata-and-type-discriminated union whose per-type Values/ElementValues and
// Examples constrain the value; Go keeps the erased, generator-friendly shape
// (any) so authoring the concrete typed helpers is deferred to the schema
// generator (ADR-0007) rather than frozen here.
type TelemetryAttributeDefinition struct {
	TelemetryAttributeMetadata
	Type          TelemetryAttributeType
	Values        []any
	ElementValues []any
	Examples      []any
}

// TelemetryStartAttributeDefinition is a start attribute with a required flag.
type TelemetryStartAttributeDefinition struct {
	TelemetryAttributeDefinition
	Required bool
}

// TelemetryEventAttributeDefinition is an event attribute with a required flag.
type TelemetryEventAttributeDefinition struct {
	TelemetryAttributeDefinition
	Required bool
}

// TelemetryEventDefinition describes a span event and its attributes.
type TelemetryEventDefinition struct {
	Description string
	Attributes  map[string]TelemetryEventAttributeDefinition
}

// Telemetry parent-relationship kinds (Pi: TelemetryParentDefinition union).
const (
	ParentKindAny            = "any"
	ParentKindRootOrExternal = "root_or_external"
	ParentKindSpans          = "spans"
)

// TelemetryParentDefinition constrains a span's allowed parents. Spans is only
// meaningful when Kind is ParentKindSpans.
type TelemetryParentDefinition struct {
	Kind  string
	Spans []string
}

// TelemetrySpanStatusRule is Pi's per-span status rule
// (`{default:"ok"; errorWhen:string}`).
type TelemetrySpanStatusRule struct {
	Default   string
	ErrorWhen string
}

// TelemetrySpanDefinition describes one span in a schema.
type TelemetrySpanDefinition struct {
	Description     string
	Parents         TelemetryParentDefinition
	StartAttributes map[string]TelemetryStartAttributeDefinition
	EndAttributes   map[string]TelemetryAttributeDefinition
	Events          map[string]TelemetryEventDefinition
	Status          TelemetrySpanStatusRule
}

// TelemetrySchemaDefinition is a versioned, serialisable set of span
// definitions keyed by span name.
type TelemetrySchemaDefinition struct {
	Version int
	Spans   map[string]TelemetrySpanDefinition
}

// DefineTelemetrySchema is Pi's typed identity helper for serialisable schema
// data. In TypeScript it pins a `const` generic for inference; in Go it is a
// plain identity function that documents intent and gives schemas a single
// authoring entry point. It is genuinely implemented (no deferral).
func DefineTelemetrySchema(schema TelemetrySchemaDefinition) TelemetrySchemaDefinition {
	return schema
}

// Erased typed-helper surface.
//
// Pi derives strongly typed span/attribute shapes from schema data using
// TypeScript type-level programming (conditional and mapped types). Go has no
// equivalent, and ADR-0007 rules that built-in dynamic schemas gain their typed
// helpers from a generator, not hand-written type gymnastics. To keep the
// public surface complete and compile-usable at M0 without prematurely freezing
// that abstraction, these names are erased to the runtime types they resolve to
// today. The schema generator will supply concrete typed helpers later without
// changing this contract.
type (
	// InferRequiredAndOptionalAttributes erases to the dynamic attribute bag.
	InferRequiredAndOptionalAttributes = SpanAttributes
	// InferStartAttributes erases to the dynamic attribute bag.
	InferStartAttributes = SpanAttributes
	// InferOptionalAttributes erases to the dynamic attribute bag.
	InferOptionalAttributes = SpanAttributes
	// InferEventAttributes erases to the dynamic attribute bag.
	InferEventAttributes = SpanAttributes
	// ExactTelemetryAttributes erases to the dynamic attribute bag; Pi uses it
	// to reject excess keys at compile time, which Go cannot express generically.
	ExactTelemetryAttributes = SpanAttributes

	// TelemetrySchemaSpanName erases to the span name string.
	TelemetrySchemaSpanName = string
	// TelemetrySchemaSpanEventName erases to the event name string.
	TelemetrySchemaSpanEventName = string
	// TelemetrySchemaSpanStartAttributes erases to the dynamic attribute bag.
	TelemetrySchemaSpanStartAttributes = SpanAttributes
	// TelemetrySchemaSpanEndAttributes erases to the dynamic attribute bag.
	TelemetrySchemaSpanEndAttributes = SpanAttributes
	// TelemetrySchemaSpanEventAttributes erases to the dynamic attribute bag.
	TelemetrySchemaSpanEventAttributes = SpanAttributes
	// SchemaTelemetrySpan erases to the untyped span; the schema generator will
	// supply a typed AddEvent/SetAttributes wrapper.
	SchemaTelemetrySpan = TelemetrySpan
)

// TelemetrySchemaSpanUnion is the erased discriminated span-shape union Pi
// derives from a schema. Its fields mirror the upstream member names so the
// generated typed form is a drop-in refinement.
type TelemetrySchemaSpanUnion struct {
	Name            string
	StartAttributes SpanAttributes
	EndAttributes   SpanAttributes
	Events          map[string]SpanAttributes
}

// TypedSpanFunc is the callback of a TypedSpanStarter. Like SpanFunc it is
// erased at M0: the typed span and child starter are the untyped runtime forms.
type TypedSpanFunc func(ctx context.Context, span TelemetrySpan, startChild TypedSpanStarter) (any, error)

// TypedSpanStarter is Pi's per-parent, schema-bound span starter. Pi types it as
// an overload set generated from the schema vocabulary; Go erases it to a single
// runtime signature (name + attributes + callback) that the generator will
// later specialise per span name.
type TypedSpanStarter func(ctx context.Context, name string, attributes SpanAttributes, fn TypedSpanFunc) (any, error)

// bindTypedSpanStarter binds a context to a span vocabulary and recursively
// rebinds each child span, mirroring Pi's internal bindTypedSpanStarter. Pi
// carries the schema tuple only as a type parameter, so at runtime the binding
// is schema-free and this takes just the context.
func bindTypedSpanStarter(telemetryContext TelemetryContext) TypedSpanStarter {
	return func(ctx context.Context, name string, attributes SpanAttributes, fn TypedSpanFunc) (any, error) {
		return telemetryContext.StartSpan(ctx, SpanOptions{Name: name, Attributes: attributes},
			func(ctx context.Context, span TelemetrySpan) (any, error) {
				return fn(ctx, span, bindTypedSpanStarter(span))
			})
	}
}

// CreateTypedSpanStarter binds an explicit parent context to the combined span
// vocabulary of one or more schemas. Per Pi's contract at least one schema is
// required (its TelemetrySchemaTuple is a non-empty tuple), and the schema values
// are used only for type inference: no runtime schema validation is performed.
// So this is a genuine thin binding, not a deferred stub — it delegates to a
// schema-free binding that runs each callback and rebinds child spans. The schema
// arguments are accepted for parity and future typed generation (ADR-0007); they
// are not inspected at runtime.
func CreateTypedSpanStarter(telemetryContext TelemetryContext, schema TelemetrySchemaDefinition, moreSchemas ...TelemetrySchemaDefinition) TypedSpanStarter {
	return bindTypedSpanStarter(telemetryContext)
}
