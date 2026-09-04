# M2.8 上下文估算与 overflow 判定

`ai.EstimateContextTokens(input)` 返回总估算、可信 usage、后续估算和 `LastUsageIndex`。只估算消息时传 `ai.Context{Messages: messages}`；索引从零开始，没有可信 usage 时为 nil，JSON 中为 null。

函数按 transcript 顺序寻找 Assistant：时间戳必须不早于它前面的任何消息，终态不能是 error 或 aborted，usage 必须大于零。优先取非零 `TotalTokens`，否则累加 input、output、cache read、cache write。新插入的较晚时间戳摘要会使后面的旧响应 usage 失效；对新前缀生成的响应可重新成为可信基准。时间戳无法识别保留原时间戳的原地编辑，调用方修改前缀时需相应更新时间戳。

有可信 usage 时只估算该 Assistant 之后的消息，再从这些消息的 `AddedToolNames` 中选择当前 `Context.Tools` 定义补入。重复标记通过名称集合合并，已移除的定义不计入；最新 usage 之前的发现标记不再补算。定义顺序和同名定义按当前 `Context.Tools` 保留，不额外做 deferred 拆分或名称正规化。没有可信 usage 时，估算 system prompt、所有消息和全部 Tool 定义。

文本长度按 JavaScript UTF-16 code unit 计数，每四个字符估算一个 token 并向上取整；每张图片固定估算 1200 token。每条消息汇总内容后取整，包括 thinking 文本、Tool 名和参数 JSON、ToolResult 的文本与图片。Tool 定义数组作为一个 JSON 整体取整；JSON Schema 的空白、HTML 转义和 Unicode 分隔符不会额外增加估算。signature、ToolResult details 和 ToolResult usage 不计入。不可序列化值使用快照的 `[unserializable]` 兜底。

`ai.IsContextOverflow(message, contextWindow...)` 保留固定 Pi 的三个判据：

- error 终态匹配 25 个 Provider 错误模式，先排除 `Throttling error:`、`Service unavailable:` 前缀以及 rate limit、too many requests。
- stop 终态的 input + cache read **严格超过**窗口。此判据不包含 output、cache write 或 totalTokens。
- length 终态、output 为零且 input + cache read **达到窗口的 99%**。

省略窗口或传零时仅检测错误文本。`ai.GetOverflowPatterns()` 返回可独立修改的 Go regexp 副本。LiteLLM 包装的服务错误若仍携带明确的上下文超限文本，按快照仍归为 overflow；静默截断而没有上述 usage 信号时无法判定。

`ai.IsRecoverableLength(message, desiredMaxOutput)` 检查 length 终态的 output 是否小于正的原始目标输出上限。这个上限必须在上下文裁剪之前保存：请求从 100 裁剪到 99，生成 99 后仍可能可恢复。OpenAI 简单请求现使用统一估算器进行原有的输出上限裁剪；本切片提供判定，不自动执行压缩或重试，相关策略由 M4 接入。

离线运行：

```sh
go run ./examples/context-overflow
go test ./ai -run 'TestContext|TestOverflowPatterns|TestIssue67' -count=1
go test ./internal/parity -run '^TestContextOverflowParity$' -count=1
node --experimental-strip-types parity/oracle/context-overflow.mjs .upstream/pi --check
```

示例经 Faux Provider 与 `Models.Complete` 演示估算、静默超限、零输出 length 和错误检测。Oracle fixture 来自锁定 Pi commit `936aff00918de1187f085f123c2812d8f2d67745`，Go 从公开 SDK 重放；Catalog 绑定输入及观察 hash。Go 补充测试验证指针消息、nil、输入不变性、regexp 副本和原始输出上限。
