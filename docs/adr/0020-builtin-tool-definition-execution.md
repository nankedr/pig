# 开放内建 Tool definition 执行边界

Issue #78 要求 write 的 Tool 与 definition 入口均可真实执行。`ToolDefinition.Execute` 从不可执行的 `ExtensionHandler` 改为 `ToolExecuteFunc`：接收 `context.Context`、call ID、JSON 参数及已有 Agent update callback，返回已有 Agent ToolResult/error。`CreateWriteTool` 使用同一个 definition 执行体，并由 Agent dispatcher 完成参数校验。直接调用 definition 时必须提供字符串 `path` 和 `content`。

这项决策取代早期“所有 definition 执行槽均到 M7 才开放”的 Stub 范围，不改变 ADR-0009 的扩展宿主决策门禁：不传入 ExtensionContext，不实现扩展加载、跨进程调用或渲染回调。其余 extension 操作槽继续使用 opaque carrier。

Go 沿用已有 Agent 的 nil Details → ToolResult `details: null` 映射；Pi 的 undefined details 不序列化。write Oracle 的 `detailsEmpty` 投影比较是否没有 details payload，其余内容、路径和成功文案均精确比较。
