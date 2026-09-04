# M2.5 Telemetry：TypeScript 到 Go

| Pi 固定基线 | Pig 公开接口 | 语义与验证 |
| --- | --- | --- |
| `packages/telemetry/src/memory.ts#InMemoryTelemetryContext` | `telemetry.InMemoryTelemetryContext` | 零值构造，无界收集；`TestInMemorySpanLifecycle`、`TestTelemetryMemoryParity` |
| `startSpan` / callback / Promise | `StartSpan(ctx, options, fn)` | 原样返回值/error，panic 结束后原样传播；`TestInMemoryPreservesFailuresAndExplicitStatus` |
| span `setAttributes` / `addEvent` / `setStatus` | `TelemetrySpan.SetAttributes` / `AddEvent` / `SetStatus` | 原子记录、事件顺序、最后显式状态；`TestInMemorySnapshotsAndPassiveRecording` |
| `getSpans` / `RecordedTelemetrySpan` | `GetSpans` / `RecordedTelemetrySpan` | 属性数组、事件、状态、指针独立复制；父子关系与结束序号 |
| `NOOP_TELEMETRY_CONTEXT` | `NOOPTelemetryContext` | 默认 NOOP；settled 父 span 的晚建 child 仍执行 callback |
| `createTelemetryAdapterConformance` | `telemetry/testing.CreateTelemetryAdapterConformance` | 原始 9 个 case 实际运行；`TestInMemoryAdapterConformance` |
| `TelemetryAdapterFixture` / `AsyncDisposable` | `Context` / `GetSpans` / `Close` | 每次 Run 独立创建与清理；`TestConformanceRejectsBrokenAdaptersAndClosesFixtures` |
| 不可读 Proxy / `Error` 详情 | 契约外动态属性、未知 status、panic 的 `Error()` | 静态 API 无 getter；具体边界见 [学习文档](../../learning/m2-telemetry.md#go-被动性映射) |

并发由 goroutine 和显式 channel barrier 验证，无全局当前 span。Pi fixture 由 `parity/oracle/telemetry.mjs` 生成，Go 使用 `internal/parity/telemetry_test.go` 重放。
