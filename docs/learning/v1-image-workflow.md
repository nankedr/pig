# V1 图片工作流

```sh
pig --model deepseek-flash --api openai-responses @screen.png "解释截图"
pig --model deepseek-flash --api openai-completions @screen.png "解释截图"
```

在交互编辑器中用 `/image add /absolute/path/screen.png` 添加图片，`/image list` 查看待发送和会话中的图片，`/image remove 1` 移除待发送图片，`/image view 1` 预览。输入文字后回车会联合提交，`/image send` 可单独发送图片。预检失败恢复附件和文字；接受后取消/失败保留 Session 图片。有待发送附件时运行中的 steer/follow-up 明确拒绝，文字回到编辑器，不丢图。

macOS Apple Silicon 的 Ctrl+V 读取系统剪贴板 PNG/JPEG/GIF/static WebP，或回填纯文本。TIFF-only 等格式给出错误，此时可先保存本地 PNG 再用 `/image add`。其余平台剪贴板和 helper 回退属于 V2。

Kitty/Ghostty 的 darwin-arm64 原生环境可在预览中显示图片，Enter/Esc 关闭，窗口变化会重新适配。tmux/screen/zellij、iTerm2 或未知终端显示明确降级说明，不发送图片协议。普通 transcript 保留附件类型；`/image history 1` 查看已发送或工具获得的图片，`/image list` 给出索引。当前上下文按已有压缩规则重放；完整 Session 树中的图片仍可通过 HTML 导出查看。

RPC 使用正式 JSONL 输入：

```json
{"id":"vision","type":"prompt","message":"解释截图","images":[{"type":"image","mimeType":"image/png","data":"纯 base64"}]}
```

公开 SDK 的 `RPCClient.PromptAndWait(ctx, text, []ai.ImageContent{image})` 发送相同附件。`get_messages` 和 message_start/message_end 事件保留图片块；图片队列显式拒绝。示例见 [examples/image-workflow](../../examples/image-workflow/main.go)。用户/工具图片保存到 v3 Session 后不依赖原文件，恢复、fork、导航继续复用现有语义。

`/export /absolute/path/session.html`、RPC export_html 或 `pig --export session.jsonl session.html` 生成独立 HTML。内联用户和工具图片可以显示/放大；Markdown 远程图片不会加载，SVG/URL/损坏图片在写文件前拒绝。无需临时进程或附件目录。

离线检查：`go test ./codingagent ./cmd/pig ./tui -run ImageWorkflow`；race 加 `-race`。浏览器检查：`node parity/export-html/images.mjs` 与已有 `check.mjs`，需安装 Playwright/Chrome。固定协议 Oracle：`node --experimental-strip-types parity/oracle/image-workflow.mjs /locked/pi --check`。原生剪贴板检查：`PIG_REQUIRE_CLIPBOARD_IMAGE_NATIVE=1 go test ./codingagent -run TestImageWorkflowNativeClipboard136 -v`，fixture 会恢复原剪贴板；普通测试跳过。这些证据只覆盖 [ADR-0047](../adr/0047-image-workflow.md) 中的 V1 范围，不表示图片生成/编辑、完整终端/全平台对等完成。

2026-10-09 的 darwin-arm64 原生验收通过四种剪贴板类型和文本回填、Kitty/Ghostty 协议 harness，以及受保护 DeepSeek 两种 API 的真实 RPC“用户附件 → 识别颜色 → read 工具图片 → write → 独立 HTML”。服务冒烟用 `PIG_REQUIRE_IMAGE_WORKFLOW_LIVE=1 go test ./cmd/pig -run TestImageWorkflowRPCLive136 -v`，仅检查语义，普通离线门禁跳过。
