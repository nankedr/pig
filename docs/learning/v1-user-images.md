# V1 用户图片：发送与恢复

```sh
pig --provider deepseek --model deepseek-flash --api openai-responses --no-tools -p @./picture.png "描述这张图片"
pig --session /absolute/path/session.jsonl --no-tools -p "继续分析同一张图片"
pig --fork /absolute/path/session.jsonl --no-tools -p "在新分支继续看图"
```

`--mode json` 使用相同附件入口。多张 `@文件` 按参数顺序追加到首个 prompt，后续 prompt 从本地历史继续，不重复新增附件。`--api openai-completions` 使用同一当前视觉模型；历史 Snapshot 中 `deepseek-v4-flash`/`deepseek-v4-pro` 保留文本能力，图片会明确失败。

公开 SDK：`ai.LoadImageFile(path)` 返回 `ImageContent`，`ai.ValidateUserImage` 校验纯 base64 与实际 MIME；`ai.DeepSeekVisionModel()` 可直接配合 `ai.Complete`/`Stream` 使用。production SDK 使用 `AgentSession.Prompt(ctx, text, PromptOptions{Images: images})`，或 `RunHeadless` 的 `InitialImages`。完整例子：[examples/user-images/main.go](../../examples/user-images/main.go)。当前服务模型配置独立于固定目录；直接创建 ModelRuntime 时以 `CreateModelRuntimeOptions{DeepSeekVision: true}` 开启，Headless 显式选择/恢复该模型自动开启。

| 范围 | V1 结果 |
| --- | --- |
| JPEG / PNG / GIF / static WebP | 实际解码校验；扩展名无关；GIF 校验全部帧（最多 64 帧/总计 40 million pixels），原字节完整发送 |
| 每图大小与尺寸 | 1 byte–8 MiB，最多 40 million pixels；超限明确错误 |
| 每次请求 | 用户图片最多 64 张、总计 16 MiB；包含重放历史，base64 长度按编码上限检查 |
| MIME 与编码 | 纯标准 base64；MIME 必须匹配实际格式；不接收完整 data URL/URL |
| 方向与缩放 | 保留原尺寸和 EXIF；本地不旋转、不缩放，超限失败；read 处理见 [工具图片](v1-tool-images.md) |
| v3 保存/重开/fork/树重建 | 内联保留图片；原文件可删除；缺失/损坏 data 或 MIME 明确错误 |
| 不支持视觉的模型 | 明确失败；不产生视觉请求，不生成占位文字 |
| 运行中队列 / SendUserMessage 图片 | 显式 Stub，拒绝而不丢图 |
| Interactive / RPC prompt 附件 | #136 接通，见 [图片工作流](v1-image-workflow.md) |
| 工具结果图片 | #135 已实现，见 [工具图片](v1-tool-images.md)；终端预览与导出见 [图片工作流](v1-image-workflow.md) |
| 外部 http(s) URL | V2，未实现 |
| Files API file_id | V2，未实现 |
| animated WebP | 当前解码器不支持，明确解码错误；剩余格式处理为 V2 |
| 其他 Provider、图片生成/编辑 | V2，显式 Stub |

`go test ./ai ./codingagent ./cmd/pig ./internal/parity -run '^TestUserImages'` 不使用真实服务。fixture 服务会检查实际 wire；真实 CLI 在删除原文件后启动新进程继续及 fork。`make user-images-oracle PIG_PI_ORACLE_CHECKOUT=/locked/pi` 重现固定 Pi 共同 wire fixture，保守校验策略不在该对等投影内。

`make vision-live-smoke` 必须使用受保护 `DEEPSEEK_API_KEY`。它验证当前 `deepseek-flash` 对本地双色 PNG 的识别及 v3 重开后同图提问，按颜色语义而非逐字 golden 验收。普通测试跳过；受保护 gate 缺密钥必须失败。2026-10-08 已在 darwin-arm64 通过 `deepseek-flash` Responses 用户 PNG/v3 恢复真实视觉 smoke；不外推到工具图片、UI、其他格式服务行为或 V1 freeze。

`ai.ValidateUserImages(context)` 统一校验历史及本次用户图片的总配额，`AgentSession.Prompt` 在写入消息前使用同一策略。正式恢复先检查整个含图用户消息的结构，再按选中分支的最终模型校验当前上下文图片；其他分支的模型变化不会改变当前分支的 codec 语义。
