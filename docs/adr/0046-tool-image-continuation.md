# 工具图片进入 production Agent 循环

#135 复用 Agent 的 text/image ToolResult、更新/终态事件和 production v3 内联 codec。工具消息保留原始块顺序、调用 ID 和图片字节；更新仅作为事件投影，下一轮模型上下文使用终态。正式 reader 对含图 toolResult 先做消息结构检查，再沿既有选中模型/分支规则校验图片与请求总配额。

Responses 将工具文本按顺序合并为一个 input_text，再按图片顺序生成 input_image，组成 function_call_output.output；无图片时沿用字符串。Chat Completions 将连续工具结果的文本保留为各自 tool 消息，图片按工具与块顺序合并到紧随其后的 user 附件消息。仅 DeepSeek 窄分支可用。此 wire 分组是固定 Pi 行为，并不改变原始 Session 块顺序。

read 按实际内容识别 JPEG/PNG/GIF/static WebP/BMP；BMP 转 PNG；offset/limit 对图片无效。默认 autoResizeImages=true，应用 JPEG/WebP EXIF 方向，在 2000×2000 和 4.5 MiB base64 内适配；候选顺序是 PNG、JPEG quality 80/85/70/55/40，再按 0.75 缩小。返回 Pi 坐标映射说明。未触发尺寸或字节限制时保留原字节（包括 EXIF 和 GIF 多帧）；关闭缩放时保留支持格式原字节，BMP 仍转换。公开 ResizeImage 沿用既有签名。

原生 Go 使用 CatmullRom 与 Go PNG/JPEG 编解码，Pi 使用 Photon/Lanczos3。验收投影包括 MIME、尺寸、角落颜色/方向、提示、未改图片的精确字节；转换/缩放后的编码字节不声明相同。JPEG/WebP 的其余 EXIF、ICC 等元数据在重新编码后不保留；小图原样传输仍保留。GIF 缩放后静态化，与 Pi 的第一帧处理相符。animated WebP 的剩余处理归 V2。

Pig 继续使用每图 8 MiB/40 million pixels、每请求用户+工具图片总计 16 MiB/64 张的保守本地限制。损坏、MIME 不匹配、超限或无法转换/缩放的 read 结果包含明确 Image omitted 文本；I/O 失败和取消沿用工具错误与 Session 语义。与 Pi 关闭缩放时可透传坏图的行为不同，Pig 总是验证实际图片。无视觉能力的模型在模型请求前明确失败，保留原 Session 图片；不使用占位文字声称理解图片。未知 Provider、URL、Files API、图像生成/编辑、Harness v4 和 Remote 集成仍为 V2 Stub。

生产 AgentSession 的 Steer/FollowUp/TakeQueuedMessages 为文本接口；运行中 Prompt.Images 和 SendUserMessage 的 image 块继续显式拒绝，不入队、不转文本、不丢图。底层 Agent.Steer/FollowUp 可携带完整 AgentMessage，但不等于产品图片队列完成。RPC 可执行读图工具；#136 已接入 RPC 用户附件和交互/预览/导出，见 ADR-0047。

恢复/fork/导航保留工具图片；压缩只重放 first-kept 之后的上下文，已移除图片仍存在原 Session 树，不在下一轮复活。重试重放终态图片，不重跑已完成的工具。取消不把 partial/update 当作终态。并行 ToolEnd 保持完成次序，持久化 ToolResult 保持 Assistant 调用次序。
