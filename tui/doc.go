// Package tui defines Pig's terminal user-interface compatibility boundary.
//
// The package currently exposes the fixed Pi baseline's terminal, input,
// layout, component, editor, theme, and image contracts. Safe local value and
// container operations are implemented without ambient work. Operations that
// require interactive runtime behavior remain Capability Stubs: they fail with
// ErrNotImplemented and do not access process input, write terminal output,
// change terminal modes, start timers, or load native helpers. Interactive
// behavior is implemented and frozen at the M6 TUI milestone.
//
// Pi's TUI package does not restrict package exports, so its source modules are
// technically reachable as deep imports. Pig maps those deep-import symbols to
// this one canonical Go package; the Parity Catalog records each mapping.
package tui
