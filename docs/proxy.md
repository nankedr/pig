# Legacy Agent proxy

`agent.StreamProxy` 向 `ProxyURL + "/api/stream"` 发送 POST，请求头使用 `Authorization: Bearer <AuthToken>`，JSON 包含 `model`、`context`、`options`。`Headers` 是传给远端 Provider 的选项；proxy 认证由 `AuthToken` 提供。

```go
stream := agent.StreamProxy(ctx, model, input, agent.ProxyStreamOptions{
    ProxyURL: "https://proxy.example.com",
    AuthToken: token,
    Reasoning: options.Reasoning,
    ThinkingBudgets: options.ThinkingBudgets,
    Transport: options.Transport,
    SessionID: options.SessionID,
    MaxRetryDelayMS: options.MaxRetryDelayMS,
})
```

在 `AgentOptions.StreamFunction` 中返回 `stream.AssistantMessageEventStream()` 即可接入 Agent。`Fetch` 可注入 `ai.FetchFunction`；省略时使用标准 HTTP client。`Fetch` 必须遵守传入 context，返回的 `BodyReader.Close` 必须解除 `Read` 阻塞；`BodyReader` 优先于缓冲的 `Body`。

服务端每行发送一个 `data: <JSON>` 精简事件，使用 `agent.MarshalProxyAssistantMessageEvent` 编码；无需发送累计 Assistant Message。文本、thinking、ToolCall 索引从 0 连续创建，delta/end 必须引用尚未结束且类型一致的 block；ToolCall end 的 ID、名称必须与 start 一致。流以 `done` 或 `error` 结束，usage 保留服务端的计数与费用。

`Next` 按 FIFO 消费事件，partial 为该时刻的独立快照；仅调用 `Result` 不会构建未消费的 partial。多个 `Next` 读取者分摊同一个队列，多个 `Result` 调用者得到独立的相同最终值。HTTP、认证、协议和远端失败通过 `StopReason=error` 表达；生成取消通过 `StopReason=aborted` 表达，均保留已收到内容。单个 `Next` / `Result` 等待 context 取消只结束该次等待，生成请求的 context 或 `ProxyStreamOptions.Signal` 取消才会终止 transport。
