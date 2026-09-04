# M2.7 TypeScript 到 Go：消息 handoff

| 固定 Pi 来源 | Pig 入口 | 行为与证据 |
| --- | --- | --- |
| `packages/ai/src/api/transform-messages.ts#transformMessages` | `ai.TransformMessages`、`ai.ToolCallIDNormalizer` | 同模型签名、跨模型 thinking 降级、ID 映射、缺失结果补全；`TestMessageHandoffParity` |
| `packages/ai/src/api/openai-completions.ts#convertMessages` | `ai.ConvertOpenAICompletionsMessages` | 目标 ID 约束、通用转换后的 wire；fixture 的 `wire-openai` / `wire-other` |
| `packages/ai/test/transform-messages-copilot-openai-to-anthropic.test.ts` | `ai/transform_messages_test.go` | 同 Provider 不同 API/模型的 thinking、Tool signature 和尾部孤立调用 |
| `packages/ai/test/lax-message-content.test.ts` | `TestMessageHandoffParity` 的 `nil-content` | Pi null/缺失内容映射到 Go nil/零值，再通过公开转换规范化；strict codec 不变 |
| `packages/ai/test/cross-provider-handoff.test.ts` | `agent/issue66_handoff_test.go`、`examples/message-handoff` | 确定性 Fetch/Faux 替代真实 Provider，验证 `Agent.SetModel` 后的实际请求和历史保留；不声称网络 Provider 全部实现 |

`ai/transform_messages_test.go#TestIssue66LockedGoAPISnapshot` 固定本次公开 API。`parity/oracle/message-handoff.mjs` 直接执行锁定 Pi 源码生成 fixture，`internal/parity/message_handoff_test.go` 从同一输入执行公开 Go SDK 并比较完整消息与 callback/wire 观察。

图片降级属于 M12，`TestTransformMessagesKeepsImageDowngradeExplicitlyDeferred` 验证其 Capability Stub；它不进入非图片对等结论。
