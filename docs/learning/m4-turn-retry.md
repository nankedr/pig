# M4.4：Provider 错误后的整轮重试

Headless 和公开 SDK 的 AgentSession 现在能在可重试 Provider 错误后继续生成。先运行离线示例：

```sh
go run ./examples/turn-retry
```

首次调用产生 partial + overloaded_error，第二次恢复成功。模型上下文留下用户消息与成功 Assistant，Session 历史则额外保留失败 Assistant。自动恢复删除的只是最后一个错误 Assistant，不重发用户 Prompt，也不删除或重放之前成功的 ToolCall/ToolResult。重试恢复成 ToolCall 时，Prompt 等待工具及后续生成全部完成。

全局或已信任项目的 settings.json 可以配置两层预算：

```json
{
  "retry": {
    "enabled": true,
    "maxRetries": 3,
    "baseDelayMs": 2000,
    "provider": { "maxRetries": 2, "maxRetryDelayMs": 60000 }
  }
}
```

`retry.provider` 控制适配器的单请求 transport retry：连接失败与被策略接受的 HTTP 状态在该层处理；已进入成功响应体的流式失败不由该层重放。AI API 返回的明确 SSE 截断错误在 Agent 边界转换为保留最后 partial 的错误 Assistant，再由 Session 判定是否重试；其他协议错误仍直接传播。`retry` 控制 AgentSession 在终态 Assistant error 后继续生成，默认开启，最多重试三次，等待 2/4/8 秒，无 jitter。实际 timer 沿用 Node：delay 小于 1ms 或超过 2147483647ms 时按 1ms 调度，事件仍报告计算出的 delayMs。Go 的 delayMs 为 int64；指数结果超出该范围时返回明确错误，不溢出或启动新的请求。两层预算独立，例如单请求预算 1、整轮预算 1，连续 503 最多发出 4 次 HTTP 请求。只关闭 retry.enabled 不会关闭 transport retry；单独设置 retry.provider.maxRetries 为 0 才会关闭后者。

错误分类来自固定 Pi 的 ai/utils/retry.ts：包括 overloaded、限流、指定 5xx、网络断连、DNS、超时、提前结束的 stream、明确要求重试的 Provider 文案等；配额、账单、余额和订阅限额优先排除。Context overflow 不触发整轮重试，压缩编排仍未开放。401、403 等不可重试错误直接终止；错误对象只有 Go error 而没有可重试 Assistant 时也不会恢复。

公开控制接口为 `AutoRetryEnabled()`、`SetAutoRetryEnabled(bool)`、`RetryAttempt()`、`IsRetrying()` 和 `AbortRetry()`。SetAutoRetryEnabled 写回 SettingsManager；禁用开关影响下一次重试判定，已经开始的等待通过 AbortRetry 取消。IsRetrying 只表示退避等待，正在执行重试请求时为 false；RetryAttempt 保留次数，直到非 error Assistant 的 message_end 后结束恢复序列。每个成功 ToolCall 响应都会重置该预算，因此一次用户 Prompt 中可以出现多段独立恢复。

事件顺序：失败 `message_end` → 写入 Session → `turn_end` → `agent_end(willRetry=true)` → `auto_retry_start` → 移除错误上下文 → 等待 → Continue。成功恢复在 `message_end` 持久化后发出 `auto_retry_end(success=true)`，随后完成 Tool 循环。耗尽预算或恢复过程中遇到永久错误时，最后一个 `agent_end(willRetry=false)` 后发出失败结束事件。整个 Prompt 只有一个 `agent_settled`。

等待取消发出 `auto_retry_end(success=false, finalError="Retry cancelled")`，次数归零且不再发出请求。HeadlessOutcome 保留失败 Assistant partial 并标记 Canceled；详见 [ADR-0022](../adr/0022-session-turn-retry.md)。同步订阅者可以在 auto_retry_start 中立即调用 AbortRetry。取消整次工作应使用 Context 或 Abort；Dispose 同时撤销订阅。WaitForIdle 仍是收尾屏障。

JSON CLI 原样输出 session header 与重试事件，Provider 失败保留为事件并沿用 JSON 模式 exit 0；文本模式只输出最后一次成功文本，终态失败 exit 1；取消 exit 130。持久化 Session 保留所有失败 Assistant，重开按 v3 原有规则装载，不因重开而自动发起请求。

固定 Pi `936aff00918de1187f085f123c2812d8f2d67745` 的公开 createAgentSession 与 Faux Provider 生成 65 个 Oracle 场景：52 类错误、成功、多次退避、耗尽、开关、零预算、取消以及 Tool 前后恢复、零/负值/超过 Node 计时上限的延迟。Go Parity Case 比较事件、attempt、Provider 上下文、当前消息、持久化历史和 Tool 执行次数。实现前先使该用例失败，再逐片实现；CLI 使用真实本地 HTTP Provider 验证两个重试层。

```sh
node --experimental-strip-types parity/oracle/turn-retry.mjs /path/to/locked-pi --check
go test -race ./codingagent -run '^TestTurnRetry' -count=1
go test ./cmd/pig -run '^TestPigTurnRetryLayers$' -count=1
```

证据归属 `contract:codingagent/turn-retry`，API snapshot 是 `codingagent/testdata/issue86_surface_golden.txt`。当前只开放已实现 Provider 适配器上的 SDK/Headless 整轮重试；JSONL RPC 控制、扩展异步 hook、压缩、摘要重试、用户队列编排和其他 Provider 适配器保持各自后续切片的 Capability Stub。本切片不宣称 M4 整体完成。
