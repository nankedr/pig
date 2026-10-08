# V1 工具图片：读截图后继续编码

```sh
pig --model deepseek-flash --api openai-responses --thinking off -p "用 read 读取 screen.png，然后修改代码"
pig --model deepseek-flash --api openai-completions --thinking off -p "用 read 读取 screen.png，然后修改代码"
pig --session /absolute/path/session.jsonl -p "继续针对同一截图修改"
```

`CreateReadTool(cwd, ReadToolOptions{AutoResizeImages: &enabled})` 支持图片。默认缩放开启；自定义 ReadOperations 只有实现可选 ReadImageOperations 才会将返回字节作为图片处理。工具终态可返回交错 `TextContent`/`ImageContent`，Agent 保留原始顺序，按对应 API 的固定 Pi 编码继续请求。公开 SDK 示例：[examples/tool-images/main.go](../../examples/tool-images/main.go)。RPC 从文本 prompt 调用 read，图片在事件和 `get_messages` 中保留；用户 RPC 附件仍为 #136 Stub。

read 识别实际 JPEG/PNG/GIF/static WebP/BMP（无关扩展名），BMP 转 PNG。默认尺寸 2000×2000，base64 小于 4.5 MiB；重编码时应用 EXIF 方向，返回原尺寸/显示尺寸/坐标换算说明。小图保留原字节，关闭缩放时保留支持格式原图；损坏、超限、无法处理时给出明确 omitted 提示。格式、请求限额及编码差异详见 [ADR-0046](../adr/0046-tool-image-continuation.md) 和 `parity/tool-images-matrix.json`。

用户+工具图片合并计入每请求 64 张/16 MiB 限额。非视觉模型明确失败，原历史不变。生产 steer/followUp 图片尚不可用，`Prompt(Images, StreamingBehavior)` 和 `SendUserMessage` 明确报错；文本队列不会接受图片。工具图片不经过这些队列。

v3 内联数据允许删除截图原文件后跨进程重开、fork 和树重建。压缩后的当前上下文按 first-kept 重放，不自动复活已压缩图片；原 Session 树保留图片。取消、更新/终态、并行结果次序及重试都沿用既有 Agent/Session 语义。

离线检查：`go test ./ai ./agent ./codingagent ./cmd/pig ./internal/parity -run '^TestToolImages'`。`make tool-images-oracle PIG_PI_ORACLE_CHECKOUT=/locked/pi` 重现固定 Pi 双 API wire 与 42 个 read 处理用例。`make tool-vision-live-smoke` 使用受保护 `DEEPSEEK_API_KEY`，验收实际 read 彩色 PNG → 两种 API 视觉识别 → write 颜色文件，按颜色语义检查，不使用模型输出精确 golden。
