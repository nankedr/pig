# M2.1 Thinking 与 signature 流

M2.1 让 reasoning model 的思考内容不再退化成普通文本或被静默丢弃。Faux 与 OpenAI Chat Completions 现在都能产出 `thinking_start`、`thinking_delta`、`thinking_end`，终态 `AssistantMessage` 保留 `ThinkingContent.ThinkingSignature`；OpenAI 的 encrypted reasoning detail 会绑定到对应 `ToolCall.ThoughtSignature`。

## 数据流

```text
SimpleStreamOptions.reasoning / thinkingBudgets
  -> model thinking-level clamp 与 Provider compat
  -> reasoning_effort / thinking_token_budget / Provider-specific thinking 字段
  -> OpenAI SSE reasoning_content | reasoning | reasoning_text
  -> ThinkingContent + thinking events
  -> reasoning.encrypted detail 按 id 绑定 ToolCall.thoughtSignature
  -> AssistantMessage outcome + Usage.reasoning
```

reasoning delta 的优先级是 `reasoning_content`、`reasoning`、`reasoning_text`，同一个 chunk 只消费第一个非空字段，避免 Provider 同时返回别名时重复输出。首个 delta 会先打开 thinking block，并让 `thinking_start.partial` 已包含该段文本；后续事件持有不可变快照。流结束时，thinking、ToolCall 和 text 的 end 事件按最终 content 顺序发送。

## 请求映射

`StreamSimpleOpenAICompletions` 先把 `ThinkingLevel` 按模型支持范围收敛，再使用 `Model.ThinkingLevelMap` 映射 Provider 值。显式 `null` 会禁用对应等级，缺失映射则使用等级本身。M2.1 支持这些 `thinkingFormat`：

- `openai`：`reasoning_effort`
- `deepseek`：`thinking.type`
- `openrouter`、`ant-ling`：`reasoning.effort`
- `qwen`：`enable_thinking`
- `zai`：`thinking.type` 与 `clear_thinking`
- `together`：`reasoning.enabled`
- `string-thinking`：字符串 `thinking`

`supportsThinkingTokenBudget` 启用时，预算来自当前 thinking 等级的覆盖值或默认值，并在 `maxTokens - 1024` 处截断，为最终回答保留空间；非正值不发送。`chat-template`、`qwen-chat-template` 和 `baseten` 属于后续兼容分支，当前明确返回 `ErrNotImplemented`。

## 历史回放与 signature

同模型回放会合并 thinking blocks，并使用第一个 `ThinkingSignature` 选择请求字段；`opencode-go` 的 `reasoning` 会规范化为 `reasoning_content`。`requiresReasoningContentOnAssistantMessages` 会为没有 thinking 的 reasoning Assistant 补空字段。`requiresThinkingAsText` 则把非空 thinking 用双换行连接为 text part。

跨模型回放只把非空、未 redacted 的 thinking 转成普通文本；redacted/空 thinking 和 Provider 专属 Tool thought signature 会被丢弃，避免把不可移植元数据发送给另一模型。Chat Completions 不转发 `TextContent.TextSignature`。

响应中的 reasoning detail 只有在 `type` 精确为 `reasoning.encrypted` 且 `id`、`data` 都非空时才有效。detail 可以先于或晚于 ToolCall 到达；同一待绑定 ID 的最后一个值生效，绑定后即被消费，不会泄漏到其他 ToolCall。

## 离线运行

Faux 示例不需要密钥或网络：

```sh
go run ./examples/thinking-signatures
```

预期输出包含 thinking delta、保留的 signature 和最终回答。固定 Pi Oracle 使用：

```sh
node --experimental-strip-types parity/oracle/openai-completions-thinking.mjs .upstream/pi --check
go test ./ai -run '^TestOpenAICompletionsThinkingAndSignatureParity$' -count=1
```

fixture 固定请求映射、同/跨模型历史转换、事件顺序、encrypted Tool signature 与 reasoning usage；逐字段完成度仍以 `parity/catalog.jsonl` 为准。
