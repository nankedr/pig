# M3.7 TypeScript → Go 导航

| 固定 Pi | Pig | 验证 |
| --- | --- | --- |
| `core/model-runtime.ts` | `codingagent/models.go`、`model_runtime.go`、`model_snapshot.go` | `issue77_runtime_test.go` |
| `core/model-registry.ts` | `codingagent/models.go` 的 `ModelRegistry` | 查询副本和来源状态 |
| `core/model-resolver.ts` | `codingagent/model_resolver.go`、`headless_settings.go` | `parity/oracle/model-runtime.mjs` → `internal/parity/model_runtime_test.go` |
| `core/agent-session-services.ts`、`core/sdk.ts` | `codingagent/session_services.go`、`sdk.go` | SDK 与 Headless 两轮请求对比 |
| `cli/list-models.ts`、`main.ts` | `codingagent/model_list.go`、`misc.go`、`headless.go` | `cmd/pig/issue77_process_test.go` |

先运行 `go run ./examples/model-runtime`，再从 `CreateAgentSessionServices` 追到 `CreateAgentSession` → `ModelRuntime.StreamSimple` → `ai.Models.StreamSimple`。Provider 及流协议保持原有边界。

Go 的 `context.Context` 承担取消；零值 runtime 仍明确失败。`ModelsPath` 的 absent/null 都只使用内嵌静态目录，具体路径、ModelsStore/cache/overlay 属于后续 M10。`Offline` 是新增 Go 选项，共用公开 `ResolveOffline`。学习材料见 [M3.7](../../learning/m3-model-runtime.md)。
