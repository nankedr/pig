# M6.2 编辑器映射

固定 Pi `packages/tui/src/components/editor.ts` → `tui.Editor`，`wordWrapLine` → `tui.WordWrapLine`。
输入状态、历史、undo snapshot、kill ring、paste registry 属于编辑器；`tui.TextUI` 只拆分终端输入、同步编辑器和摆放完整屏幕中的光标。

Go 保留既有公开 Editor 接口与可选能力接口，`EditorCursor.Col`、`TextChunk.StartIndex/EndIndex` 使用 UTF-8 字节偏移。Pi Oracle 在采集时从 UTF-16 索引转换，不通过修改预期文本掩盖行为差异。视觉上下导航沿用 Pi 的 UTF-16 粘性列选择，再对齐到字素/占位符边界；终端显示列以字素宽度计算。

`Editor` 不拥有终端生命周期；同步回调由宿主串行调度。`TextUI.AddToHistory` 允许 InteractiveMode 将恢复的 v3 用户消息加入输入历史。完整 SDK 表面由 API snapshot 锁定。未支持的分支、公开行为用例和运行证据以 `contract:tui/multiline-editor` 为准。
