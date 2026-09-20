# M6.7 补全映射

| 固定 Pi 边界 | Pig 公开边界 | 可观察语义 |
| --- | --- | --- |
| `CombinedAutocompleteProvider` | `tui.NewCombinedAutocompleteProvider` | 命令模糊过滤、参数回调、路径前缀与 fd 搜索 |
| `getSuggestions(..., {signal,force})` | `GetSuggestions(context.Context, ..., AutocompleteOptions)` | bool 区分无结果，context 取消 |
| `applyCompletion` | `ApplyCompletion` | 保留未替换文本；Go 用 UTF-8 字节列 |
| `Editor.setAutocompleteProvider` | `Editor.SetAutocompleteProvider` | goroutine 只返回快照结果，宿主串行发布 |
| InteractiveMode 资源装配 | `codingagent.NewSessionAutocompleteProvider` | 读取现有授权资源与 settings；不单独发现资源 |
| 选择后的交互提交 | `InteractiveMode` → `AgentSession.Prompt` | 原有模板/Skill 展开与 v3 Session；文件引用保留文本 |

证据、运行方法和 partial 说明见 [学习说明](../../learning/m6-autocomplete.md)。没有新增模型、认证、扩展 ABI 或包生态能力。
