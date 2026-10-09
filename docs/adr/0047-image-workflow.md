# V1 附件交互、预览与导出

#136 复用 #134/#135 的用户/工具 `ImageContent`、production v3 内联保存、DeepSeek 双 API 请求和 Session 生命周期，不增加外部附件引用。

Interactive 支持 CLI `@local-image` 初始附件和 `/image add <path>`、`list`、`remove <n>`、`view <n>`、`history <n>`。`/image send` 发送仅图片消息，普通文字提交携带当前全部待发送附件。预检失败或未接受的取消恢复附件和文字；接受后的失败/取消沿用 Session outcome，附件保留在历史。运行中的新附件独立保留；有待发送附件时文本 steer/follow-up 明确拒绝并恢复文字。路径是显式本地输入，遵循 read 的宿主路径权限，不引入工作区 containment。新命令是 Pig 的明确入口，不能当作 Pi CLI 命令对等。

V1 darwin-arm64 Ctrl+V 通过 CGO-free purego 调用 NSPasteboard；优先 PNG/JPEG/GIF/static WebP，纯文本回填编辑器。无内容、错误格式、超限和 TIFF-only 给出可解释错误；不启动 helper。TIFF 转换、Linux/Windows helper 和完整回退矩阵归 #15/#128。原生验收 fixture 暂存并恢复原剪贴板的全部 item/type 数据，普通离线测试不碰系统剪贴板。

预览是一个有界 dialog，原图保持不变，显示应用 EXIF 方向后转换 PNG。仅 darwin-arm64 的明确 Kitty/Ghostty 环境且没有 tmux/screen/zellij 时发送 Kitty 协议；其他情况显示降级说明，提供附件来源、内联历史索引和 HTML export。预览最多 60 列/10 行，使用保守 8×16 cell 比例；resize 后适配剩余行，重新上传并只删除自身随机 image ID。关闭、取消、停止删除自身资源并恢复编辑器；背景文字重绘不会裁切图像序列。预览中滚动键不修改 dialog；关闭后既有 transcript 滚动恢复。普通 transcript 显示附件 MIME/内联标识，工具图片由 `/image history <n>` 查看。不宣称自动内联 scrollback、iTerm2、动态 cell 查询和全平台图像缓存对等；这些保持 V2 Stub。

RPC prompt.images 只接受数组和合法 ImageContent，空闲请求在正式预检后回执；事件、get_messages 和持久化保留原字节/身份。steer/follow_up 和运行中图片 prompt 保持明确 Stub，不转成文字。CLI RPC 的 @FILE 仍拒绝，附件通过 JSONL images 传入。

HTML export 对整个树中的内联图片做格式、MIME、base64、尺寸/字节验证，仅 PNG/JPEG/GIF/static WebP；CSP 的 img-src 从 none 改成 data:，仍无 file/http/https 资源。JS 再限制 MIME 与 base64，用户和所有工具附件可显示及点击放大；Markdown 图片继续显示降级说明，不读取本地路径或 URL。数据内嵌在 HTML，删除原文件、结束进程后仍可展示；错误输入在写输出前拒绝。扩展自定义 HTML、图像生成/编辑不在范围内。

固定 Pi Oracle 只证明 Kitty 分块、placement、删除控制序列；已有 user/tool 双 API Oracle 继续覆盖模型 wire。图片处理、本地严格校验、剪贴板入口和 dialog 布局是已声明的 Pig V1 选择。浏览器安全、真实 CLI/RPC、公开 SDK/PTY、原生剪贴板证据分别记录，不以当前服务响应代替固定基线。
