# M6.11：从历史消息 fork 与 Session 树导航

对应 #119，固定 Pi `936aff00918de1187f085f123c2812d8f2d67745`。沿用 legacy AgentSession、v3 Session 和本地资源，不新增 RPC 命令。

## 在终端中使用

- `/fork` 打开历史用户消息选择器，按 Session 文件中的历史排列，默认选中最后一条用户消息。上下移动支持首尾循环，Enter 创建独立 Session，Esc 取消。原 Session 不变；新 Session 只复制所选消息之前的祖先链，所选文本回到编辑器，确认或修改后才生成。
- `/tree` 浏览整个 Session 树，活动分支优先排列，展示分支连接、活动路径、当前 leaf 和标签。直接输入按词搜索，Esc 首次清空搜索，再次退出。上下选择，左右或 PageUp/PageDown 翻页。
- Ctrl+Left / Alt+Left 折叠分支或沿祖先移动，Ctrl+Right / Alt+Right 展开或沿子节点移动。Ctrl+D 默认视图，Ctrl+T 隐藏工具结果，Ctrl+U 只看用户消息，Ctrl+L 只看标签，Ctrl+A 全部，Ctrl+O / Ctrl+Shift+O 循环过滤。快捷键通过既有 keybindings.json 修改。
- Shift+L 编辑所选节点标签，清空后保存可删除标签，Esc 放弃。标签使用既有 v3 label 条目保存。Shift+T 显示标签时间。
- 选择目标后可跳过摘要、生成摘要或提供自定义摘要提示。取消摘要选择会回到同一树节点；取消自定义提示会回到摘要选择。`branchSummary.skipPrompt=true` 直接使用无摘要导航。摘要期间 Esc 取消，已输入的草稿仍可继续使用。

选择用户消息会导航到其父节点，将该消息作为草稿；已有非空草稿优先保留。选择其他节点则导航到该节点。无摘要导航只改变内存 leaf，后续 Prompt 或 label 写入后，正式 reader 才会沿新 parentId 链重开；原分支一直保留。这是 ADR-0027 的既有行为。

只打开或取消选择器不会停止生成。确认导航后恢复排队消息到编辑器，取消并等待旧 Prompt worker 收尾，再调用 AgentSession.NavigateTree。摘要失败、取消或非法节点不会提交路径。标签写入失败留在编辑对话框，显示错误；生成中标签编辑明确报 busy。

## SDK 与验证

```sh
go run ./examples/interactive-branches
go test ./codingagent -run '119|TestSessionTreeNavigationFailuresAreAtomic' -count=1
go test ./cmd/pig -run TestPigInteractiveBranchesParity119 -count=1
go test -race ./codingagent -run 'TestInteractive.*119' -count=1
make m6-branches-oracle PIG_PI_ORACLE_CHECKOUT=/path/to/locked-pi
```

公开构造器为 `NewUserMessageSelectorComponent` 与 `NewTreeSelectorComponent`，由回调返回选择。实际 Session 操作继续使用 `AgentSessionRuntime.Fork`、`AgentSession.NavigateTree` 和 `SessionManager.AppendLabelChange`。示例使用 Faux，写入独立临时目录并通过正式 reader 重开。

Oracle 包括公开选择器的过滤/导航/标签/复制回调 fixture，以及真实 CLI 子进程 + PTY + loopback SSE 的完整流程。CLI fixture 比较每次 Provider 的用户和 assistant 上下文、源文件未修改、fork 父文件、完整历史、新活动路径、标签、摘要和终端恢复。普通 Go 测试只重放锁定 fixture，不需要 Pi、Node、联网或真实凭证；CLI harness 需要本地 Python 3。

## 明确边界

树的精确颜色、像素布局、横向自动平移、工具调用参数格式、标签时间的本地化显示尚未证明对等。SDK 空历史选择器不实现 Pi 的 100ms 自动关闭；CLI 空历史会直接提示。系统剪贴板复制沿用现有 Capability Stub，树内复制回调可由 SDK 注入，CLI 明确显示错误。

M7 扩展 hook、M11 完整认证、M12 图片、M13 六平台运行验收和 #99 包生态继续保持既定边界，Catalog 状态保留 partial。
