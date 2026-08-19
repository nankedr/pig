// Package ai defines Pig's model-facing messages, content blocks, tools,
// models, request options, Provider/API Adapter seams, and event streams.
//
// Message, content, constrained-sampling, and AssistantMessageEvent values are
// closed unions with published wire discriminators. Their codecs reject unknown
// variants. Open extension data is retained only at explicitly open seams, such
// as a custom API Adapter's raw options; application-specific Agent messages
// belong to package agent.
//
// EventStream separates a model Stream Outcome from Go operational errors. A
// Provider failure or cancellation is a terminal AssistantMessage outcome that
// retains partial content. Go errors report waiter cancellation, malformed
// representations, internal invariant failures, or an explicit M0 Capability
// Stub.
//
// This package is an M0 contract scaffold. Pure value, codec, and in-memory
// stream behavior is available. Provider I/O, authentication, model catalogs,
// and protocol-specific implementations remain later-milestone capabilities
// and must fail with ErrNotImplemented rather than performing side effects.
package ai
