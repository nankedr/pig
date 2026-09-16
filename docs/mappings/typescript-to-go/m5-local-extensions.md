# M5.8：本地扩展入口导航

| 固定 Pi 路径 / 概念 | Pig 入口 | 边界 |
| --- | --- | --- |
| `core/package-manager.ts`：`resolve`、本地 extensions | `local_extensions.go`：本地文件发现 | 不调用包管理 API，不读取 manifest，不获取 npm/git |
| `core/resource-loader.ts`：`reload`、`additionalExtensionPaths` | `DefaultResourceLoader.Reload`、`AdditionalExtensionPaths` | 仅刷新数据快照，项目读取受信任门控制 |
| `ResolvedResource` 的 path/enabled/metadata | `ExtensionDiscoveryResult.Entries` | Enabled 表示选择；显式 origin 为 top-level，Pi PackageManager 原始值为 package |
| `getExtensions`、`extensions/loader.ts` 的加载入口 | `GetExtensions`、`DiscoverAndLoadExtensions` | 保持结构化未实现错误；新 `GetExtensionDiscovery` 只查询入口 |
| CLI `-e`、`--no-extensions`、未知长 flag | `Main`、`reportCLIExtensionDiscovery`、`RunCLI` | 显式执行非零退出；未知 flag 顺序不变 |
| 扩展 runtime、工厂、trust hook、资源回调 | `extensions.go` 不透明载体及资源 Stub | M7 研究/ADR 之前不冻结语言或加载 ABI |

源码基线 `936aff00918de1187f085f123c2812d8f2d67745`；[失败用例](../../../parity/oracle/fixtures/local-extensions.json)、[SDK 验收](../../../codingagent/issue107_extensions_test.go)、[CLI 验收](../../../cmd/pig/issue107_extensions_test.go)、[可运行示例](../../../examples/local-extensions/main.go)与[学习材料](../../learning/m5-local-extensions.md)。
