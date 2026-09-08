# M4.8 自动压缩：TypeScript → Go

固定 Pi：936aff00918de1187f085f123c2812d8f2d67745，production v3 AgentSession。

| Pi 路径 / 方法 | Pig 路径 / 方法 | 阅读重点 |
| --- | --- | --- |
| core/agent-session.ts / prompt、_handlePostAgentRun | codingagent/session.go / Prompt | 输入前检查、Provider retry 优先、整轮收尾 |
| core/agent-session.ts / _checkCompaction | codingagent/session_auto_compaction.go / checkAutoCompaction | threshold、overflow、length、旧 usage、恢复预算 |
| core/agent-session.ts / _runAutoCompaction | codingagent/session_auto_compaction.go / runAutoCompaction | 事件、取消和一次恢复 |
| core/agent-session.ts / compact、_summarizationRetryCallbacks | codingagent/session_compaction.go / compactPrepared | 手动/自动共享摘要请求与原子 v3 提交 |
| core/agent-session.ts / steer、followUp | codingagent/session_messages.go；session_auto_compaction.go | 自动压缩期间暂存、队列模式与恢复 |
| core/agent-session.ts / setAutoCompactionEnabled、autoCompactionEnabled | codingagent/session.go；settings_accessors.go | 有效设置和同步持久化 |
| core/agent-session.ts / _lastAssistantMessage | codingagent/headless.go / headlessOutcome | 被移除 partial 的独立结果所有权 |

先运行 examples/auto-compaction，再对照 parity/oracle/auto-compaction.mjs 的公开 SDK 场景。codingagent/issue90_compaction_test.go 重放同一 fixture；issue90_runtime_test.go 通过 Headless 检查取消、持久化失败及真实子进程重开。Go Context 和队列 admission 的决策见 [ADR-0028](../../adr/0028-auto-compaction.md)。
