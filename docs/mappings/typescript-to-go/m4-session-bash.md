# M4.11：Session Bash 的 TypeScript → Go 导航

固定 Pi commit：`936aff00918de1187f085f123c2812d8f2d67745`。

| Pi | Pig | 可观察契约 |
| --- | --- | --- |
| `core/agent-session.ts` executeBash/abortBash/recordBashResult | `codingagent/session_bash.go` | ExecuteBash/AbortBash/RecordBashResult、执行集合、输出事件 |
| `core/agent-session.ts` _pendingBashMessages/_flushPendingBashMessages | `codingagent/session.go`、`session_bash.go` | 生成期间排队，Prompt 结算与下次请求前刷新 |
| `core/bash-executor.ts` executeBashWithOperations | `codingagent/session_bash.go` executeSessionBash | 清理、滚动输出、截断、保留完整文件 |
| `core/tools/bash.ts` createLocalBashOperations | `codingagent/bash_local.go`、`bash_unix.go` | 默认无 timeout、shell 配置、取消和进程树 |
| `core/messages.ts` bashExecutionToText/convertToLlm | `codingagent/extensions.go`、`agent/harness_messages.go` | v3 开放消息解码、上下文排除及模型文本 |
| `test/suite/agent-session-bash-persistence.test.ts` | `codingagent/issue93_bash_test.go`、`issue93_runtime_test.go` | 公开 SDK 输出、并发、持久化及恢复 |
| `test/suite/regressions/5303-bash-output-truncation.test.ts` | 既有 `issue80_bash_test.go` 与 `issue93_process_unix_test.go` | 共享宿主退出读取契约、直接会话取消清理 |

Oracle：`parity/oracle/session-bash.mjs` → `fixtures/session-bash.json`，通过固定 Pi 的公开 SDK 生成，Go 通过 CreateAgentSession/ExecuteBash/Prompt/SessionManager 重放；不以私有 helper 测试单独充当对等证据。

Pi 的 AbortSignal 映射为 context；取消返回 BashResult.Cancelled 和缺省 ExitCode。消息通过 RawAgentMessage 保留 v3 小写字段与缺省字段，模型转换复用现有 Coding Agent 转换器。输出事件的可选 ID 按每个订阅者复制。执行完不触发模型调用，调用者再显式 Prompt。

完整范围和未交付分支以 `contract:codingagent/session-bash` 为准；RPC/TUI/扩展分支不因 SDK 方法实现而提升。
