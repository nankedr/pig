# M6.4 文本消息与 Tool 执行视图

对应 [#112](https://github.com/nankedr/pig/issues/112)，对等基线固定为 Pi `936aff00918de1187f085f123c2812d8f2d67745`。

```sh
go run ./examples/text-rendering
go test ./tui ./codingagent -run '112' -count=1
go test ./cmd/pig -run 'TestPig.*112' -count=1
```

交互式 CLI 使用原有 legacy AgentSession 与 v3 Session。生成期间可阅读 Markdown、thinking、Tool 参数增量和执行更新。Ctrl+O 展开／折叠 Tool 输出，Ctrl+T 切换 thinking 显示；支持 keybindings.json 重绑，生成过程中也立即生效。thinking 初始可见性读取 hideThinkingBlock 设置。隐藏时显示 Thinking...，不删除消息内容。

公开 `codingagent.Transcript` 将消息和 Session 事件投影为可渲染组件。`SetMessages` 恢复历史，`Update` 接收实时事件，`Render(width)` 只生成终端行。它没有修改 Session 或模型输入的权限。ToolCall 先按 Assistant 内容顺序占位，执行事件按 ID 更新已有位置，最终结果之后的更新被忽略。ToolResult 消息不会重复显示同一个调用。摘要、分支摘要、可显示的 CustomMessage 与 bash 消息在同一历史路径渲染。

`AssistantMessageComponent`、`UserMessageComponent` 和 `ToolExecutionComponent` 可独立用于 SDK。Assistant 合并连续 thinking 块，展示已产生的部分正文，再显示错误、取消或长度截断。Tool 默认显示至多十个输出行，展开后展示全文。显示状态不进入模型上下文。

`tui.Markdown` 使用锁定的纯 Go Goldmark 解析器，支持标题、强调、代码、链接回退 URL、引用、列表及嵌套列表。正文和 Tool 输出先移除光标移动、OSC 剪贴板等控制指令，仅保留 SGR 样式；按字素宽度换行。代码 tab 与 diff tab 转为三个空格。`HighlightCode` 回调由调用者可选提供。

## 证据与边界

`parity/oracle/fixtures/text-rendering.json` 来自 Pi 公开组件，固定 Markdown 和 Assistant 文本行；CLI fixture 来自真实 Pi 子进程、PTY 与分阶段放行的回环 SSE 服务。Go 普通验收只读已锁定 fixture，不需要 Node、Pi 或在线 Provider。CLI 测试还比较生成结束与重新打开 Session 后的最终屏幕，并覆盖窄窗口、长输出、错误、截断和取消。

```sh
node parity/oracle/text-rendering.mjs /path/to/locked-pi --check
node parity/oracle/text-rendering-cli.mjs /path/to/locked-pi --check
```

对等目录保留 partial：内置 Tool 专用布局、精确配色、语法高亮、diff 词级反色、表格／LaTeX／Mermaid、复杂 Markdown 增量解析、OSC 超链接和 shell integration 标记、滚动历史及完整 TUI 布局没有声明完整对等。摘要类 Pi 独立组件仍为 Stub，当前通过 Transcript 提供文本显示。图片输出仅为文字占位，M12 才实现图片；扩展自定义渲染与运行时属于 M7。六平台终端验收属于 M13，#99 包生态不在本票范围。
