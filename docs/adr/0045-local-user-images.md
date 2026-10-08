# 用户图片的本地输入与内联恢复

#134 在公开 `ai` SDK、`AgentSession.Prompt(Images)` 和 print/JSON headless 路径支持用户图片。只接受本地文件编码或调用者提供的纯 base64 `ImageContent`，发送 Responses `input_image`（detail=auto）或 Chat Completions `image_url`。不使用占位文字替代图片，不支持视觉的模型明确失败。

`ai.DeepSeekVisionModel()` 是 2026-10-08 官方视觉文档对应的当前 `deepseek-flash` 配置，ModelRuntime 仅在 `CreateModelRuntimeOptions.DeepSeekVision` 显式开启时在固定 Snapshot 之外加入此模型；Headless 对显式选择或恢复 deepseek-flash 自动开启。默认模型查询/循环保持固定基线。两个历史模型、Snapshot 数据和 ai 内置目录保持不变；上下文、token 上限、thinking 和成本估计暂沿用历史 Flash 配置，不宣称当前服务价格已验证。响应/视觉 smoke 是服务证据，不能替代固定 Pi Oracle。

V1 图片策略是保留原始字节、尺寸和 EXIF 方向元数据，不在本地旋转或缩放；由服务解释原图。JPEG/PNG/GIF/static WebP 经过实际解码校验，文件扩展名不决定 MIME。调用者 MIME 与实际格式不符时明确拒绝，不能传入 URL 或完整 data URL 当作 base64。限制为每图 8 MiB、40 million pixels，每请求用户图片总计 16 MiB、64 张；是 Pig 的保守本地限制，不声称等同服务最大值。GIF 校验全部帧和 trailer，最多 64 帧、总解码像素 40 million，原始文件完整传输；服务自行选择帧；animated WebP 在当前解码器不支持，返回解码错误。

production v3 使用既有 `image` 块内联保存 data/mimeType，不引入外部附件引用或新版本。删除原文件不影响重开、fork、树上下文重建与完整历史重放。正式 reader 先验证整个含图用户消息的结构，再按选中路径的最终模型，对当前 deepseek-flash 上下文在恢复前验证实际图片块和请求总配额，旧 Pi Session 的合成/原始图片编码保持既有 codec 语义（真正请求时仍进行实际图片校验），缺失或损坏数据返回带 Session 路径的附件错误；宽容 ParseSessionEntries 保留原有导入语义。已压缩掉的图片仍在原始 Session 树中，压缩后的当前上下文只按既有 first-kept 语义重放，不额外复活被压缩历史。

固定 Pi 的合法视觉图片 wire 由独立 Oracle 比较。更严格的本地格式/大小/MIME 校验及非视觉明确失败是 Pig 的显式边界，不把它们作为 Pi 对等证据。运行中的图片队列、SendUserMessage 图片、RPC/Interactive 附件、工具结果图片与 read 旋转/缩放已由 #135 处理，见 ADR-0046；终端及导出仍由 #136 处理。URL、Files API、其他厂商和图像生成/编辑逐项归入 V2，仍为显式 Stub。
