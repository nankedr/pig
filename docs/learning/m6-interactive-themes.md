# M6.12：会话主题切换与更新

Issue #120。固定对等基线为 `936aff00918de1187f085f123c2812d8f2d67745`。

## 使用

运行交互 CLI，输入 `/settings`，选择 Theme。上下移动立即预览，Enter 确认并保存，Esc 取消。Automatic 中分别选择 Light theme、Dark theme，Apply 保存 `light/dark`；取消恢复原设置。内置 dark/light 与 M5 已加载本地主题采用同一资源来源规则。

编辑当前本地主题 JSON，约 100ms 后历史、新消息、Tool、编辑器和对话框重新着色。无效、删除或不可读文件保留最近有效主题并反馈 warning；恢复文件后继续更新。不会重建编辑器或 Session。`/reload` 可重新发现已授权资源；新 Session/恢复 Session 同样重绑主题资源。

COLORTERM 的 truecolor/24bit 及已知终端标识决定真彩色；不支持时使用 256 色。自动模式启用终端 2031 通知，997 报告改变当前明暗主题；启动时同时查询 996 和 OSC 11，前者优先。没有 theme 设置时用 OSC 11/COLORFGBG 探测，高置信度结果才保存。

## SDK

`NewThemeController(settings, loaded)` 构造实例；`ApplySettings` 应用设置，`Preview` 只预览，`SetTheme` 确认保存，`Current` 返回只读快照。`Refresh` 执行一次文件检查；`Watch(ctx, changed)` 阻塞到取消，调用方等待它返回即可确认定时器释放。`ReplaceResources` 切换资源与设置，旧文件不再被检查。将 `Current()` 传入 `Transcript.SetTheme` 可重新渲染已有消息。

`go run ./examples/interactive-themes` 是无需网络的最小示例。Go 调用方不得修改 Current 返回的快照或已传入的 Theme 值。

## 证据与范围

- `theme-runtime.json`：固定 Pi 公开解析器和 ThemeSelector 的输入、输出。
- `themes-cli.json`：真实 Pi CLI 子进程 PTY，在 truecolor/256color 下修改本地主题、预览取消、保存并检查草稿/历史与终端恢复。Pig 使用同一 harness；settings 顶层菜单定位按现有 UI 做适配。
- `issue120_themes_test.go`：公开 SDK、文件失败恢复、重复切换和 Watch 取消。
- `issue120_interactive_test.go`：公开 InteractiveMode 加 PTY，检查 scheme 优先级、通知改变草稿/对话框颜色、固定转自动时重新探测、主题失效的 Session 替换及退出清理。

执行 `go test -race ./codingagent -run '120'`、`PIG_TEST_RACE=1 go test -race ./cmd/pig -run 120`。Oracle 用 `make m6-themes-oracle PIG_PI_ORACLE_CHECKOUT=/path/to/locked/pi` 复核，普通测试只读锁定 fixture，离线运行。

目录条目 `contract:codingagent/interactive-themes` 保持 partial：精确菜单布局/搜索、语法高亮和完整 Markdown 样式未宣称对等。无 CGO 轮询与监听范围偏离见 ADR-0038；包生态 #99、扩展 M7、认证 M11、图片 M12、六平台 M13 不在本票内。
