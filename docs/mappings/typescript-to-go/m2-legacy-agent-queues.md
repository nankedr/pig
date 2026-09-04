# M2.9 TypeScript 到 Go 导航

Code Baseline：`936aff00918de1187f085f123c2812d8f2d67745`。

| Pi | Pig | 证据 |
| --- | --- | --- |
| `agent.ts#MessageQueue`、`Agent.steer/followUp`、模式与 clear | `agent/agent.go` 的独立受锁 FIFO 队列 | `TestLegacyAgentQueueModes`、`TestLegacyAgentClearQueuesAndStopBoundary` |
| `agent-loop.ts#runLoop` 的 steering/follow-up 边界 | `agent/runtime.go`、`agent/tool_runtime.go` | `TestLegacyAgentFollowUpWaitsForToolsAndSteering`、共同 Parity fixture |
| `agent.ts#Agent.continue` 和 `skipInitialSteeringPoll` | `agent/agent.go#Continue/startRun` | `TestLegacyAgentQueueStateTransitions`、八场景 Oracle 重放 |
| `Agent.abort/reset/waitForIdle`、`processEvents` | `agent/agent.go` 的 run context、idle channel 与 listener 顺序 | `TestLegacyAgentWaitsForAllEndListenersAfterError`、并发 race 测试 |

`parity/oracle/legacy-agent-queues.mjs` 直接运行固定 Pi 源码，生成共同 fixture 与投递/listener 错误的 deviation fixture。`internal/parity/legacy_agent_queues_test.go` 重放公开 Go SDK；`agent/issue29_surface_test.go` 继续锁定现有公开签名。

见 [学习文档](../../learning/m2-legacy-agent-queues.md) 与 [ADR-0018](../../adr/0018-legacy-agent-queue-admission.md)。
