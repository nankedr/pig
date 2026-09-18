# M6.4 TypeScript → Go

| 固定 Pi 边界 | Pig 边界 | 映射 |
| --- | --- | --- |
| InteractiveMode 事件与历史组件编排 | InteractiveMode + Transcript | 同一消息投影；Tool ID 定位、原始调用顺序、终态保护 |
| AssistantMessageComponent | NewAssistantMessageComponent / UpdateContent / Render | thinking 分组、Markdown、错误与部分输出 |
| UserMessageComponent | NewUserMessageComponent / Render | 保留用户列表标记和反斜杠转义 |
| ToolExecutionComponent | NewToolExecutionComponent | 参数更新、执行状态、结果、展开／折叠；专用布局 partial |
| Markdown | tui.NewMarkdown | 纯 Go 解析、主题回调、按显示宽度换行 |
| utils.ts | VisibleWidth / WrapTextWithANSI / TruncateToWidth | SGR 与字素宽度；控制序列安全过滤 |
| renderDiff | codingagent.RenderDiff | 加／删／上下文文本着色；词级反色仍 partial |

普通验收从 CLI 真实进程和 SDK 公开包出发。详见 [中文学习说明](../../learning/m6-text-rendering.md) 与 `contract:codingagent/text-rendering`。颜色字节、框线和品牌不作为语义对等判据。
