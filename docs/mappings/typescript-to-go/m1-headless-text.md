# M1 Headless text 与 JSON TypeScript 到 Go 导航

本页只帮助对照固定 Pi 源码与 Pig 的 M1 Headless text/JSON 实现。能力状态和证据仍以 `parity/catalog.jsonl` 为唯一权威。

| Pi 来源 | Pig 实现 | 责任 | 主要证据 |
| --- | --- | --- | --- |
| `packages/coding-agent/src/main.ts#main` | `codingagent.Main`、`codingagent.CreateHeadlessSession` | 解析真实进程输入并显式装配内存 Session | `cmd/pig/headless_process_test.go` |
| `packages/coding-agent/src/modes/print-mode.ts#runPrintMode` | `codingagent.RunPrintMode`、`codingagent.RunHeadless` | 共享 Prompt 生命周期、最终 outcome、text 呈现与 session-first JSONL | `codingagent/headless_test.go`、`cmd/pig/headless_process_test.go` |
| `packages/coding-agent/src/modes/json-event.ts#JsonAgentSessionEvent` | `codingagent.JSONAgentSessionEvent`、`codingagent.JSONAgentSessionMessageUpdateEvent` | 投影 wire 事件并移除累计 message 与嵌套 partial snapshot | `codingagent/exact_results_test.go`、`cmd/pig/headless_process_test.go` |
| `packages/coding-agent/src/core/session-manager.ts#SessionHeader` | `codingagent.SessionHeader`、`codingagent.SessionManager.GetHeader` | 在任何 AgentSessionEvent 之前输出内存 v3 Session header | `cmd/pig/headless_process_test.go` |
| `packages/coding-agent/src/core/agent-session-runtime.ts#createAgentSessionRuntime` | `codingagent.CreateAgentSessionRuntime`、`codingagent.AgentSessionRuntime.Dispose` | runtime 创建与 Session 生命周期所有权 | `codingagent/session_api_final_review_test.go` |
| `packages/coding-agent/src/core/sdk.ts#createAgentSession` | `codingagent.CreateAgentSession` | 注入 Provider/stream、Tool 与内存 v3 SessionManager | `codingagent/agent_session_runtime_test.go` |
| `packages/coding-agent/src/core/tools/read.ts#createReadTool` | `codingagent.CreateReadTool` | 真实文件读取与 ToolResult continuation | `codingagent/headless_test.go`、`internal/parity/codingagent_read_continuation_test.go` |
| `packages/ai/src/providers/register-builtins.ts` 与 DeepSeek catalog | `ai.BuiltinModels`、`ai.Models.StreamSimple` | 固定模型目录、标准 `DEEPSEEK_API_KEY` 与 Chat Completions stream | `cmd/pig/headless_process_test.go` |

Pig 把可复用生命周期放在 `RunHeadless`，把 stdout 策略放在 `RunPrintMode`。这是 Go 侧的产品组合 seam，不把 Headless 误建模成第二套 Agent loop。JSON presenter 在订阅 Session 后复用同一生命周期，先输出内存 Session header，再逐行输出投影后的 `AgentSessionEvent`；它不接受 stdin command，也不替代 M4 RPC。
