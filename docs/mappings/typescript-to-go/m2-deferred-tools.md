# M2.4 Deferred tools TypeScript 到 Go 导航

| 固定 Pi 来源 | Pig 入口与行为 | 验证 |
| --- | --- | --- |
| `ai/src/utils/deferred-tools.ts#splitDeferredTools` | `ai.SplitDeferredTools`：规范化去重、稳定顺序、开关与 transcript 拆分 | `internal/parity/deferred_tools_test.go#TestDeferredToolsParity` |
| `ai/src/types.ts#ToolResultMessage.addedToolNames` | `ai.ToolResultMessage.AddedToolNames`，值和指针变体、null/empty | `ai/issue63_deferred_tools_test.go` |
| `agent/src/types.ts#AgentToolResult.addedToolNames` | Tool 执行结果保留发现标记，SetTools 后 Continue 使用新定义 | `agent/issue63_deferred_tools_test.go` |
| `ai/src/providers/faux.ts#createFauxCore` | Faux factory 中通过公开 helper 观察请求中的 Tool 分组 | `examples/deferred-tools/main.go` |

`parity/oracle/deferred-tools.mjs` 直接调用锁定 commit `936aff00918de1187f085f123c2812d8f2d67745` 的函数，生成共享规则 fixture 与“标记后调用”偏离 fixture。Go 对等重放保留完整 Tool 元数据和组内顺序；Pi 的 deferred Map 仅投影为有序 values 数组，没有忽略字段或 runner normalization。

后续调用恢复 immediate 是 #63 要求的 [ADR-0016](../../adr/0016-deferred-tools-used-promotion.md) 偏离，独立断言 Pi 与 Pig 的不同结果。公共 API 冻结在 `ai/testdata/issue63_surface_golden.txt`。具体 API Adapter 的 deferred-tools 测试与 wire 字段仍按 M10 Catalog 项验收；本切片不宣称这些适配器已实现。
