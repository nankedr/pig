# Legacy Agent 的运行中投递与结束 barrier

Issue #68 要求 idle steer/follow-up 明确报错、非法调用不污染状态、wait-idle 等待全部结束 listener。这对固定 Pi `936aff00918de1187f085f123c2812d8f2d67745` 的宽松入队与 listener 抛错处理构成明确偏离，按该 issue 的验收要求采用。

Pig 仅在未取消、仍可接收消息的活动 run 中允许 Steer/FollowUp。正常结束的最终空队列检查与停止接收在同一锁内；`agent_end` 开始后保持 busy，但拒绝新投递。取消或 ShouldStopAfterTurn 前已经接受而未消费的消息继续保留，可通过 Continue 恢复，或由 idle Reset 清除。

结束 listener 按注册顺序全部执行，以 `errors.Join` 返回错误；不会因为一个结束 listener 失败再次制造 Assistant 失败消息或重复 `agent_end`。其他事件的 listener 错误仍按既有 Go 契约直接传播。该行为延续 ADR-0006 的 partial outcome 与运行结局边界。

两条队列的 FIFO、one/all、工具/steering 优先、follow-up 的下一 turn 以及 Continue 从 assistant 尾部取队列的行为保持基线语义。共同 fixture 证明这些相同部分；`legacy-agent-queues-deviation.json` 单独记录 Pi 的 idle/结束入队和 listener 失败行为，Go 测试分别断言偏离结果，不通过 normalization 隐藏差异。

Issue #85 将同一 admission 决策接到 AgentSession；独立 `session-messages-deviation.json` 通过固定 Pi 的公开 SDK 记录 idle、取消后、agent_end 和 agent_settled 的宽松入队结果。Go 的 Session 不在拒绝后写展示队列或发送成功队列事件。Session 的 ClearQueue 保留已发布的 error-only 签名，清空前内容可通过独立快照查询。

Session 用本次用户消息的单调毫秒时间戳与文本识别消费，避免固定 Pi 按非空文本查找时的空文本残留，以及相同文本在新 Prompt 和保留队列之间误删展示项。时间戳是生成的身份，不要求与 Pi 的 Date.now 字节一致；内容、消费顺序与已消费历史仍须相同。该调整服务于 #85 的并发与历史一致性要求，不改变 Legacy Agent 调度。

Pi 的 sendUserMessage 返回 Promise，Go 保留已发布的同步 error 签名。为避免 idle queue_update 回调启动新运行时自等待，或多 listener 通知倒序，队列通知在途时 Prompt/SendUserMessage 启动新运行明确拒绝。已有活动运行的 steer/follow-up 不受此限制；新运行应在通知调用（如 ClearQueue）返回后开始。这是同步 Go API 的回调重入边界，由公开 SDK 回归测试固定。
