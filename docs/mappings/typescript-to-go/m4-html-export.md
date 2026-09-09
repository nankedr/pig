# HTML export 的 TypeScript → Go 导航

固定 Pi commit：`936aff00918de1187f085f123c2812d8f2d67745`。

| Pi 入口/测试 | Pig | 证据 |
| --- | --- | --- |
| main.ts 的 export 分派、exportFromFile | RunCLI → ExportFromFile，codingagent/export_html.go | issue97_export_test.go 与真实 CLI 浏览器门禁 |
| agent-session.ts exportToHtml | AgentSession.ExportToHTML，锁内读取 v3 树快照 | SDK 当前 leaf 与源文件保护 |
| rpc-mode.ts export_html、RpcClient.exportHtml | rpc_mode.go、rpc_client_lifecycle.go | cmd/pig/issue97_export_test.go |
| export-html/template.*、vendor | codingagent/exporthtml，通过 go:embed 发布 | manifest.json 来源/资产 hash、完整许可 |
| export-html-xss.test.ts | parity/export-html/check.mjs | 真实浏览器攻击，含基线测试未发现的 read offset 注入 |
| export-html-whitespace / skill-block | 固定模板 Markdown、工具空白与技能展示 | 固定 Pi DOM fixture；扩展 TUI→ANSI pipeline 延至 M7 |
| SessionManager.open | ExportFromFile 使用只读 v3 校验 | 不隐式迁移或覆盖输入；非法结构明确失败 |

Go 保留现有 ExportToHTML/ExportHTML 签名，另公开 ExportFromFile 文件入口。ExportOptions 的主题/扩展 renderer 未宣称映射完成；图片保留 M12，主题 M5，扩展 M7。会话与响应中的 wire 标识保留。参见[学习材料](../../learning/m4-html-export.md)和[ADR-0033](../../adr/0033-safe-html-export.md)。
