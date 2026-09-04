# Proxy 流按需重建不可变快照并拒绝非法协议

Issue #69 沿用 ADR-0006：接收端立即验证精简事件并维护 Stream Outcome，FIFO 只排入精简事件，消费方调用 `Next` 时才重建该事件的不可变 partial；`Result` 独立完成并可并发重复读取，不共享 Pi 的可变消息引用。`ProxyStreamOptions.Fetch` 是不进入请求 JSON 的 Go transport 注入点，`AssistantMessageEventStream()` 将消费端包装接入现有 Agent SDK；请求保持固定 Pi 的 `/api/stream`、Bearer 认证及所有可序列化选项，`maxRetryDelayMs` 交给远端 Provider，本地不重试生成请求。

按 issue 的协议要求，Pig 对未知 discriminator、非法索引、类型不匹配、重复 start/end、ToolCall 身份变化、非法嵌套值和缺失 terminal 的 EOF 生成保留已有内容的 error Outcome；这些输入在固定 Pi 中可能被忽略、覆盖或永不完成。合法流的按事件快照和 terminal 由固定 Pi Oracle 验证，投影仅去掉时间戳与 Pi 私有的 `partialJson` 缓冲字段；异常输入由公开 SDK 测试验证，不宣称与 Pi 的宽松行为相同。请求 context 或 `Signal` 取消都会中止 transport，已接收内容进入 aborted Outcome；注入的 Fetch 必须响应 context，BodyReader.Close 必须释放阻塞读取。
