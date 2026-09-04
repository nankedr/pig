# M2 冻结源码导航

Pi 路径均相对于 Code Baseline `936aff00918de1187f085f123c2812d8f2d67745`。本表是导航；Capability Status 和证据记录以 [Parity Catalog](../../../parity/catalog.jsonl) 为准。

| 公共链路 | Pi 源码 | Pig 入口 | 离线证据与示例 |
| --- | --- | --- | --- |
| thinking/signature | `packages/ai/src/api/openai-completions.ts`、`types.ts` | `ai.StreamOpenAICompletions`、Faux | `ai/issue60_thinking_test.go`、`ai/issue70_freeze_test.go`；`examples/thinking-signatures` |
| usage/cost/cache | `packages/ai/src/models.ts`、`providers/faux.ts` | `ai.CalculateCost`、Faux、Models | `internal/parity/usage_cost_cache_test.go`；`examples/usage-cost-cache` |
| deferred response | `packages/ai/src/providers/faux.ts`、`models.ts` | `Models.FetchDeferred/CancelDeferred` | `internal/parity/deferred_lifecycle_test.go`、`ai/issue62_provider_test.go`；`examples/deferred-response` |
| deferred tools | `packages/ai/src/utils/deferred-tools.ts` | `ai.SplitDeferredTools` | `internal/parity/deferred_tools_test.go`、`agent/issue63_deferred_tools_test.go`；`examples/deferred-tools` |
| Telemetry | `packages/telemetry/src/memory.ts`、`testing/conformance.ts` | `InMemoryTelemetryContext`、`CreateTelemetryAdapterConformance` | `internal/parity/telemetry_test.go`、`telemetry/testing/conformance_test.go`；`examples/telemetry` |
| compat/deprecated/resources | `packages/ai/src/compat.ts`、`legacy-api-aliases.ts`、`session-resources.ts` | `ai.Stream/Complete`、API registry、cleanup | `internal/parity/compat_session_resources_test.go`、`ai/issue65_compat_test.go`；`examples/compat-session-resources` |
| handoff | `packages/ai/src/api/transform-messages.ts` | `ai.TransformMessages`、`Agent.SetModel` | `internal/parity/message_handoff_test.go`、`agent/issue66_handoff_test.go`；`examples/message-handoff` |
| overflow | `packages/ai/src/utils/overflow.ts`、`utils/estimate.ts` | `ai.EstimateContextTokens`、`ai.IsContextOverflow` | `internal/parity/context_overflow_test.go`；`examples/context-overflow` |
| steering/follow-up | `packages/agent/src/agent.ts`、`agent-loop.ts` | `Agent.Steer/FollowUp/Continue/Abort/WaitForIdle` | `internal/parity/legacy_agent_queues_test.go`、`agent/issue68_queues_test.go`；`examples/legacy-agent-queues` |
| proxy | `packages/agent/src/proxy.ts` | `agent.StreamProxy`、`AssistantMessageEventStream` | `internal/parity/agent_proxy_test.go`、`agent/proxy_runtime_test.go`；`examples/agent-proxy` |

各 Oracle 在 `parity/oracle/`，已提交结果在 `parity/oracle/fixtures/`。proxy 的请求、取消和不可变快照契约见 [使用说明](../../proxy.md) 与 [ADR-0019](../../adr/0019-proxy-stream-reconstruction.md)。

冻结和复现命令见 [M2 集成与冻结](../../learning/m2-freeze.md)；M1 text/json、Tool、Provider 和取消回归仍使用 [M1 导航](m1-headless-text.md) 中的测试与 snapshots。
