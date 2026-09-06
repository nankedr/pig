# M3.8 TypeScript → Go：write 与文件修改队列

Code Baseline：`936aff00918de1187f085f123c2812d8f2d67745`，Issue #78。

| Pi 路径 / 入口 | Pig 路径 / 入口 |
| --- | --- |
| `packages/coding-agent/src/core/tools/write.ts`：createWriteTool、createWriteToolDefinition | `codingagent/write_tool.go`：CreateWriteTool、CreateWriteToolDefinition |
| `write.ts`：WriteOperations | `codingagent/tools.go`：WriteOperations；`write_tool.go`：localWriteOperations |
| `core/tools/file-mutation-queue.ts`：withFileMutationQueue | `codingagent/file_mutation_queue.go`：WithFileMutationQueue |
| `core/tools/path-utils.ts`：resolveToCwd | `codingagent/tools.go`：resolveToolPath（read 共用的规范化部分） |
| `core/tools/tool-definition-wrapper.ts`：wrapToolDefinition | `CreateWriteTool` → `agent.EraseAgentTool`，只接入内建执行槽 |
| Headless 显式 tools 选择 | `codingagent/headless.go`：CreateHeadlessSession |
| `test/tools.test.ts`：write tool | `codingagent/issue78_write_test.go`：Session Parity、definition 与路径、参数/宿主错误 |
| `test/file-mutation-queue.test.ts` | `codingagent/issue78_write_test.go`：队列、取消、失败、不同文件与 symlink |
| Pi Oracle | `parity/oracle/write-tool.mjs` 与 `fixtures/write-tool.json` |
| 产品验证 | `cmd/pig/issue78_process_test.go` |
| SDK 演示 | `examples/write-read/main.go` |

Go context 对应 AbortSignal；同步文件调用返回前不释放队列。Go 的共享 map/channel 尾链对应 Pi 的 Promise map；注册期间按完整 realpath 或 ENOENT/ENOTDIR 绝对路径选 key。JS `content.length` 映射 UTF-16 code unit 数，不是 Go 字节长度。

`ToolDefinition.Execute` 的早期 opaque 占位在此切片变为内建 `ToolExecuteFunc`，不传 ExtensionContext，见 ADR-0020。其他 extension ABI 槽不在此切片内。
