# M4.1 grep：TypeScript → Go

固定 Pi commit：`936aff00918de1187f085f123c2812d8f2d67745`。行为归属 `contract:codingagent/grep-tool`。

| Pi 路径 / 边界 | Pig 路径 / 边界 |
| --- | --- |
| `src/core/tools/grep.ts#createGrepToolDefinition` | `codingagent/grep_tool.go#CreateGrepToolDefinition`，共享执行体、schema、提示词 |
| `src/core/tools/grep.ts#createGrepTool` | `codingagent/grep_tool.go#CreateGrepTool`，接入既有 Agent 权威校验 |
| `GrepOperations` / `GrepToolInput` / `GrepToolDetails` | `codingagent/tools.go`，context/limit 用 `*float64`，可替换 stat 和文件读取 |
| `src/utils/tools-manager.ts#getToolPath/ensureTool` | `codingagent/grep_tool.go`，Pig bin/PATH 探测和可见缺失错误；自动下载明确延期 |
| `src/core/tools/truncate.ts` | grep 专属 UTF-16 行截断、共享 `TruncateHead` 的字节截断 |
| `src/core/sdk.ts` / `src/core/tools/index.ts` | `codingagent/session_services.go`、`sdk.go`、`headless.go`，显式选择与 prompt |
| `test/tools.test.ts` 的 grep 三个原始用例 | `parity/oracle/grep-tool.mjs` 扩充为确定性真实文件用例；`codingagent/issue83_grep_test.go` 在公开 Session 验证 |
| `AbortSignal` / child_process | Go `context`、进程组、`Cmd.WaitDelay`，见 `issue83_process_unix_test.go` |
| CLI 内容搜索→ToolResult→后续调用 | `cmd/pig/issue83_process_test.go` 与 `examples/grep-edit` |

上表 Pi 路径相对 `packages/coding-agent/`。行截断代理对、取消清理和自动安装的偏离见 [ADR-0022](../../adr/0022-grep-external-tool-lifecycle.md)，使用方法见 [中文学习材料](../../learning/m4-grep-tool.md)。
