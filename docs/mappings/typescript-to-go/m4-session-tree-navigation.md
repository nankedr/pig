# Session 树导航：TypeScript → Go

| 固定 Pi 入口 | Pig 入口 | 观察行为 |
| --- | --- | --- |
| core/sdk.ts createAgentSession | codingagent/sdk.go CreateAgentSession | 公开会话创建、历史恢复和 Prompt |
| core/agent-session.ts navigateTree | codingagent/session_tree_navigation.go AgentSession.NavigateTree | no-op、目标选择、EditorText、标签、消息替换和失败边界 |
| navigateTree options / result | NavigateTreeOptions / NavigateTreeResult | 可选无摘要参数、编辑文本；Go 增加 context 和 error |
| core/session-manager.ts branch / resetLeaf / appendLabelChange | codingagent/session_tree.go 及导航提交 | parentId/leaf/label 语义；标签原子提交沿用既有 v3 编码 |
| buildSessionContext | codingagent/session_manager.go BuildSessionContext | 目标路径配置、压缩和分支摘要投影 |
| test/agent-session-tree-navigation.test.ts | parity/oracle/session-tree-navigation.mjs、codingagent/issue91_navigation_test.go | 固定 Pi source/dist 和 Go 公开 SDK 对比 |
| test/suite/regressions/tree-during-streaming.test.ts | codingagent/issue91_runtime_test.go | 忙状态拒绝、重入监听器、继续生成及资源保留 |
| session_before_tree / session_tree | 后继扩展切片 | 未执行扩展 hook，不增加伪造订阅事件 |
| summarize / abortBranchSummary | 后继摘要导航切片 | 不同目标 Summarize=true 明确 Stub |

运行 `go run ./examples/session-tree-navigation`，配合[中文学习材料](../../learning/m4-session-tree-navigation.md)及 [ADR-0027](../../adr/0027-session-tree-navigation.md)。
