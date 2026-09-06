# M3.5 TypeScript → Go：Project Trust

固定 Pi commit：`936aff00918de1187f085f123c2812d8f2d67745`。学习入口：[Project Trust 与项目设置](../../learning/m3-project-trust.md)。

| Pi 源码 / 测试 | Pig 实现 / 证据 |
| --- | --- |
| `src/core/trust-manager.ts`：resource probe、ProjectTrustStore | `codingagent/trust.go`；`internal/parity/project_trust_test.go`、`codingagent/issue75_trust_test.go` |
| `src/core/project-trust.ts`：resolveProjectTrusted | `codingagent/headless_settings.go`：prepareHeadlessProjectSettings；`codingagent/issue75_headless_test.go` |
| `src/main.ts`：启动信任判定与 Session 选择 | `codingagent/misc.go`、`headless.go`；`cmd/pig/issue75_process_test.go` |
| `src/core/settings-manager.ts`：project scope、setProjectTrusted、deepMergeSettings | `codingagent/settings_manager.go`；`codingagent/issue75_settings_test.go` |
| `src/core/resource-loader.ts`：loadProjectContextFiles、worktree 去重 | `codingagent/context_files.go`；`codingagent/issue75_context_test.go` |
| `test/trust-manager.test.ts`、`test/resource-loader.test.ts` | 两个 `parity/oracle/project-trust*.mjs` 与对应 fixture；公开 SDK 和真实 CLI 回放 |

Go `ProjectTrustDecision` 用 `*bool` 表示 true / false / nil。持久决定使用 `Set` / `SetMany`，本次决定用 Headless 的 `ProjectTrustOverride`。TUI 选择器和 pre-trust extension hook 未提前移植。

Pig 的 settings 构造默认 untrusted，并在 Headless Session 目录选择之前完成 trust；固定 Pi 在该位置会窄读项目 `sessionDir`，此差异遵循 ADR-0010。trust store 的只读空路径不创建状态，写入保持 `0700` / `0600`；正式 store 不读取 Pi 状态。

Catalog 行为归属为 `contract:security/project-trust` 和 `contract:config/settings`。API 快照见 `codingagent/testdata/issue75_surface_golden.txt`，Headless 参数变更也更新 #71/#74 快照。可运行示例是 `examples/project-trust`。
