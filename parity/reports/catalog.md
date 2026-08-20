<!-- GENERATED FILE - DO NOT EDIT BY HAND -->
# Parity Catalog Report (non-authoritative)

> This is a NON-AUTHORITATIVE generated view of `parity/catalog.jsonl`.
> Do not hand-edit this file; it is regenerated from the Parity Catalog.
> The Parity Catalog (`parity/catalog.jsonl`) is the single machine-readable authority.

## Summary

- Total entries: 37

| Status | Count |
| --- | --- |
| inventoried | 12 |
| scaffolded | 17 |
| partial | 6 |
| implemented | 0 |
| verified | 0 |
| deferred | 2 |

## Entries by module

### agent

| ID | Status | Milestone | Kind | Target | Upstream |
| --- | --- | --- | --- | --- | --- |
| contract:session/v4-harness | inventoried | M8 | contract | github.com/nankedr/pig/agent | agent |
| module-agent | scaffolded | M1 | package | github.com/nankedr/pig/agent | agent |

### ai

| ID | Status | Milestone | Kind | Target | Upstream |
| --- | --- | --- | --- | --- | --- |
| contract:auth/pig-ai/login-cli | inventoried | M11 | contract | github.com/nankedr/pig/cmd/pig-ai | ai |
| cmd-pig-ai | scaffolded | M11 | command | github.com/nankedr/pig/cmd/pig-ai | ai |
| contract:ai/api-adapter | scaffolded | M1 | contract | github.com/nankedr/pig/ai | ai |
| contract:ai/model | scaffolded | M1 | contract | github.com/nankedr/pig/ai | ai |
| contract:ai/provider | scaffolded | M1 | contract | github.com/nankedr/pig/ai | ai |
| module-ai | scaffolded | M1 | package | github.com/nankedr/pig/ai | ai |
| contract:ai/auth | partial | M1 | contract | github.com/nankedr/pig/ai | ai |
| contract:ai/content | partial | M1 | contract | github.com/nankedr/pig/ai | ai |
| contract:ai/event-stream | partial | M1 | contract | github.com/nankedr/pig/ai | ai |
| contract:ai/message | partial | M1 | contract | github.com/nankedr/pig/ai | ai |
| contract:ai/options | partial | M1 | contract | github.com/nankedr/pig/ai | ai |
| contract:ai/tool | partial | M1 | contract | github.com/nankedr/pig/ai | ai |

### client

| ID | Status | Milestone | Kind | Target | Upstream |
| --- | --- | --- | --- | --- | --- |
| contract:client/request-cancellation | scaffolded | M9 | contract | github.com/nankedr/pig/client | client |
| module-client | scaffolded | M9 | package | github.com/nankedr/pig/client | client |

### codingagent

| ID | Status | Milestone | Kind | Target | Upstream |
| --- | --- | --- | --- | --- | --- |
| contract:cli/pig/args | inventoried | M3 | contract | github.com/nankedr/pig/cmd/pig | coding-agent |
| contract:cli/pig/exit-codes | inventoried | M3 | contract | github.com/nankedr/pig/cmd/pig | coding-agent |
| contract:config/auth-json | inventoried | M3 | contract | github.com/nankedr/pig/codingagent | coding-agent |
| contract:config/models-json | inventoried | M3 | contract | github.com/nankedr/pig/codingagent | coding-agent |
| contract:config/settings | inventoried | M3 | contract | github.com/nankedr/pig/codingagent | coding-agent |
| contract:migration/auth-and-layout | inventoried | M5 | contract | github.com/nankedr/pig/codingagent | coding-agent |
| contract:rpc/command-union | inventoried | M4 | contract | github.com/nankedr/pig/codingagent | coding-agent |
| contract:rpc/jsonl-transport | inventoried | M4 | contract | github.com/nankedr/pig/codingagent | coding-agent |
| contract:session/migration | inventoried | M3 | contract | github.com/nankedr/pig/codingagent | coding-agent |
| contract:session/v3-jsonl | inventoried | M3 | contract | github.com/nankedr/pig/codingagent | coding-agent |
| cmd-pig | scaffolded | M1 | command | github.com/nankedr/pig/cmd/pig | coding-agent |
| module-codingagent | scaffolded | M3 | package | github.com/nankedr/pig/codingagent | coding-agent |
| deferred-extension-runtime | deferred | M7 | contract | github.com/nankedr/pig/codingagent | coding-agent |
| deferred-pig-server | deferred | M0 | contract | github.com/nankedr/pig/codingagent | server |

### protocol

| ID | Status | Milestone | Kind | Target | Upstream |
| --- | --- | --- | --- | --- | --- |
| contract:protocol/cbor | scaffolded | M9 | contract | github.com/nankedr/pig/protocol | protocol |
| contract:protocol/frame | scaffolded | M9 | contract | github.com/nankedr/pig/protocol | protocol |
| contract:protocol/schema-codec | scaffolded | M9 | contract | github.com/nankedr/pig/protocol | protocol |
| contract:protocol/version | scaffolded | M9 | contract | github.com/nankedr/pig/protocol | protocol |
| module-protocol | scaffolded | M9 | package | github.com/nankedr/pig/protocol | protocol |

### telemetry

| ID | Status | Milestone | Kind | Target | Upstream |
| --- | --- | --- | --- | --- | --- |
| module-telemetry | scaffolded | M2 | package | github.com/nankedr/pig/telemetry | telemetry |

### tui

| ID | Status | Milestone | Kind | Target | Upstream |
| --- | --- | --- | --- | --- | --- |
| module-tui | scaffolded | M6 | package | github.com/nankedr/pig/tui | tui |
