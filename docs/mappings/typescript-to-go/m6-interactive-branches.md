# M6.11 TypeScript → Go

固定 Pi commit：`936aff00918de1187f085f123c2812d8f2d67745`。

| Pi 公开边界 | Pig 实现 | 验证 |
| --- | --- | --- |
| `UserMessageSelectorComponent` | `codingagent/user_message_selector.go`，使用已有 ForkMessage | `fork-selector.json` + `TestForkSelectorParity119` |
| `TreeSelectorComponent` | `codingagent/tree_selector.go`，TreeSelectorOptions 注入键绑定与持久化回调 | `tree-selector.json` + `TestTreeSelectorParity119` |
| `InteractiveMode.showUserMessageSelector/showTreeSelector` | `codingagent/interactive_branches.go` | `interactive-branches-cli.json`，真实 PTY 与 Provider 上下文比较 |
| `AgentSessionRuntime.fork` | 复用 `codingagent/session_runtime.go` | `TestBranchSelectionRuntime119`，正式 reader 重开 |
| `AgentSession.navigateTree` | 复用 `codingagent/session_tree_navigation.go`、`session_branch_summary.go` | `TestInteractiveBranchSummaryRecovery119`、`TestInteractiveTreeStreamingCancellation119` |
| editor 草稿恢复 | `tui.TextUI.SetEditorText` | 原子保留/替换草稿，API snapshot 与终端路径覆盖 |

权威记录为 `contract:codingagent/interactive-branches`；精确视觉布局、系统剪贴板与后续里程碑分支保持 partial/Stub。决策见 ADR-0037，学习流程见 `docs/learning/m6-interactive-branches.md`。
