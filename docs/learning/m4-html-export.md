# M4.15：安全浏览 HTML 会话

运行 `go run ./examples/html-export /tmp/pig-session.html`，用浏览器打开输出文件。示例通过公开 SessionManager 写入正式 v3 用户和助手消息，再调用 AgentSession.ExportToHTML；无 Provider、Pi 安装或网络依赖。省略路径会在临时目录验证输出并清理。

| 入口 | 用法 | 输出 |
| --- | --- | --- |
| CLI | `pig --export session.jsonl result.html` | `Exported to: result.html` |
| 文件 SDK | `codingagent.ExportFromFile(ctx, input, output)` | 文件路径或错误 |
| 会话 SDK | `session.ExportToHTML(ctx, output)` | 当前 leaf、整棵树、systemPrompt 与 Tools |
| RPC wire | `{"id":"e1","type":"export_html","outputPath":"result.html"}` | `{"id":"e1","type":"response","command":"export_html","success":true,"data":{"path":"result.html"}}` |
| RPCClient | `client.ExportHTML(ctx, output)` | 从 response.data.path 返回路径 |

省略输出路径时，在进程工作目录生成 `pig-session-<输入文件名去掉.jsonl>.html`。显式相对路径也相对进程目录，支持 `~/`，不创建父目录。SDK 内存会话和尚未写盘的会话不能导出。输入只读，不能把输出指向输入本身或其文件链接。文件不存在、空文件、损坏行、非法父链、非 v3、未知消息/条目都会返回错误；RPC 写入失败后仍能查询会话。

默认显示当前分支，侧栏可以切换其他分支、过滤和搜索；ToolResult 显示在对应工具调用内，保留空白。压缩保留原历史，分支摘要显示 Markdown。HTML 内含固定 dark 样式、Markdown 解析器、代码高亮和许可，拷贝单文件即可离线浏览。页面提供整棵树的 JSONL 下载。链接只能由用户显式点击，页面不会自动加载外部图片或资源。

图片块明确返回未实现错误；Markdown 图片在页面显示 M12 提示。扩展专用 HTML renderer 等待 M7，自定义 Tool 采用参数 JSON 与纯文本回退。自定义主题等待 M5。这些范围在页面和 Parity Catalog 中明确声明，详见 [ADR-0033](../adr/0033-safe-html-export.md)。

普通离线验证：`go test ./codingagent ./cmd/pig -run '^(TestIssue97Export|TestRPC97)' -count=1`。浏览器门禁：`node parity/export-html/check.mjs`，需要预先安装 Playwright 与 Chromium；可设置 `PIG_PLAYWRIGHT_MODULE` 为模块文件绝对路径、`PIG_CHROMIUM` 为浏览器可执行文件，门禁不会自动安装。也可用 `PIG_BINARY` 指向发布二进制验证内嵌资产。`make m4-html-browser` 运行同一门禁。

固定 Pi 复核：`node --experimental-strip-types parity/oracle/export-html.mjs <locked-pi-checkout> --check`。正式 fixture 从既有 session-interop 的 pi-writer-v3 数据派生，覆盖兄弟分支、压缩和分支摘要；Chrome 观察 Markdown、代码、工具空白及切换行为。攻击门禁包含原始 HTML、关闭 script 标签、恶意 URL、属性、工具参数、模型/标签/ID 元数据。此切片不替代 #98 的全量冻结与发布验收。
