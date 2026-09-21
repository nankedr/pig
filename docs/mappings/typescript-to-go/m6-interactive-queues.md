# M6.8 TypeScript → Go：取消、插话与队列

| 固定 Pi 边界 | Pig 边界 | 证据 |
| --- | --- | --- |
| InteractiveMode 的 onSubmit / handleFollowUp | InteractiveMode.Run + TextUI.ReadInput | queues.json：真实 CLI 两类队列、优先级、下一轮历史 |
| handleDequeue / restoreQueuedMessagesToEditor | AgentSession.TakeQueuedMessages + TextUI.PrependEditor | queues-restore.json、queues-abort.json；SDK 消费竞争 |
| Agent.clearAllQueues / AgentSession.clearQueue | 保留 ClearQueue，新增 Agent / AgentSession.TakeQueuedMessages | 原子回取只返回尚未消费项；Go 增量 API snapshot |
| onEscape / abortRetry | 带轮次标识的 TextInput + 当前轮 context / AgentSession.AbortRetry | CLI 部分 Assistant/Bash 输出、Session 记录和退出清理 |
| auto_retry_start/end / RetryStatusIndicator | 既有 Session 重试事件 + 交互层倒计时 | queues-retry.json、queues-retry-success.json |

延续 ADR-0018/0024 的 Go admission 和取消时序偏离。收尾期尚未入队的新输入等待完整 settled；取消时回填，避免重复提交。重试等待期新投递仍按现有契约拒绝并回填。底层 transport retry 不属于此次 UI 实现。

学习路径见 [中文说明](../../learning/m6-interactive-queues.md)。Catalog 保留 explicit partial，五个 Oracle fixture 均来自固定源码离线构建后的真实子进程；不把私有 helper 测试当对等证明。
