# M3.9 TypeScript → Go：edit 与 diff

固定 Code Baseline：`936aff00918de1187f085f123c2812d8f2d67745`，Issue #79。

| Pi 路径 / 入口 | Pig 路径 / 入口 |
| --- | --- |
| `core/tools/edit.ts`：createEditTool、createEditToolDefinition | `codingagent/edit_tool.go`：CreateEditTool、CreateEditToolDefinition |
| `edit.ts`：prepareEditArguments | `edit_tool.go`：prepareEditArguments，经 `agent.PrepareArgumentsFunc` 接入校验前阶段 |
| `edit.ts`：EditToolInput、EditToolDetails、EditOperations | `codingagent/tools.go`：同名 Go 类型，JSON tags 与必需参数校验 |
| `core/tools/edit-diff.ts`：applyEditsToNormalizedContent、normalizeForFuzzyMatch | `codingagent/edit_match.go`：applyFileEdits、normalizeEditFuzzy |
| `edit-diff.ts`：applyReplacementsPreservingUnchangedLines | `edit_match.go`：preserveUnchangedEditLines，按实际替换范围分组 |
| `edit-diff.ts`：generateDiffString、generateUnifiedPatch | `codingagent/edit_diff.go`：editDiffDetails、editUnifiedPatch |
| 基线依赖 `diff@8.0.4`：Myers diffLines、structuredPatch | `edit_diff.go`：editLineDiff，保留路径选择和 tie breaking；BSD-3-Clause 许可已保留 |
| `core/tools/file-mutation-queue.ts` | `codingagent/file_mutation_queue.go`：与 write 共用 WithFileMutationQueue |
| `fs.access(R_OK \| W_OK)` | `codingagent/edit_access_unix.go`；其他平台为可读写打开检查，尚无行为门禁声明 |
| `test/tools.test.ts`、`test/edit-tool-legacy-input.test.ts` | `codingagent/issue79_edit_test.go`、`issue79_paths_test.go`、`issue79_queue_test.go` |
| Oracle | `parity/oracle/edit-tool.mjs`、`fixtures/edit-tool.json` |
| SDK / CLI 验证 | `examples/edit-read/main.go`、`cmd/pig/issue79_process_test.go` |

Go 使用 UTF-8 字节偏移定位合法 Unicode 子串；空的模糊 needle 计数保留 Pi 的 UTF-16 split 语义。NFKC 使用已有 x/text 依赖；行尾裁剪采用 ECMAScript 空白集合，避免把 U+0085 当作 JS trimEnd 空白。多区域编辑不逐项修改匹配基底；只有全部校验成功才写入。diff 使用不可变路径链保留 jsdiff 分支历史，未添加运行时 Node 依赖。

ToolDefinition 的 Execute 与 PrepareArguments 是内建工具的现有 Agent 入口，renderCall/renderResult 仍为 opaque。未实现的预览和扩展宿主不由此切片启用。
