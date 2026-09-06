# M3.6 TypeScript → Go：凭证恢复

Pi 固定 commit：`936aff00918de1187f085f123c2812d8f2d67745`。

| Pi 路径 / 边界 | Pig 路径 / 边界 | 验证 |
| --- | --- | --- |
| `packages/ai/src/auth/credential-store.ts` | `ai/credential.go`，CredentialStore / APIKeyCredential.Extra | 原始格式、未知字段精度 |
| `packages/coding-agent/src/core/auth-storage.ts`，AuthStorage | `codingagent/auth_storage.go`，NewAuthStorage、Read / Modify / List / Delete | `internal/parity/credentials_test.go`、跨进程写锁 |
| 同文件 readStoredCredential | `codingagent.ReadStoredCredential` | 原始引用不执行 |
| `core/resolve-config-value.ts` | `codingagent/credential_value.go` | 模板、转义、命令缓存、10 秒 timeout |
| `core/runtime-credentials.ts` 与请求 key override | `headlessCredentials` / Models request overrides | CLI key > store > 环境；已有 entry 失败不换身份 |
| `config.ts` 的 getAgentDir 与 pi-ai CLI auth path | `internal/statepath`、`codingagent.ResolveAuthPath`、`internal/pigaicli.ResolveAuthPath` | ADR-0008 Pig 路径偏离 |
| `core/model-runtime.ts` 的启动认证链 | `codingagent.CreateHeadlessSession` 接入 ai.Models | `cmd/pig/issue76_process_test.go` |
| `test/auth-storage.test.ts` / `test/resolve-config-value.test.ts` | `codingagent/issue76_credentials_test.go` | 并发、取消、缓存、权限、损坏文件 |

Oracle：`parity/oracle/credentials.mjs` 从固定 Pi 实现执行实际文件 CRUD 和引用解析，生成 `fixtures/credentials.json`。运行 `node --experimental-strip-types parity/oracle/credentials.mjs .upstream/pi --check` 检查无漂移。目录权威为 `contract:config/auth-json`；API snapshot 为 `codingagent/testdata/issue76_surface_golden.txt`。

学习入口：`docs/learning/m3-credentials.md`。示例：`examples/credentials/main.go`。ReadOnly facade、ModelRuntime、OAuth/ambient 和 pig-ai 凭证运行命令不在本切片实现范围。
