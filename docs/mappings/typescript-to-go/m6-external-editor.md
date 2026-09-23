# 外部编辑器映射

| 固定 Pi 基线 | Pig | 证据 |
| --- | --- | --- |
| SettingsManager.getExternalEditorCommand | 同名 SettingsManager.GetExternalEditorCommand | TestExternalEditorSelection123 |
| InteractiveMode.handleOpenExternalEditor | InteractiveMode.OpenExternalEditor；Run / GetUserInput 的 app.editor.external | external-editor-cli.json、真实 CLI PTY |
| editor.getExpandedText / setText | TextUI.EditExternally | TestExternalEditorExpandedDraft123 |
| editInExternalEditor | OpenExternalEditor 的私有临时文件操作 + ProcessTerminal.RunCommand | 初始草稿、字面 argv、Provider 内容、目录清理 |
| ui.stop / start / requestRender | 暂停标记、输入 generation、终端 Stop / Start、布局失效 | CLI 背景生成与全屏；SDK Stop/Cancel |
| inherited stdio | 注入 ProcessTerminal 的文件输入输出 | 公开 SDK PTY 与本机 Vim |

错误提示与取消清理采用 ADR-0041 的显式 Go 偏离。非零退出保留草稿与基线一致；不把 Pig 的新增提示或恢复能力伪称为 Pi fixture 的断言。API 增量记录在 `codingagent/testdata/issue123_surface_golden.txt`，权威证据条目为 `contract:codingagent/external-editor`。
