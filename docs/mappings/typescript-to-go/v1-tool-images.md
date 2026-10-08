# V1 工具图片源码映射

| 固定 Pi 制品 | Pig 实现 | 证据与边界 |
| --- | --- | --- |
| `core/tools/read.ts#createReadTool` | `codingagent/tools.go`, `tool_images.go` | 图片先于文本分页；既有可选 ReadImageOperations，BMP 转换、失败文本、取消 |
| `utils/image-process.ts`, `image-resize-core.ts`, `exif-orientation.ts` | `codingagent/tool_images.go#ResizeImage` | 默认 2000×2000 / 4.5MiB base64，8 方向；Go 原生 codec，重编码字节不声明相同 |
| `api/openai-responses-shared.ts#convertToolResultOutput` | `ai/openai_responses.go#responsesInput` | function_call_output 图文；固定 Oracle |
| `api/openai-completions.ts#convertMessages` | `ai/openai_completions.go` | 连续 ToolResult 图片按序批量追加 user 消息；固定 Oracle；DeepSeek 窄分支 |
| `agent-loop.ts` 工具更新/终态/并行批次 | 既有 `agent/tool_runtime.go` / `agent/event.go` | 复用，无新循环；`agent/tool_images_test.go` 验证图片生命周期 |
| `agent-session.ts`, `session-manager.ts` | `codingagent/session_manager.go` / 既有 Session 服务 | v3 内联恢复、fork、压缩；`tool_images_session_test.go`；正式 reader 扩至 toolResult |

公开 API 签名保持不变，既有 Go API snapshot 持续回归；`TestToolImagesCatalog` 额外锁定 ResizeImage 签名。生产文本队列与 RPC 用户附件保持明确 Stub，不将底层 Agent 图片队列当作产品入口证据。用户文件策略仍见 ADR-0045；处理/非视觉/坏图差异见 ADR-0046。Catalog `contract:codingagent/tool-images` 是本票证据锚点，交付归属 V1；其余图片体系归 V2。
