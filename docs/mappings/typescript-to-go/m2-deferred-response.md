# M2.3 Deferred response TypeScript 到 Go 导航

| 固定 Pi 来源 | Pig 入口与实现 | 验证 |
| --- | --- | --- |
| `providers/faux.ts#createFauxCore` | `ai.CreateFauxCore`、`ai/faux_deferred.go` | `ai/issue62_deferred_test.go` |
| `providers/faux.ts#fauxProvider` | `ai.NewFauxProvider` 暴露真实 fetch/cancel capability | `ai/issue62_provider_test.go` |
| `models.ts#fetchDeferred`、`cancelDeferred` | `ai.Models.FetchDeferred`、`CancelDeferred` 沿用认证与转换分派 | `TestDeferredModelsPreservesAuthenticatedRequestOptions` |
| `types.ts#DeferredHandle`、`SimpleStreamOptions` | 既有 Go 类型、`Optional` 与 `DeferredBoolean` / `DeferredWindowOptions` | `TestDeferredLifecycleThroughModels` |
| `utils/event-stream.ts` | `AssistantMessageEventStream.Result` 与 done/error 终态 | `TestDeferredCancelDuringStreamingPreservesPartialAndTerminalOrder` |

`parity/oracle/deferred-lifecycle.mjs` 从锁定 Pi commit `936aff00918de1187f085f123c2812d8f2d67745` 生成 fixture；`internal/parity/deferred_lifecycle_test.go` 通过公开 Go API 重放。时间戳与随机 ID 按 Case 声明投影为存在性及稳定性断言，其余事件、字段与计数直接比较。

并发、快照和无效取消的 Go 语义见 [ADR-0015](../../adr/0015-deferred-response-lifecycle.md)。整体完成度继续以 `parity/catalog.jsonl` 为准。
