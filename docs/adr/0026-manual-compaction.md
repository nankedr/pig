# 手动压缩的请求与提交边界

Issue #89 实现 production v3 AgentSession 的手动压缩。固定 Pi 的 cut point、split turn、历史更新提示、文件操作、输出预算和摘要重试分别映射到 codingagent；仅复用 agent 已交付的 token 估计及 RetryPolicy 数据类型，不接入 Harness v4 日志或运行时。

Compact 先占用 Session 操作入口，取消并等待当前整轮完成（包含工具收尾与 v3 终态记录），再发布 compaction_start。摘要期间拒绝新 Prompt 和配置变更；Steer/FollowUp 沿用已有内存队列，供之后显式 Prompt 消费，不自动启动生成。与 Pi 的压缩期间队列自动排空分支不同，该分支留待队列/自动压缩切片。不要从正在执行的同步生成 listener 内阻塞调用 Compact；应交给另一个 goroutine。AbortCompaction、Abort 和 Dispose 可从同步 listener 调用。

摘要使用当前模型、thinking、ModelRuntime/认证和既有 stream；每个历史/前缀请求有独立 routing session ID，禁用 prompt cache，同一请求的重试复用该 ID。摘要重试的预算来自 settings.retry，生命周期使用 summarization_retry_*，不触碰整轮 retryAttempt/AbortRetry。只分类 Assistant 终态和已有适配器明确的缺失 finish_reason SSE 中断；任意 Go error 不推断为可重试。失败请求的 usage 不计入结果，split turn 合并两个成功请求的 usage。

在 Session 和 SessionManager 锁内检查取消及原分支 leaf，准备完整 v3 文件，临时文件写入后原子 rename，再发布内存条目和上下文。取消检查与 rename 之间为提交边界；提交后到达的取消不撤回成功结果。已有完整历史保留，裁剪只改变当前模型上下文。存储失败没有新条目，也不裁剪；不声称跨进程并发写入或断电事务。直接修改底层 Agent 不属于 Session 的操作事务。

compaction_end 发布前解除压缩占用，listener 可提交下一次 Prompt。结果、事件和持久化条目各自拥有数据，前一个 listener 的修改不影响后一个 listener。WaitForIdle 包括压缩请求与提交，结束事件的同步回调不属于其等待范围。Go Context 取消保留 errors.Is 语义；Oracle 的跨语言取消错误字符串投影为 cancelled，事件 aborted 字段按原值比较。Pi 在无 AbortSignal 的孤立 aborted Assistant 上可能提交空摘要；Pig 将该终态视为取消，不裁剪。

沿用已有函数的 ...any 形状，但已实现的摘要 API 仅接受零个选项或一个 SummaryOptions 值；Compact 的 preparation 为 *CompactionPreparation。这样将 Pi 的位置参数映射为类型明确的 Go 配置。非法选项和非正输出预算明确失败。提示文本来自固定 Pi MIT 源码，归属见 THIRD_PARTY_NOTICES。

自动阈值/溢出压缩、扩展 hook、自定义扩展摘要、branch summary、RPC/TUI 操作及尚未交付的 Adapter 仍未实现。本切片不宣称 M4 整体冻结。
