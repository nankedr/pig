# 交互维护命令映射

| 固定 Pi 基线 | Pig 公开边界与实现 | 证据 |
| --- | --- | --- |
| handleCompactCommand / compaction events | InteractiveMode.Run → AgentSession.Compact；进度、结果与 Context 取消 | maintenance-cli.json；CLI 后续 Provider 上下文、v3 压缩记录 |
| handleReloadCommand | Run → AgentSession.Reload → 键绑定、设置、主题、自动补全与当前诊断 | CLI 模板/Skill/Context File；公开 SDK 慢重载取消与退出 |
| handleSessionCommand | GetSessionStats / SessionStats.ContextUsage | CLI 当前上下文、累计用量；API snapshot |
| handleExportCommand | AgentSession.ExportToHTML | CLI 引号路径、嵌入会话数据、错误后继续 |

对等基线 `936aff00918de1187f085f123c2812d8f2d67745`。新交互入口复用原有 SDK，无新增业务 API。取消错误与压缩期间输入回填属于 ADR-0042 的显式偏离；JSONL 导出、托管分享、按模型统计细分、扩展与六平台不列为 verified。
