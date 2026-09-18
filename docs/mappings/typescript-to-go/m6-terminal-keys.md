# M6.3 源码映射

固定 Pi `936aff00918de1187f085f123c2812d8f2d67745`，权威证据为 `contract:tui/terminal-keys`。范围与平台限制见[学习说明](../../learning/m6-terminal-keys.md)。

| Pi | Pig 公开边界 | 证据 |
| --- | --- | --- |
| `tui/src/keys.ts` | `tui.ParseKey`、`MatchesKey`、`DecodePrintableKey`、事件类型查询 | `keys.json`、`TestKeyParity111` |
| `tui/src/stdin-buffer.ts` | `tui.StdinBuffer`、`SetHandlers` | `TestStdinBufferParity111`、UTF-8 分片/超时/清理测试 |
| `tui/src/keybindings.ts` | `tui.KeybindingsManager`、`EditorOptions.Keybindings` | `TestKeybindingsParity111`、`TestEditorKeybindings111` |
| `tui/src/terminal.ts` | `tui.ProcessTerminal`、协商与 Shift+Enter 归一化函数 | `TestTerminalNegotiationRestart111`、`TestTerminalSpecialCases111` |
| `tui/src/native-modifiers.ts`、Darwin helper | `tui.IsNativeModifierPressed` | macOS CoreGraphics + purego；CGO-disabled build；其他平台留至 M13 |
| `coding-agent/src/core/keybindings.ts` | `codingagent.KeybindingsManager` | `TestKeybindingsFile111`、真实 CLI 配置 fixture |
| `coding-agent/src/modes/interactive/interactive-mode.ts` | `InteractiveModeOptions.Keybindings`、`tui.TextUI.ReadInput` | `keys-cli.json`、中断请求及真实暂停恢复测试 |

输入计时器由 StdinBuffer 所有，读取循环由 ProcessTerminal 所有；TextUI 只处理完整输入事件。应用级动作复用既有 AgentSessionRuntime，不新增会话持久化或模型执行路径。
