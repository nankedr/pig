# M4.3：通过 Session 投递和排队消息

`CreateAgentSession` 返回的公开 SDK 现在支持文本 `SendUserMessage`、`Steer`、`FollowUp`、队列模式、查询和清空。底层 FIFO、steering 优先级、工具执行时机和 one/all 消费仍由 Legacy Agent 管理，Session 只同步队列展示、事件与既有消息持久化。

```sh
go run ./examples/session-messages
go test -race ./codingagent -run '^TestSessionMessages' -count=20 -shuffle=on
node --experimental-strip-types parity/oracle/session-messages.mjs .upstream/pi --check
```

| 调用 | 空闲 | 生成中 | 取消后或收尾中 |
| --- | --- | --- | --- |
| `SendUserMessage(content)` | 开始一轮并等待结束 | 报错，要求指定 DeliverAs | 活动轮次尚未结束时拒绝 |
| `SendUserMessage(content, {DeliverAs: steer/followUp})` | 开始一轮；投递方式只影响运行中行为 | 交给对应 Legacy Agent 队列 | 拒绝 |
| `Prompt(ctx, text, {StreamingBehavior: steer/followUp})` | 开始可取消的轮次 | 交给对应队列 | 拒绝 |
| `Steer(text)` / `FollowUp(text)` | 拒绝 | 在 Legacy Agent 仍接收消息时入队 | 拒绝 |

Go 中选项使用 `codingagent.SendUserMessageOptions` 和 `codingagent.PromptOptions` 结构体。`SendUserMessage` 保留已发布的无 context 签名：空闲时同步等待本轮，取消用 `Abort`，结束屏障用 `WaitForIdle(ctx)`；需要上游 context 时用 `Prompt`。运行中的投递只确认接收，不等待消费，也不会持久化未消费消息。

`SendUserMessage` 将多个文本块以换行合为一个文本块，并把 `/...` 当作字面文本。`PromptOptions.ExpandPromptTemplates=false` 明确关闭展开；true、图片、Source 与 PreflightResult 仍返回精确的结构化未实现错误。`Steer`/`FollowUp` 当前只有文本签名；模板、skill command、扩展输入处理与扩展命令执行由后续阶段提供，不能据此宣称扩展 API 已可运行。

`SetSteeringMode` 与 `SetFollowUpMode` 同时更新 SettingsManager 和 Legacy Agent。无效模式拒绝且不改设置。查询返回独立切片，队列事件为每个 listener 分别复制切片；并发入队的队列通知按接收顺序发布。listener 可查询、向已有运行投递、清空和调用 Abort。队列通知在途时，启动新运行会明确报错，防止同步回调自等待及通知倒序；应在 ClearQueue 返回后再调用 Prompt/SendUserMessage 开始新运行。同样不能在 callback 内等待当前运行结束；需等待时从其他 goroutine 调用。

消费在用户 `message_start` 前更新队列并发送 `queue_update`，`message_end` 使用既有 AppendMessage 写入历史。`agent_settled` 与 `WaitForIdle` 位于历史追加之后。未消费消息在取消后留在内存，可通过后续 Prompt/SendUserMessage 继续消费或 ClearQueue 丢弃；关闭进程不会保存队列。已消费消息重开后完整恢复，旧历史不会重复追加。ClearQueue 清除尚在 Legacy Agent 队列中的消息，不撤回已经开始消费的消息；Go 保留原有 error-only 签名，不返回 Pi 的清空前队列对象。不要混用 `Session.Agent()` 上的直接排队/重置操作来维护 Session 的队列展示。

固定 Pi 基线的 `agent-session.ts` 和 `agent-session-concurrent.test.ts` 定义参考行为。共同 fixture 覆盖四种模式组合、清空、文本归一化、运行中拒绝缺省方式、模型输入、事件顺序和 Session 重开；独立 deviation fixture 保存 Pi 在 idle、取消后和结束阶段仍接受入队的行为。Pig 遵循 ADR-0018 的既有拒绝语义，不通过归一化隐藏此差异。同文本及空文本的并发消费按本次消息身份匹配，避免基线按文本查找造成的显示残留；生成的用户时间戳作为本次身份，在同 Session 内单调递增。

证据归属 `contract:codingagent/session-messages`，状态是 partial：本切片未开放图片、模板展开、扩展 runtime、压缩期间的重试队列、RPC 或交互 UI。
