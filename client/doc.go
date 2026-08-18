// Package client defines Pig's transport-neutral client for the Remote Session
// Protocol. That protocol validates structured messages, encodes them as CBOR,
// and uses length-prefixed byte framing. It is separate from Coding Agent JSONL
// RPC, which controls a local process through newline-delimited JSON.
//
// This M0 package maps the complete Pi client public contract into idiomatic Go
// names. Remotely meaningful operations are capability stubs until M9: they
// return ErrNotImplemented immediately without dialing, invoking listeners,
// starting goroutines, or retaining request or session state.
//
// Blocking operations accept context.Context as their Go call shape. M0 does
// not implement or claim the approved future cancellation behavior in which a
// canceled local waiter leaves a request tombstone for a late response.
//
// A SessionLease coordinates shared or exclusive lifecycle use within one
// Client. It is not authentication, authorization, or proof of remote or
// cross-client ownership. Published snapshots are immutable by contract; the
// M9 implementation must clone slice-backed values before exposing them.
package client
