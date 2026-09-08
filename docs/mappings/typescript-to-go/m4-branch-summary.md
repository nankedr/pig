# 分支摘要：TypeScript → Go

| 固定 Pi 入口 | Pig 入口 | 行为 |
| --- | --- | --- |
| core/sdk.ts createAgentSession | codingagent.CreateAgentSession | 公开 SDK 创建与持久化恢复 |
| core/agent-session.ts navigateTree | session_tree_navigation.go、session_branch_summary.go | 离开路径选择、摘要、挂载与原子提交 |
| core/compaction/branch-summarization.ts collectEntriesForBranchSummary / prepareBranchEntries | compaction.go | 共同祖先、预算、已有摘要和文件收集 |
| generateBranchSummary | branch_summary.go | GenerateBranchSummaryOptions、指令、2048 输出预算与文件列表 |
| compaction.ts completeSummarization | compaction_summary.go | 复用 stream、独立路由 ID、禁用缓存和摘要重试 |
| abortBranchSummary / isCompacting | session.go | 取消、占用状态与 WaitForIdle |
| session-manager.ts branchWithSummary | SessionManager.BranchWithSummary / 导航提交 | v3 摘要字段、挂载位置和标签语义 |
| test/agent-session-tree-navigation.test.ts | issue92_summary_test.go、parity/oracle/branch-summary.mjs | source/dist 与 Go 公开 SDK Oracle 对比 |
| test/suite/regressions/6324-branch-summary-ambient-auth.test.ts | issue92_runtime_test.go | 注入 stream 及 Provider 认证、实际 HTTP 重试 |
| session_before_tree / session_tree | 后继扩展切片 | 保持 Stub，不模拟扩展事件 |

运行 `go run ./examples/branch-summary`，配合[中文学习材料](../../learning/m4-branch-summary.md)和 [ADR-0028](../../adr/0028-branch-summary-navigation.md)。
