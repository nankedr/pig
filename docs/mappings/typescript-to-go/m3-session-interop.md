# M3.2 TypeScript → Go 导航

固定基线：`936aff00918de1187f085f123c2812d8f2d67745`。能力状态以 Parity Catalog 为准。

| Pi 实现/测试 | Pig 边界 | 证据 |
| --- | --- | --- |
| `core/session-manager.ts#loadEntriesFromFile`、`SessionManager.open` | `OpenSessionManager` | `codingagent/issue72_session_limits_test.go`；逐行无固定大小上限 |
| `migrateV1ToV2`、`migrateV2ToV3`、`_setSessionFile` | `MigrateSessionEntries`、`OpenSessionManager` | `internal/parity/session_interop_test.go`；打开迁移、写回和重开 |
| `session-manager/migration.test.ts`、`file-operations.test.ts` | 显式历史文件 → AgentSession → Provider → v3 文件 | `cmd/pig/issue72_process_test.go` |
| `buildSessionContext`、`sessionEntryToContextMessages`、`session-manager/build-context.test.ts` | `BuildSessionContext`、`BuildContextEntries`、`SessionEntryToContextMessages` | `codingagent/issue72_session_restore_test.go`；既有 summary 到 Provider |
| `getLabel`、`getSessionName` | `SessionManager.GetLabel`、`GetSessionName` | `internal/parity/session_interop_test.go` |
| `SessionManager.create` / `append*` / `open` | `examples/session-interop` 与生产 writer/reader | `parity/oracle/session-interop.mjs` 双向正式实现 fixture |

API snapshot：`codingagent/testdata/issue72_surface_golden.txt`。Go carrier 的字段名按已登记的 Go API 映射到 Pi 属性；固定字段和值不做归一化。JSONL 原始记录保持开放，生产 Session v3 与 Harness Session v4 仍是独立边界。
