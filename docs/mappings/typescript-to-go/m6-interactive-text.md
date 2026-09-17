# M6.1 TypeScript → Go：Interactive 文本切片

| Pi 固定基线 | Pig | 本票行为 |
| --- | --- | --- |
| `coding-agent/src/main.ts` Interactive 路由 | `codingagent/misc.go` Main / runSessionMain | TTY 判断后共用资源与 Session 装配 |
| `modes/interactive/interactive-mode.ts` | `codingagent/interactive.go` InteractiveMode | Init、Run、Stop、输入与 AgentSession 流事件编排 |
| `tui/src/terminal.ts` ProcessTerminal | `tui/terminal.go`、`terminal_posix.go` | raw mode、可停止读取、尺寸变化、恢复；无 CGO |
| TUI + editor + transcript 的最小文本职责 | `tui/text.go` TextUI | 新增 Go 组合边界，基础输入与纯文本输出；完整 Pi 组件面仍保留独立 Stub |
| Interactive shutdown | InteractiveMode.Stop / ProcessTerminal.Stop | Ctrl+D、双 Ctrl+C、/quit、SIGTERM/SIGHUP 退出 0，SDK context 取消保留 Go error |

CLI 与 SDK 都复用 legacy AgentSession、v3 Session 和现有本地资源，不接入 Harness-v4。`RunHeadless` 作为既有 Session prompt 生命周期辅助函数复用，其 OnEvent 提供 text delta；Interactive 的终端逻辑不放入产品层。新增 Terminal 注入用于宿主组合和公开边界验收，没有改变既有 Terminal 接口的必需方法。

先读 [中文学习说明](../../learning/m6-interactive-text.md)，再从真实 PTY 测试 `cmd/pig/issue109_interactive_test.go` 跟到 SDK 与 tui。对等 fixture 保存可观察语义，不比较整屏品牌、颜色或逐字节 ANSI。
