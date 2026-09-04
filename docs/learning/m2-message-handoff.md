# M2.7 跨 Provider 消息 handoff

Legacy Agent 的历史记录保留每条 Assistant 的 `provider`、`api`、`model` 与原始 signature。`SetModel` 改变下一次调用使用的模型；Chat Completions 适配器在构造请求时调用 `ai.TransformMessages`，不把转换后的内容写回历史，因此可以再次切回原模型。

`TransformMessages(messages, model, normalizer...)` 返回独立的消息快照与 error。自定义适配器可以提供 `ToolCallIDNormalizer`；回调收到原始 ID、目标 Model 和来源 Assistant 的独立快照。同模型由 Provider、API、Model ID 三者同时相等判定，同模型不会调用 normalizer；未提供回调时 ID 原样保留。

转换分为两步：

1. 保留同模型的非空 thinking、带签名的空 thinking 和 redacted thinking，以及 text/thinking/Tool thought signature。跨模型把非空、未 redacted 的 thinking 转为 text，移除 text signature 和非空 Tool thought signature，丢弃 redacted/空 thinking。固定 Pi 保留空字符串/null 的 Tool thought signature，它们不携带可重放签名。
2. 维护本次转换的 ID 映射，后续 ToolResult 复用相同 ID。在下一条 Assistant、User 或 transcript 结束前，为仍无结果的 ToolCall 插入 `isError: true`、正文 `No result provided` 的 ToolResult。error/aborted Assistant 会触发前一轮补全，但自身不进入重放上下文，也不产生自己的合成结果。

Chat Completions 的目标约束继续由适配器决定：Responses 的 `call_id|item_id` 两段分别清洗；合并后超长则使用稳定短 hash，保留不同 item 的区分。OpenAI 目标的普通 ID 按 40 个 UTF-16 单元截断；其他 Provider 不截断普通 ID。自定义 Anthropic normalizer 的 64 字符约束由 Oracle 用例验证，不代表 Anthropic 网络适配器已经实现。

Go SDK 中的 nil Assistant/ToolResult content 和零值 User content 规范化为非 nil 空内容。旧 compact string User content 保持字符串；strict JSON codec 的输入校验保持不变。整个 nil Message 仍返回错误。图片可以为视觉模型原样传递；对不支持图片的模型，图片降级返回 `ErrNotImplemented`，归属 M12。Chat Completions 的图片 wire 分支及原有错误操作名保持其 M12 边界。

离线示例：

```sh
go run ./examples/message-handoff
```

示例先运行 Faux，再切换到带本地 Fetch 的 Chat Completions 目标，打印实际重放请求、原始 thinking signature 和最终回答，不需要网络或密钥。

对等证据来自锁定 Pi commit `936aff00918de1187f085f123c2812d8f2d67745`：

```sh
node --experimental-strip-types parity/oracle/message-handoff.mjs .upstream/pi --check
go test ./internal/parity -run '^TestMessageHandoffParity$' -count=1
go test ./ai ./agent -run 'TestTransformMessages|TestIssue66Locked|TestAgentModelHandoff' -count=1
```

19 个场景覆盖同/跨模型签名、ID 回调来源、完整与缺失 ToolResult、失败 Assistant、nil/旧 content、长 ID 与 Chat Completions wire。fixture 只把合成结果的时钟值投影为 0；真实测试另外检查时间戳范围。Go SDK 用 nil/零值表达 Pi 的缺失/null content，快照隔离延续 ADR-0006。完成度以 Parity Catalog 为准。
