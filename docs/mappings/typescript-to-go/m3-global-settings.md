# M3.4 TypeScript → Go：全局 settings

| 固定 Pi 入口 | Pig 入口 | 阅读重点 |
| --- | --- | --- |
| `core/settings-manager.ts` Settings / PackageSource | `codingagent/settings.go`、`settings_json.go` | 可选值、空数组、稀疏对象、未知字段与联合类型 |
| SettingsManager.create / fromStorage / inMemory | NewSettingsManager / NewSettingsManagerFromStorage / NewInMemorySettingsManager | 只加载全局配置 |
| deepMergeSettings / migrateSettings | `codingagent/settings_manager.go` | 对象递归合并、数组替换和迁移 |
| FileSettingsStorage.withLock / persistScopedSettings | fileSettingsStorage.WithLock / SettingsManager.set | 锁内重读，按修改字段合并，错误收集 |
| SettingsManager getters / setters | `codingagent/settings_accessors.go` | 数据默认值不等于运行时能力启用 |
| `main.ts` sessionDir 与 buildSessionOptions | `codingagent/misc.go` runHeadlessMain | 命令行、环境变量和设置的优先级 |
| `core/sdk.ts` 模型与 thinking 恢复 | `codingagent/headless_settings.go` | 显式模型、Session、默认值与能力 clamp |
| `test/settings-manager-bug.test.ts` / nested retry regression | `codingagent/issue74_settings_test.go` | 保留外部修改、并发进程与损坏配置 |
| 真实 Pi CLI | `cmd/pig/issue74_process_test.go` | 同一 Oracle fixture 验证新建、退出、重启和覆盖 |

可运行示例：`examples/global-settings/main.go`。行为登记在 Parity Catalog 的 `contract:config/settings`；学习材料见 [全局设置](../../learning/m3-global-settings.md)。
