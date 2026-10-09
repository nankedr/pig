# V1 图片交互源码映射

| 固定 Pi 基线 | Pig | 范围 |
| --- | --- | --- |
| modes/interactive/interactive-mode.ts initialImages / pasteImage | codingagent/interactive.go、interactive_images.go、clipboard_darwin_arm64.go | 本地附件管理、联合提交、失败/取消与纯文本队列边界；Pig 新增 /image 命令 |
| utils/clipboard-image.ts / clipboard-native.ts | codingagent/clipboard_darwin_arm64.go | CGO-free NSPasteboard 支持类型；TIFF/helper/其余平台 V2 |
| tui/src/terminal-image.ts encodeKitty / deleteKittyImage | tui/image.go | 固定 Pi 分块/placement/删除 Oracle；附加严格输入校验 |
| tui terminal-image / interactive tool images | tui/image_preview.go、codingagent/interactive_image_preview.go、transcript.go | Kitty/Ghostty 有界预览；保守 cell 比例，其他协议/自动图片 scrollback V2 |
| modes/rpc/rpc-mode.ts prompt.images | codingagent/rpc_mode.go | dual API prompt、事件和 get_messages 内联图片；队列拒绝 |
| core/export-html/template.js user/tool images | codingagent/export_html.go、exporthtml/template.js | 格式验证、data-only CSP、内联图片/模态查看，既有 XSS/path 边界 |

用户与工具处理沿用 [V1 用户图片](v1-user-images.md) 和 [V1 工具图片](v1-tool-images.md)。固定 Kitty 序列 fixture：parity/oracle/fixtures/image-workflow.json；其他行为和服务证据独立记录。
