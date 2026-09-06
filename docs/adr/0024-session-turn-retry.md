# 整轮重试的取消与结果所有权

Issue #86 在 AgentSession 层继续既有 Agent 历史，沿用固定 Pi 的错误分类、预算、指数退避、事件顺序和错误 Assistant 仅从模型上下文移除的规则。错误消息仍写入 v3 Session；ToolResult 和已完成 ToolCall 保留，不能为恢复错误而重放整条用户 Prompt。

沿用 ADR-0006 的不可变快照与 Go Context 取消模型，Pig 在发布 `auto_retry_start` 前建立可取消等待。同步 SDK listener 可以立即调用 AbortRetry；固定 Pi 在该事件之后才建立 AbortController，同步取消可能丢失。这个刻意偏离消除取消窗口，不改变 Oracle 中异步取消、成功、耗尽、历史和事件内容的结果。IsRetrying 表示等待可取消，RetryAttempt 表示本次恢复序列的次数；普通结束事件发布后归零，等待取消则先归零再发布结束事件。

等待取消后，Agent 上下文中已经没有失败 Assistant。HeadlessOutcome 独立保留该 Assistant 的 partial 内容并标记 Canceled，避免误报“没有结果”或退回更早的成功回复。它不重新插入模型上下文，也不重复持久化。AbortRetry 只取消等待，正在生成时调用是无操作；Abort/Context/Dispose 负责整次运行的取消。Prompt 与 WaitForIdle 必须等到完整 Tool 循环和重试收尾。

`retry.provider` 仍是单请求 transport 策略，先在适配器内部耗尽；`retry` 是 AgentSession 策略，只根据终态 Assistant 决定是否继续。后者不根据任意 Go error 推断可重试性，持久化、参数或 listener 错误不能触发新的模型请求。RPC、扩展 hook、压缩与摘要重试继续由各自切片实现。

OpenAI Chat Completions 的底层 SSE API 继续以 ErrOpenAISSETruncated 返回截断协议错误。Agent 在已观察到 Assistant start 的情况下，仅在错误明确说明缺少 finish_reason 时，将中途截断转换成携带最后 partial 的错误 Assistant（固定 Pi 的 Stream ended without finish_reason 文案），再完成 message/turn/agent end 生命周期。已完成响应后的残缺尾帧维持原协议错误，不触发恢复。其他协议错误、Capability Stub 和任意 Go error 不作转换；取消则产生 aborted Assistant。这让公开 Session 的重试策略看到 Pi 中同等的终态，同时保留底层 AI 协议校验契约。
