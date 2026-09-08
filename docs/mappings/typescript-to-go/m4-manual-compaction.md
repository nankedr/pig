# 手动压缩：TypeScript → Go

| 固定 Pi | Pig | 可观察行为 |
| --- | --- | --- |
| core/agent-session.ts compact / abortCompaction / isCompacting | codingagent/session_compaction.go / session.go | 停止整轮、摘要生命周期、取消、提交和新上下文 |
| core/compaction/compaction.ts prepareCompaction / findCutPoint | codingagent/compaction_summary.go PrepareCompaction / compaction.go FindCutPoint | 当前分支、保留区间、split turn、上次摘要 |
| generateSummaryWithUsage / generateTurnPrefixSummary | GenerateSummaryWithUsage / completeSummary | 完整提示、80%/50% 预算、thinking、独立 sessionId |
| completeSummarization / ai utils/retry.ts retryAssistantCall | completeSummary / retrySummary / summaryResponse | 摘要重试与独立事件、明确的 SSE 缺失终态映射 |
| compaction/utils.ts extractFileOpsFromMessage / computeFileLists | extractFileOperations / Compact | 只读与修改路径、XML 附录和 details |
| session-manager.ts appendCompaction / buildSessionContext | SessionManager.AppendCompaction / BuildSessionContext | 原子 v3 保存、完整历史、重开上下文 |
| core/sdk.ts createAgentSession | codingagent/sdk.go CreateAgentSession | 当前 ModelRuntime/认证和 stream 复用 |
| test/compaction.test.ts / agent-session-compaction.test.ts | parity/oracle/manual-compaction.mjs / issue89_compaction_test.go | 公开 SDK 源码/发布构建对等用例 |
| regressions/6647-compaction-retries-transient-stream-drop.test.ts | issue89_runtime_test.go | 实际 SSE 中断恢复、取消与失败无提交 |
| regressions/7253-manual-compact-during-response.test.ts | TestManualCompactionStopsActiveTurn | 等待活跃生成终态后再写压缩记录 |

Go 用 SummaryOptions 聚合 Pi 位置参数。私有准备函数不单独作为对等证据；主要边界是 AgentSession.Compact，继续生成由 Headless 真实 HTTP 请求验证。示例：`go run ./examples/manual-compaction`。差异及未实现分支见 [ADR-0026](../../adr/0026-manual-compaction.md)。
