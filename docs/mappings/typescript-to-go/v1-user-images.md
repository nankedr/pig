# V1 用户图片源码映射

| 固定 Pi / 产品行为 | Pig | 证据 |
| --- | --- | --- |
| openai-responses-shared.ts#convertResponsesMessages 用户图片 | ai/openai_responses.go#responsesInput | 固定 user-images Oracle + 公开 SDK fake HTTP/SSE |
| openai-completions.ts#convertMessages 用户图片 | ai/openai_completions.go#appendOpenAIUserMessage | 相同合法 wire Oracle；指针块与混合顺序 |
| AgentSession prompt 图片 + headless 首条消息 | codingagent/session.go、headless.go、modes.go、misc.go | Prompt.Images / InitialImages、真实 @文件 print CLI |
| SessionManager v3 内联消息、fork、buildSessionContext | codingagent/session_manager.go、session_tree.go | 正式 SDK 恢复、树导航、fork + 真实 CLI 跨进程 |
| 本地实际格式和保守限制 | ai/user_images.go | JPEG EXIF 保留、PNG/GIF/WebP、无效/超限文件测试；独立 Pig 策略 |
| 当前 DeepSeek 视觉模型（超出历史目录） | ai.DeepSeekVisionModel + codingagent/model_snapshot.go overlay | parity/user-images-matrix.json；独立服务 smoke，非 Pi Oracle |

固定 Code Baseline 为 `936aff00918de1187f085f123c2812d8f2d67745`，Catalog Baseline 不变。仅合法视觉用户图片的转换语义参与 Oracle；文件处理、当前模型和严格附件错误都明确分开取证。工具结果、UI、生成/编辑和剩余输入方式仍为 Stub。
