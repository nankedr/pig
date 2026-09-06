# 默认编码任务：TypeScript → Go

固定 Pi commit：`936aff00918de1187f085f123c2812d8f2d67745`。

| Pi | Pig | 可观察边界 |
| --- | --- | --- |
| `core/tools/index.ts#createCodingTools` | `codingagent.CreateCodingTools` | read/bash/edit/write 工厂与 options |
| `core/sdk.ts#createAgentSession` | `codingagent.CreateAgentSession`、`CreateAgentSessionFromServices` | 默认装配、选择/排除/禁用、settings、Session 身份 |
| `core/agent-session.ts` 工具与 prompt 构建 | `session_services.go`、`headless.go` | CLI/SDK 同一工具来源，提示词与实际工具一致 |
| `core/sdk.ts` streamFn | `codingagent/sdk.go` | Provider retry/timeout、transport/queue/thinking 参数 |
| `core/session-manager.ts` open/create/fork | `OpenSessionManager`、`NewSessionManager`、`ForkSessionManager` | append-only 恢复和不修改源文件的 fork |
| `core/tools/bash.ts` session environment | `codingagent/bash_tool.go` | 运行时当前身份，按 ADR-0008 使用 PIG_* |
| `api/openai-completions.ts` cacheRetention none | `ai/openai_completions.go` | 保留 Provider SessionID、无缓存/affinity 网络字段 |
| `modes/print-mode.ts` | `cmd/pig`、`codingagent/modes.go` | text/json 跨进程任务与 session-first 事件 |

学习路径：`go run ./examples/coding-task` → [M3.11](../../learning/m3-coding-task.md) → `codingagent/issue81_tools_test.go` → `cmd/pig/issue81_process_test.go`。选择语义的 Pi Oracle 位于 `parity/oracle/coding-tools.mjs`，公开 API 快照与 Catalog 均随 issue #81 同步。
