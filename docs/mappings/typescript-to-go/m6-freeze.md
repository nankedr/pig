# M6 TypeScript → Go 冻结导航

Pi 基线：`936aff00918de1187f085f123c2812d8f2d67745`。源码/公开成员清单来自 `parity/surface/symbols.jsonl`，上游测试文件来自 `parity/inventory/files.jsonl`；对应 Go target、状态和证据在 `parity/catalog.jsonl`。`scripts/m6-audit.py` 将每个 M6 条目及证据文件摘要锁到 `internal/m6gate/testdata/catalog_scope.txt`，不自动晋升任何旧条目。

| Pi 公开边界 | Pig 入口 | 细分映射与对等证据 |
| --- | --- | --- |
| InteractiveMode 生命周期 | codingagent.InteractiveMode、tui.ProcessTerminal | [交互文本](m6-interactive-text.md)，interactive.json |
| Editor / Input / 历史 | tui.Editor、Input | [编辑器](m6-multiline-editor.md)，editor*.json |
| 键解析与 KeybindingsManager | tui / codingagent.KeybindingsManager | [按键](m6-terminal-keys.md)，keys*.json |
| 消息与 Tool 组件 | codingagent.Transcript | [文本渲染](m6-text-rendering.md)，text-rendering*.json |
| MainScreen / AltScreen 布局 | tui.RenderLayoutFrame、TextUI | [布局与滚动](m6-layout-scrolling.md)，layout-scrolling*.json |
| Select/Input/Confirm/Trust dialogs | tui.ShowSelectDialog、ProjectTrust | [对话框](m6-trust-dialog.md)，trust-dialog.json |
| AutocompleteProvider / 资源命令 | tui.CombinedAutocompleteProvider | [自动补全](m6-autocomplete.md)，autocomplete*.json |
| steering / follow-up / cancel | legacy AgentSession 队列、InteractiveMode | [队列](m6-interactive-queues.md)，queues*.json |
| model / thinking / scoped models | ModelRuntime、选择器 | [模型](m6-model-selection.md)，model-selection*.json |
| resume / SessionSelector | v3 SessionManager、选择器 | [Session](../../learning/m6-session-selection.md)，session-selection*.json |
| fork / tree / labels / summary | AgentSession、树/消息选择器 | [分支](m6-interactive-branches.md)，interactive-branches*.json |
| Theme / watch / preview | ThemeController、Theme | [主题](../../learning/m6-interactive-themes.md)，themes-cli.json |
| SettingsSelector | SettingsManager、SettingsSelectorComponent | [设置](../../learning/m6-interactive-settings.md)，settings-cli.json；逐项设置清单 parity/interactive-settings-inventory.json |
| `!` / `!!` / Bash component | AgentSession.ExecuteBash、BashExecutionComponent | [Bash](m6-interactive-bash.md)，bash-cli.json |
| external editor | InteractiveMode.OpenExternalEditor、ProcessTerminal.RunCommand | [外部编辑器](m6-external-editor.md)，external-editor-cli.json |
| compact / reload / session / export | AgentSession.Compact/Reload/GetSessionStats/ExportToHTML | [维护](m6-interactive-maintenance.md)，maintenance-cli.json |
| 同一会话组合 | CLI + examples/m6-workflow 公开 SDK | parity/terminal/m6-workflow.py → m6-workflow-{regular,fullscreen}.json → internal/m6gate/workflow_test.go |

SDK 示例直接创建共享 Session runtime 和 InteractiveMode；Headless 命名的工厂只负责装配，不调用 CLI 或复制交互实现。无新增公开 SDK 类型；既有所有 API snapshots 随审计和全量测试验证。

终端输出和副作用证据、未完成范围及发布门禁见 [M6 冻结](../../learning/m6-freeze.md)。人工记录另见 [验收步骤](../../learning/m6-manual-acceptance.md)，不由自动回放生成。
