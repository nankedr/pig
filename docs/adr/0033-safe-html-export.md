# 安全、离线的 HTML 会话导出

Issue #97 对照 Pi `936aff00918de1187f085f123c2812d8f2d67745` 的 export-html/index.ts、template.*、export-html-xss/whitespace/skill-block 测试。CLI、ExportFromFile、AgentSession.ExportToHTML 和 RPCClient.ExportHTML 复用同一生成器，导出整棵正式 v3 会话树与当前 leaf。SDK 额外包含当前 systemPrompt 和 Tool 定义。RPC 保留 `export_html`、`outputPath`、`data.path` wire；参数和错误响应仍关联请求 id。

浏览器模板、marked 18.0.5、highlight.js 11.9.0 和固定 dark 配色来自该快照，通过 go:embed 发布进二进制与 SDK 模块。导出的单文件无需 npm、Node、CDN、Pi 安装或服务。来源、资产 hash、改动记录见 codingagent/exporthtml/manifest.json；完整 MIT/BSD notice 随源码、二进制和每个 HTML 保留。用户品牌采用 Pig；`pi-url-params` 与 `pi-share-base-url` 元数据保留为显式嵌入器 wire，宽度缓存改用 Pig key。

安全偏离：只读解析 v3 输入，不使用会迁移/重写的 OpenSessionManager；拒绝空文件、损坏 JSON、非法拓扑及未知格式。输出不得覆盖输入（包含现有 symlink/hardlink 同一文件）。SDK 在 SessionManager 锁内取一致 header/entries/leaf 快照，I/O 在锁外进行。文件名默认 `pig-session-<basename>.html`，写到进程工作目录，显式相对路径仍相对进程目录；支持 `~/`。不创建输出父目录，写入失败返回错误；显式输出可以覆盖既有非输入文件。

会话 JSON 用 base64 隔离 script 标签上下文。固定脚本通过 SHA-256 CSP 执行，内联事件改为固定监听器；read 行号参数补 HTML 转义。禁止资源和连接网络，链接由用户显式点击打开。原始 Markdown HTML 按字面显示，URL 经过 scheme 白名单与属性转义。CSP 是第二道防线，浏览器测试另行断言无注入节点/事件属性、无攻击执行、无自动 HTTP 请求。

兼容边界：消息与 ToolResult 的图片块明确失败，Markdown 图片显示 M12 提示；不声称图片已经实现。扩展 renderer 在 M7，普通自定义 Tool 仍显示参数 JSON 与文本结果，不执行扩展代码或接受预渲染 HTML。自定义主题在 M5，当前页面明确声明固定 dark 配色。未知条目、角色和内容块明确失败；合法但非显示的 custom/settings/label 条目依固定 Pi 保留在树和嵌入数据。独立迁移、SDK ExportToJSONL、托管分享和 M4 freeze/release 不在此切片完成声明中。

验证用例先从已提交 session-interop 的 pi-writer-v3 fixture 派生分支与摘要输入，再由固定 Pi 的真实 exportFromFile 和 Chromium 生成 Parity Case。Go 通过公开 CLI/SDK/RPC 验证文件与失败语义；浏览器门禁重新运行实际 pig CLI 并比较 DOM 语义和攻击结果，不以私有 helper 测试替代对等证据。
