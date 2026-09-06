# M3.8：写文件后继续对话

write 的执行链是模型 ToolCall → Agent 参数校验 → 文件修改队列 → 创建父目录 → 写入 UTF-8 内容 → ToolResult → 模型 continuation。read 可在下一轮读取新内容。`CreateWriteTool` 和 `CreateWriteToolDefinition` 共享执行体，后者还提供 prompt snippet 和 guideline。

无需凭证或网络的完整 SDK 示例：

```sh
go run ./examples/write-read
```

真实 Headless 使用显式工具选择；默认工具集合仍只提供 read：

```sh
go run ./cmd/pig --provider deepseek --model deepseek-v4-flash --tools read,write -p '创建 hello.txt，再读取它确认内容'
go run ./cmd/pig --provider deepseek --model deepseek-v4-flash --tools read,write --mode json -p '创建 hello.txt，再读回'
```

SDK 使用 `CreateWriteTool(cwd)`，将返回值注入 `CreateAgentSessionOptions.AgentTools`，并在 `Tools` 中选择 `write`。模型给出的参数按 JSON Schema 校验；缺少 path/content 不会隐式写入空文件。空字符串 content 则是合法的清空操作。直接调用 definition 的 `Execute` 传入 `map[string]any{"path": ..., "content": ...}`，返回 Go error；通过 Session 调用时错误成为 ToolResult，模型可继续处理。

路径继承宿主权限，无 workspace containment 或逐次审批。支持相对、绝对、`..`、`~`、`@` 前缀、file URL 与 Pi 规定的 Unicode 空格规范化。write 不使用 read 的 macOS 截图/NFD 文件名回退。已有 symlink 按宿主语义写入目标。

文件队列在单个进程内共享。注册时对存在路径执行 realpath，已有 symlink 别名共享同一 FIFO；ENOENT/ENOTDIR 回退到绝对词法路径。因此不存在的路径经过 symlink 父目录时，并不保证不同别名会合并，这与固定 Pi 一致。队列不是跨进程文件锁。

取消在每个文件操作之间检查。若 mkdir/write 已经开始，必须等操作结束后再释放队列；否则迟到的写入可能覆盖下一次操作。排队中的取消也保持 FIFO 位置，轮到时返回取消错误。取消不承诺撤销已经发生的磁盘修改。权限、目录或其他 I/O 错误均不会输出成功结果。后续 edit 可直接复用 `WithFileMutationQueue`。

成功文案沿用 Pi：`Successfully wrote N bytes to PATH`。N 实际是 JavaScript UTF-16 code unit 数；`你好🙂` 为 4，而磁盘 UTF-8 为 10 字节。内容不改换行或补末尾换行。

验证入口：

```sh
node --experimental-strip-types parity/oracle/write-tool.mjs <locked-pi-checkout> --check
go test -race ./codingagent -run '^TestWriteTool' -count=1
go test ./cmd/pig -run '^TestPigWriteReadContinuation$' -count=1
```

Oracle 直接运行固定 Pi write/read，Pig 从公开 AgentSession 重放相同输入，比较 ToolResult、read 回读和 definition 元数据。`detailsEmpty` 只比较无 details payload，保留已有 Go nil/null 与 Pi undefined 的表示差异，见 ADR-0020。并发测试通过注入文件系统操作的完成屏障验证同文件阻塞、不同文件独立、symlink 别名、取消及失败恢复；CLI 测试启动本地 Provider fixture，经真实进程验证 text/JSON 和模型上下文。

扩展宿主、交互渲染、edit 和默认完整 coding tools 集合仍以各自后续切片为准。能力状态及证据见 Parity Catalog 的 `contract:codingagent/write-tool`。
