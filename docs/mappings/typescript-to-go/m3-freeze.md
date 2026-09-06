# M3 冻结源码导航

Pi 路径相对于 Code Baseline `936aff00918de1187f085f123c2812d8f2d67745`。本表只负责导航，支持范围与精确证据以 [Parity Catalog](../../../parity/catalog.jsonl) 为准。

| 切片 | Pi 源码（`packages/coding-agent/src/`） | Pig 公开入口 | 证据与 Go 示例 |
| --- | --- | --- | --- |
| M3.1 持久化 | `core/session-manager.ts` | `NewSessionManager`、`OpenSessionManager`、`RunHeadless` | `internal/parity/session_persistence_test.go`、`cmd/pig/issue71_process_test.go`；`examples/session-persistence` |
| M3.2 历史恢复/互操作 | `core/session-manager.ts`、`core/messages.ts` | `OpenSessionManager`、`BuildSessionContext`、`ConvertToLLM` | `parity/oracle/session-interop.mjs`、`internal/parity/session_interop_test.go`、`cmd/pig/issue72_process_test.go`；`examples/session-interop` |
| M3.3 继续/fork | `core/session-manager.ts`、`core/agent-session.ts` | `ContinueRecentSessionManager`、`SessionManager`、CLI `--continue/--fork` | `internal/parity/session_navigation_test.go`、`session_tree_test.go`、`cmd/pig/issue73_process_test.go`；`examples/session-navigation` |
| M3.4 全局 settings | `core/settings-manager.ts` | `SettingsManager`、`CreateHeadlessSession` | `internal/parity/global_settings_test.go`、`cmd/pig/issue74_process_test.go`；`examples/global-settings` |
| M3.5 Project Trust | `core/trust-manager.ts`、`core/resource-loader.ts` | `ProjectTrustStore`、`CreateHeadlessSession` | `internal/parity/project_trust_test.go`、`cmd/pig/issue75_process_test.go`；`examples/project-trust` |
| M3.6 凭证 | `core/auth-storage.ts`、`config.ts` | `AuthStorage`、`ResolveAuthPath`、`ai.CredentialStore` | `internal/parity/credentials_test.go`、`codingagent/issue76_credentials_test.go`；`examples/credentials` |
| M3.7 ModelRuntime | `core/model-runtime.ts`、`core/model-registry.ts`、`core/model-resolver.ts` | `NewModelRuntime`、`ModelRegistry` | `internal/parity/model_runtime_test.go`、`cmd/pig/issue77_process_test.go`；`examples/model-runtime` |
| M3.8 write | `core/tools/write.ts` | `CreateWriteTool`、`CreateWriteToolDefinition` | `codingagent/issue78_write_test.go`、`cmd/pig/issue78_process_test.go`；`examples/write-read` |
| M3.9 edit | `core/tools/edit.ts`、`core/tools/edit-diff.ts` | `CreateEditTool`、`CreateEditToolDefinition` | `codingagent/issue79_edit_test.go`、`cmd/pig/issue79_process_test.go`；`examples/edit-read` |
| M3.10 bash | `core/tools/bash.ts` | `CreateBashTool`、`CreateBashToolDefinition` | `codingagent/issue80_bash_test.go`、`cmd/pig/issue80_signal_unix_test.go`；`examples/bash-read` |
| M3.11 默认四工具 | `core/tools/index.ts`、`core/sdk.ts` | `CreateCodingTools`、`CreateAgentSession`、`CreateHeadlessSession` | `codingagent/issue81_tools_test.go`、`cmd/pig/issue81_process_test.go`；`examples/coding-task` |

Go 入口均在 `codingagent`，另有标注包名的例外。固定 Oracle 脚本位于 `parity/oracle/`，Go API snapshots 位于 `codingagent/testdata/issue71`–`issue81` 对应 golden 文件。原有 read、Headless 与 M1/M2 行为继续按 [M1 导航](m1-headless-text.md) 和 [M2 导航](m2-freeze.md) 回归。

门禁、证据范围与安装命令见 [M3 集成与冻结](../../learning/m3-freeze.md)。
