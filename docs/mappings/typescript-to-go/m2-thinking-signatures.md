# M2.1 Thinking 与 signature TypeScript 到 Go 导航

本页对照固定 Pi 源码与 Pig 的 M2.1 thinking/signature 实现。完成度和证据以 `parity/catalog.jsonl` 为唯一权威。

| Pi 来源 | Pig 实现 | 责任 | 主要证据 |
| --- | --- | --- | --- |
| `packages/ai/src/types.ts#ThinkingContent` | `ai.ThinkingContent`、`ai.ToolCall.ThoughtSignature`、`ai.Usage.Reasoning` | 公开 thinking、signature 与 reasoning usage 值 | `ai/issue60_surface_test.go`、`ai/issue60_thinking_test.go` |
| `packages/ai/src/types.ts#AssistantMessageEvent` | `ai.AssistantMessageThinkingStartEvent`、`ThinkingDeltaEvent`、`ThinkingEndEvent` | 不可变 partial snapshot 与 thinking 事件生命周期 | `TestOpenAICompletionsThinkingAndSignatureParity` |
| `packages/ai/src/types.ts#StreamOptions` | `ai.SimpleStreamOptions.Reasoning`、`ai.ThinkingBudgets` | reasoning 等级、预算覆盖与 simple-stream 转换 | `TestSimpleOpenAICompletionsClampsReasoningAndThinkingBudget` |
| `packages/ai/src/api/openai-completions.ts#streamSimple` | `ai.StreamSimpleOpenAICompletions` | thinking level clamp、默认预算与回答 token 预留 | `TestOpenAICompletionsThinkingAndSignatureParity` |
| `packages/ai/src/api/openai-completions.ts#stream` | `ai.StreamOpenAICompletions` | Provider-specific request 字段、SSE reasoning 优先级、usage 和 content 顺序收尾 | fixed Pi `openai-completions-thinking.json` |
| `packages/ai/src/api/openai-completions.ts#convertMessages` | `ai.ConvertOpenAICompletionsMessages` | 同模型 thinking/signature 回放、跨模型 text 转换和 redaction | `TestOpenAICompletionsTransformsThinkingAcrossModels` |
| `packages/ai/src/api/openai-completions.ts#parseChunkUsage` | OpenAI Chat Completions usage mapper | `reasoning_tokens` 单独记录，output/total 不重复累计 | `TestOpenAICompletionsStreamsThinkingDetailsAndReasoningUsage` |
| `packages/ai/src/providers/faux.ts` | Faux Provider stream | 离线 thinking start/delta/end、取消和 Provider error 的 partial 保留 | `TestFauxStreamsThinkingAndPreservesItsReplayMetadata` |

Go 侧仍使用统一 `AssistantMessageEventStream`，没有为 reasoning 建第二套 stream。Provider wire 差异收敛在 Chat Completions adapter 内，公开调用者只处理 `ThinkingContent`、事件与最终 `AssistantMessage`。
