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
// This package is a partial M1 AI contract/runtime. Pure value and codec
// behavior is live, along with the in-memory CredentialStore and ModelsStore,
// provider/model runtime composition, built-in provider/model registry
// assembly, and model catalog/query helpers. Those helpers intentionally keep
// the fixed baseline model snapshot in honest pending-capture state: built-in
// providers exist, but built-in model lists and generated-at provenance remain
// absent until the real catalog snapshot is captured.
//
// Real provider side effects remain later-milestone capabilities. Ambient
// environment and filesystem access, provider/catalog network refresh, real
// provider stream I/O, OAuth refresh/login flows, and protocol-specific
// implementations must still fail with ErrNotImplemented or other structured
// non-success outcomes rather than touching the network, process environment,
// or filesystem.
package ai
