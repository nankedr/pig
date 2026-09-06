# find/ls：TypeScript → Go 导航

固定 Pi：`936aff00918de1187f085f123c2812d8f2d67745`。

| Pi | Pig | 验证入口 |
| --- | --- | --- |
| `core/tools/find.ts#createFindToolDefinition/createFindTool` | `codingagent/find_tool.go` | `TestFindLsSessionParity`、`TestFindLsDefinitionOperationsAndCancellation` |
| `core/tools/ls.ts#createLsToolDefinition/createLsTool` | `codingagent/ls_tool.go` | `TestFindLsSessionParity`、`TestFindLsPathsAndByteTruncation` |
| `FindOperations`、`LsOperations` | `codingagent/tools.go` 已发布接口 | `TestIssue84FindLsAPISnapshot` |
| `find.ts#relativizeFindResultPath` | find definition 中的 `filepath.Rel` / `filepath.ToSlash` | Oracle 的 `/` 根目录、自定义相对/绝对路径与目录后缀 |
| `utils/tools-manager.ts#getToolPath/ensureTool` | `find_tool.go#findBinary` | `TestFindLsFDPlatformContract`，缺失时禁止下载 |
| `child_process.spawn` / AbortSignal | `exec.CommandContext` / context + Wait | `TestFindLsFDCancellationWaitsForExit` |
| `core/tools/index.ts`、SDK 显式选择 | `session_services.go#createCodingTools`、`sdk.go` | `TestFindLsExplicitSDKReadContinuation`、`TestPigFindLsReadContinuation` |
| `tools.test.ts`、regressions #3302/#3303/#6104 | `parity/oracle/find-ls.mjs` / fixture | `TestFindLsSessionParity` 经公开 Session 继续生成 |

TypeScript 的可选 `details` 对应 Go nil；错误由已有 Agent dispatcher 转为 ToolResult。Go `FindGlobOptions.Limit` 为 int，无法表示的自定义分数参数明确报错。本机 fd 数字参数、ls 数字比较保留 Pi 语义。English collation 是当前已验证排序分支，其他 locale 和跨平台运行尚未宣称完成。

[学习材料](../../learning/m4-find-ls.md) · [平台决策](../../adr/0022-find-ls-platform-contract.md) · [可运行示例](../../../examples/find-ls-read/main.go)
