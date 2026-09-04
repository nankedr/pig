# M2.6 TypeScript 到 Go 导航

Code Baseline：`936aff00918de1187f085f123c2812d8f2d67745`。

| Pi | Pig | 验证 |
| --- | --- | --- |
| `packages/ai/src/compat.ts` 注册表、stream/complete | `ai/compat.go` | `ai/issue65_compat_test.go`、`internal/parity/compat_session_resources_test.go` |
| `packages/ai/src/legacy-api-aliases.ts` 16 个旧别名 | `ai/compat.go`，真实 Chat Completions 实现在 `ai/openai_completions.go` | `TestCompatDeprecatedAliasesShareRegisteredProvider`、deviation fixture |
| `packages/ai/src/providers/faux.ts` core 与 Provider，compat `registerFauxProvider` | `ai/faux.go` 与 `ai/compat.go` 的公开声明 | `TestCompatFauxQueueIsolationAndCompletionOutcomes`、共同 fixture |
| `packages/ai/src/session-resources.ts` | `ai/session_resources.go` | `ai/issue65_cleanup_test.go`、共同与 deviation fixture |

`parity/oracle/compat-session-resources.mjs` 从固定 Pi 生成两份 fixture；`ai/testdata/issue65_surface_golden.txt` 冻结 Go API。并发注册、reset、回调快照是 Go 端的 race 测试证据，不能等同于单线程 Pi Oracle 证据。

见 [学习文档](../../learning/m2-compat-session-resources.md)与 [ADR-0017](../../adr/0017-compat-registry-and-resource-cleanup.md)。
