# M2.8 上下文估算与 overflow TypeScript 到 Go 导航

| 固定 Pi 来源 | Pig 入口 | 责任与证据 |
| --- | --- | --- |
| `packages/ai/src/utils/estimate.ts#estimateContextTokens` | `ai.EstimateContextTokens`、`ai.ContextUsageEstimate` | 可信前缀 usage、后续消息、动态 Tool；`TestContextOverflowParity` |
| `packages/ai/src/utils/estimate.ts` 的文本、图片、消息与 JSON helper | `ai/estimate.go` 内部 helper | UTF-16、图片固定成本、thinking 与 Tool 参数；通过公开 Context 入口验证 |
| `packages/ai/src/utils/overflow.ts#isContextOverflow` | `ai.IsContextOverflow` | Provider 错误、排除模式、静默超限和 99% 临界值；`context-overflow.json` |
| `packages/ai/src/utils/overflow.ts#isRecoverableLength` | `ai.IsRecoverableLength` | 与裁剪前原始输出上限比较；fixture 和 `TestContextEstimateClampsSimpleRequestAndPreservesDesiredOutput` |
| `packages/ai/src/utils/overflow.ts#getOverflowPatterns` | `ai.GetOverflowPatterns` | 有序 regexp 列表；fixture 验证每个模式的命中位置，Go 验证副本独立性 |
| `packages/ai/src/api/simple-options.ts#clampMaxTokensToContext` | OpenAI 简单请求的输出上限裁剪 | 复用统一估算器，保留既有 4096 safety tokens 和最少 1 的边界 |

Pi 的 Context/消息数组联合参数在 Go 使用 `ai.Context`，仅消息数组写成 `ai.Context{Messages: messages}`。可选窗口使用变参 `...int64`；`LastUsageIndex` 使用 `*int` 保留索引零与 null 的区别。估算相关 helper 在固定 Pi 中属于内部工具，本切片将完整 Context 估算作为公开 Go 能力，未扩展 agent/codingagent 的压缩接口。

`ai/issue67_surface_test.go` 固定公开类型和函数签名；`ai/issue67_context_test.go` 覆盖 Faux 的 Stream/Complete 和 Go 特有输入形态。能力状态以 `parity/catalog.jsonl` 为唯一权威。
