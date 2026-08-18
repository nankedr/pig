// Package protocol defines Pig's compatibility surface for the Remote Session
// Protocol. The Remote Session Protocol is a strict, schema-defined wire
// protocol whose validated messages are encoded as CBOR and carried in frames
// with a 4-byte big-endian length prefix.
//
// The Remote Session Protocol is distinct from the Coding Agent JSONL RPC
// protocol. Coding Agent JSONL RPC exchanges line-delimited JSON requests,
// responses, and events with a coding-agent process; its messages and transport
// are not interchangeable with this package's CBOR-framed session protocol.
//
// This package is an M0 scaffold. Its wire declarations and public capability
// surface are present for source compatibility, but schema validation and
// parsing, CBOR encoding and decoding, frame encoding and decoding, message
// codecs, and streaming decoders remain M9 capability stubs that report
// ErrNotImplemented.
package protocol
