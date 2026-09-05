# M3.3 TypeScript → Go 导航

固定 Pi：`936aff00918de1187f085f123c2812d8f2d67745`；对应 Issue #73。

| Pi 入口 | Pig 入口 | 实现位置 |
| --- | --- | --- |
| SessionManager.list / listAll / continueRecent | ListSessions / ListAllSessions / ContinueRecentSessionManager | codingagent/session_discovery.go |
| SessionManager.forkFrom | ForkSessionManager | codingagent/session_tree.go |
| SessionManager.getTree / getChildren / branch / resetLeaf | SessionManager.GetTree / GetChildren / Branch / ResetLeaf | codingagent/session_tree.go |
| appendLabelChange / getLabel / createBranchedSession | AppendLabelChange / GetLabel / CreateBranchedSession | codingagent/session_tree.go |
| appendSessionInfo / getSessionName | AppendSessionInfo / GetSessionName | codingagent/session_manager.go |
| AgentSessionRuntime.newSession / switchSession / fork | AgentSessionRuntime.NewSession / SwitchSession / Fork | codingagent/session_runtime.go |
| AgentSession.getUserMessagesForForking / setSessionName | AgentSession.GetUserMessagesForForking / SetSessionName | codingagent/session.go |
| main.ts resolveSessionPath / createSessionManager | Main → selectHeadlessSession | codingagent/session_selection.go |

树数据和手工摘要写入由 SessionManager 承担。AgentSession.NavigateTree 和自动分支摘要编排继续归 M4，ExtensionHandler 的执行与扩展取消归 M7（ADR-0009）。Go context 取消和重绑失败通过 error 可观察；候选 Session 清理比固定 Pi 的失败路径更严格，见学习材料。

验证入口：`internal/parity/session_navigation_test.go`、`internal/parity/session_tree_test.go`、`codingagent/issue73_runtime_test.go`、`codingagent/issue73_session_test.go`、`cmd/pig/issue73_process_test.go`。API 快照：`codingagent/testdata/issue73_surface_golden.txt`。运行示例：`go run ./examples/session-navigation`。
