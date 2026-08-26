// Package codingagent provides Pig's product-level Coding Agent SDK.
//
// It composes the legacy agent.Agent production path with AI, terminal, remote
// protocol, and client facilities. It deliberately does not alias or bridge the
// independent AgentHarness v4 session model. Operations whose implementation
// belongs to later milestones fail with ErrNotImplemented before performing
// ambient filesystem, credential, resource, process, or network work.
package codingagent
