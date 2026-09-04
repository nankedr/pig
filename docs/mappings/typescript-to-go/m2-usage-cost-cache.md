# M2.2 Usage、cost 与 cache TypeScript 到 Go 导航

本页对照固定 Pi 源码与 Pig 的 M2.2 实现。完成度和证据以 `parity/catalog.jsonl` 为唯一权威。

| Pi 来源 | Pig 实现 | 责任 | 主要证据 |
| --- | --- | --- | --- |
| `packages/ai/src/types.ts#Usage` | `ai.Usage`、`ai.UsageCost` | 统一 token buckets、可选 breakdown 与分项/总成本 | `ai/issue61_openai_usage_test.go` |
| `packages/ai/src/models.ts#calculateCost` | `ai.CalculateCost` | 基础费率、最高匹配 tier、1h cache write 双倍 input rate | `ai/model_helpers_test.go`、`usage-cost-cache.json` |
| `packages/ai/src/providers/faux.ts#withUsageEstimate` | Faux Provider stream | UTF-16 token 估算、session cache write/read、隔离与禁用 | `ai/issue61_usage_test.go`、`TestUsageCostCacheParity` |
| `packages/ai/src/api/openai-completions.ts#parseChunkUsage` | `ai.StreamOpenAICompletions` 内部 usage mapper | chunk/choice 来源、详细/旧式 cache、input clamp、reasoning 与成本 | `ai/issue61_openai_usage_test.go`、`TestUsageCostCacheParity` |
| `packages/ai/src/utils/event-stream.ts` | `ai.AssistantMessageEventStream` | done、重复 `Result` 和 partial cancellation 的 usage 一致性 | `ai/issue61_usage_test.go` |
| `packages/ai/src/models.ts#createModels` | `ai.Models.Complete` | 将 Provider stream 归约为保留 usage/cost 的终态消息 | `internal/parity/usage_cost_cache_test.go` |

Parity fixture 由锁定 commit `936aff00918de1187f085f123c2812d8f2d67745` 的 Pi 源码生成。Go 侧通过公开入口重放，不直接调用私有 mapper。
