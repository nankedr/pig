# M5 本地资源：TypeScript → Go

| 固定 Pi 入口 | Pig 入口 | 学习材料 |
| --- | --- | --- |
| `core/resource-loader.ts` | `codingagent/resources.go` | [来源与优先级](../../learning/m5-local-resources.md) |
| `core/system-prompt.ts` | `codingagent/system_prompt_resources.go` | [system prompt](../../learning/m5-system-prompts.md) |
| `core/prompt-templates.ts` | `codingagent/prompt_templates.go` | [模板](../../learning/m5-prompt-templates.md) |
| `core/skills.ts` | `codingagent/skills.go` | [Skill](../../learning/m5-skills.md) |
| `modes/interactive/theme/theme.ts` | `codingagent/themes.go`、`codingagent/export_html.go` | [主题](../../learning/m5-themes.md) |
| `core/agent-session.ts` reload | `codingagent/session_reload.go` | [重载阶段语义](../../learning/m5-session-reload.md) |
| `core/package-manager.ts` 本地入口发现 | `codingagent/local_extensions.go` | [扩展发现](../../learning/m5-local-extensions.md) |
| CLI startup / session factory | `codingagent/headless.go`、`CreateAgentSession` | [组合示例](../../../examples/m5-workflow/main.go)、[冻结验收](../../learning/m5-freeze.md) |

Pi 路径相对 `packages/coding-agent/src/`。固定资源 Oracle 在 `parity/oracle/`，普通测试重放 `fixtures/`；集成门禁位于 `internal/m5gate/`。CLI 通过重开恢复加载修改后的资源，持续会话重载使用 SDK；不新增 RPC wire。包管理和扩展执行仍按 ADR-0034 保持未实现，不能从资源 API 映射推导出整个包生态对等。
