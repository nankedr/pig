# Legacy Agent 的运行中投递与结束 barrier

Issue #68 要求 idle steer/follow-up 明确报错、非法调用不污染状态、wait-idle 等待全部结束 listener。这对固定 Pi `936aff00918de1187f085f123c2812d8f2d67745` 的宽松入队与 listener 抛错处理构成明确偏离，按该 issue 的验收要求采用。

Pig 仅在未取消、仍可接收消息的活动 run 中允许 Steer/FollowUp。正常结束的最终空队列检查与停止接收在同一锁内；`agent_end` 开始后保持 busy，但拒绝新投递。取消或 ShouldStopAfterTurn 前已经接受而未消费的消息继续保留，可通过 Continue 恢复，或由 idle Reset 清除。

结束 listener 按注册顺序全部执行，以 `errors.Join` 返回错误；不会因为一个结束 listener 失败再次制造 Assistant 失败消息或重复 `agent_end`。其他事件的 listener 错误仍按既有 Go 契约直接传播。该行为延续 ADR-0006 的 partial outcome 与运行结局边界。

两条队列的 FIFO、one/all、工具/steering 优先、follow-up 的下一 turn 以及 Continue 从 assistant 尾部取队列的行为保持基线语义。共同 fixture 证明这些相同部分；`legacy-agent-queues-deviation.json` 单独记录 Pi 的 idle/结束入队和 listener 失败行为，Go 测试分别断言偏离结果，不通过 normalization 隐藏差异。
