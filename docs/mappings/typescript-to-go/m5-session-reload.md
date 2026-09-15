# M5.7 TypeScript → Go：Session 资源重载

固定基线：`936aff00918de1187f085f123c2812d8f2d67745`。

| Pi | Pig | 观察点 |
| --- | --- | --- |
| `core/agent-session.ts#reload` | `codingagent/session_reload.go` 的 `AgentSession.Reload(ctx)` | 设置、队列模式、资源、system prompt 顺序；保留会话 |
| `syncQueueModesFromSettings` | `SettingsManager.GetSteeringMode/GetFollowUpMode` → `Agent.Set*Mode` | 失败时已完成阶段仍可查询 |
| `core/resource-loader.ts#reload` | `codingagent/resources.go` 的 `DefaultResourceLoader.Reload` | 文件加载、信任过滤、一次发布 |
| `_rebuildSystemPrompt` / `_buildRuntime` 的本地资源部分 | `codingagent/headless.go#configureSessionPrompt` | 重用当前活动 Tool，刷新资源输入 |
| `promptTemplates` / `_expandSkillCommand` | `AgentSession.PromptTemplates` / `codingagent/skills.go` | 后续生成展开新版命令 |
| 公开 SDK 的活动生成和 reload | `codingagent/issue106_reload_test.go` | 使用 Faux Provider 实际继续，读取 Oracle fixture |
| Promise 拒绝 | 返回 `error`，并接受 `context.Context` | 取消/并发接纳偏离见 ADR-0035 |

`parity/oracle/session-reload.mjs` 使用固定 Pi 公开 SDK 生成 fixture；归一化临时目录、产品名和 Pi 安装目录文档提示，资源正文和对话内容不改写。`codingagent/issue106_lifecycle_test.go` 验证取消、失败恢复、信任、分支持久化和查询竞态；API 快照记录已发布入口，未增加 RPC wire 命令。

扩展重建、session shutdown/start hooks、trust hooks、全局 Provider registry reset 没有随本地资源重载实现；Pig 保留现有模型/Provider 和 Tool 状态，不声称扩展 ABI 对等。包生态继续由 #99 跟踪。
