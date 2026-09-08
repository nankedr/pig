# 会话统计：TypeScript → Go

| 固定 Pi 入口 | Pig 入口 | 对等行为 |
| --- | --- | --- |
| core/sdk.ts createAgentSession | codingagent/sdk.go CreateAgentSession | 创建和恢复公开 Session |
| core/agent-session.ts getSessionStats | codingagent/session_stats.go GetSessionStats | 所有持久化条目的消息、Tool、token、cost 和 Session 身份 |
| core/usage-totals.ts addUsageToTotals | GetSessionStats 中的局部累加 | 四项 token 与 cost.total，含 Tool/summary/partial usage |
| core/agent-session.ts getContextUsage | sessionContextUsage，经 SDK 查询验证 | 当前分支最新压缩边界与未知用量 |
| core/compaction/compaction.ts estimateContextTokens / calculateContextTokens / estimateTokens | sessionContextUsage 及已有 CalculateContextTokens / EstimateTokens | 有效 Assistant 锚点与后续消息估算 |
| core/agent-session.ts getLastAssistantText | codingagent/session_stats.go GetLastAssistantText | 跳过空取消、拼接 text、JS trim、空值 |
| core/extensions/types.ts ContextUsage | codingagent/extensions.go ContextUsage | 指针区分已知零与未知，扩展 hook 本身仍未实现 |
| test/agent-session-stats.test.ts / test/compaction.test.ts | parity/oracle/session-stats.mjs / codingagent/issue88_stats_test.go | 固定 Pi source/dist Oracle 和 17 组公开 SDK 对等场景 |
| core/session-manager.ts v3 文件 | session-interop fixture / codingagent/issue88_runtime_test.go | 分支与压缩、独立进程重开、查询不修改文件 |

Go 保留既有 SessionStats.Tokens 的 ai.Usage 类型；Pi 的 tokens.total 映射为 TotalTokens，费用读取 SessionStats.Cost。未新增公开签名。

RPCClient、RPC 命令、TUI、扩展和压缩编排仍保持各自未实现状态；本查询切片只消费已有 v3 数据。

[学习材料](../../learning/m4-session-stats.md) · [可运行示例](../../../examples/session-stats/main.go)
