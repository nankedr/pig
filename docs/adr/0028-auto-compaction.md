# 自动压缩的恢复预算与队列边界

Issue #90 在 production v3 AgentSession 的 Prompt 前后检查自动压缩，复用 ADR-0026 的 preparation、摘要认证/预算/重试和原子 v3 提交。Harness v4 不参与。settings.compaction.enabled 默认开启，公开 getter 读取有效 settings，setter 沿用同步持久化；窗口、reserveTokens 和 keepRecentTokens 来自当前模型与 settings。

固定 Pi 优先整轮 Provider retry；耗尽或错误不可重试时再检查压缩。agent_end.willRetry 只表示整轮 retry，overflow 的恢复承诺在 compaction_end.willRetry。成功但 usage 超窗口按 overflow 压缩，不重发已完成的答案；length 只有低于模型原始 maxTokens，或符合零输出上下文溢出条件才恢复。threshold 使用严格大于 contextWindow-reserveTokens，错误或全零 usage 回退到最近有效 usage 加尾随估算。timestamp 不晚于最新 compaction 的 Assistant 与旧 usage 不再触发。

每个用户输入最多一次 overflow compact-and-retry；普通成功 Assistant 重置预算，error 和 length 不重置。失败/截断 Assistant 已写入 v3，只从模型上下文移除，压缩重建后可能被恢复，因此继续前再移除最后一条 error/length。固定 Pi 在“503 → overflow”且前一个 503 仍位于保留尾部时，继续生成返回 cannot continue from message role: assistant；共同 Oracle 记录这一终态，Pig 保留它，不声称所有 Provider retry/overflow 组合均可恢复。空摘要仍沿用基线提交规则，重试后仍 overflow 则终止，不无限压缩。

本切片调整 ADR-0018 在自动压缩期间的 Session admission：底层 Agent 此时空闲，Session 暂存 steering/follow-up，恢复时按队列模式交还底层运行。threshold 若有排队消息则继续；摘要普通失败也可以处理已排队的新用户请求。运行后取消摘要则保留队列，等待显式新 Prompt 或 ClearQueue。输入前检查属于已提交的新 Prompt：AbortCompaction 只取消该摘要，当前 Prompt 仍执行并消费队列；Abort/父 Context 则取消整次运行。底层 Agent、普通 idle、Provider retry 等待和 agent_end 的投递限制不变。扩展 custom queues 与手动压缩期间队列不在本切片范围。

自动压缩的取消在 compaction_start 发布前可用，沿用 ADR-0024 的 Go 同步 callback 取消规则，避免 Pi 先发布事件再创建 AbortController 的窗口。IsCompacting 在开始/结束 callback 中为 true；Prompt 与 WaitForIdle 覆盖摘要、提交及后续生成。配置修改在自动压缩期间拒绝；Prompt 带明确 streamingBehavior、Steer/FollowUp 可入队。结束 callback 不应同步调用阻塞的 Compact；应在另一个 goroutine 发起。

取消或持久化失败不发布新 compaction 条目。overflow 的原始 partial 从模型上下文移除后，HeadlessOutcome 独立保留它；取消摘要还标记 Canceled，避免退回此前成功回答。提交边界后取消不撤回结果，Abort/Context/Dispose 仍能阻止下一轮生成。恢复调用因取消或错误未开始投递时，已从缓冲队列取出但仍未消费的消息归还队首；ClearQueue 已删除的消息不恢复。无 preparation 时不发送虚假开始/成功事件。公开低层构造器未配置 SessionManager 时跳过自动压缩，保持已有无持久化 AgentSession 的执行能力。

证据为固定 Pi 源码/发布 SDK 的 auto-compaction Oracle、公开 SDK Parity Case，以及 Headless 的取消、失败、设置和真实子进程重开测试。RPC/TUI 控件、扩展 hook/自定义摘要、尚未交付的 API Adapter 仍未实现；branch summary 已由 ADR-0030 / Issue #92 接通，不宣称 M4 整体冻结。
