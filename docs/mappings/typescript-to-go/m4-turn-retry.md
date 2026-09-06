# M4.4：整轮重试源码导航

固定 Pi Code Baseline：`936aff00918de1187f085f123c2812d8f2d67745`。

| Pi | Pig | 观察边界 |
| --- | --- | --- |
| coding-agent/src/core/agent-session.ts：_runAgentPrompt、_handlePostAgentRun | codingagent/session.go：Prompt | 一次 Prompt 等待多次 Agent Continue 与完整 Tool 循环 |
| agent-session.ts：_prepareRetry、_willRetryAfterAgentEnd | codingagent/session_retry.go | 预算、退避、willRetry、上下文裁剪与取消 |
| ai/src/utils/retry.ts：isRetryableAssistantError | codingagent/session_retry.go：retryableSessionError | 通过 SDK Oracle 验证分类，未新增 AI 公共接口 |
| agent-session.ts：_handleAgentEvent | codingagent/session.go：handleAgentEvent | listener → 持久化 → 恢复结束事件 |
| agent-session.ts：abortRetry、isRetrying、retryAttempt、autoRetryEnabled、setAutoRetryEnabled | codingagent/session.go 同名 Go 方法 | 设置开关、等待状态、attempt 与终止 |
| settings-manager.ts：getRetrySettings/getProviderRetrySettings | codingagent/settings_accessors.go | 外层 retry 与内层 retry.provider |
| ai/src/api/openai-completions.ts：截断 stream 的 error Assistant | agent/runtime.go：保留最后 partial 并完成错误消息生命周期 | 底层 AI 的 ErrOpenAISSETruncated 契约保持不变 |
| ai transport retry | ai/openai_completions.go | 单请求重试，终态错误进入 Session |
| coding-agent/src/modes/print-mode.ts | codingagent/headless.go、modes.go | final partial、text/JSON、退出码 |
| coding-agent/test/agent-session-retry.test.ts、test/suite/agent-session-retry-events.test.ts | parity/oracle/turn-retry.mjs、codingagent/issue86_retry_test.go | 锁定公开 SDK 的确定性用例 |
| 真实 Headless CLI | cmd/pig/issue86_process_test.go | 本地 HTTP 的 transport/整轮预算与落盘 |

[学习材料](../../learning/m4-turn-retry.md)解释事件顺序与配置；[ADR-0024](../../adr/0024-session-turn-retry.md)记录同步取消窗口和 Headless partial 结果的 Go 映射。示例：`go run ./examples/turn-retry`。
