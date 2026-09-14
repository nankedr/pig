# M5.2 system prompt：TypeScript → Go

| 固定 Pi 入口 | Pig 入口 | 行为 |
| --- | --- | --- |
| `core/resource-loader.ts: DefaultResourceLoader.reload` | `codingagent/resources.go: Reload` | 先确定 settings 信任，再发现本地提示词；发布文件/内容/诊断快照 |
| `discoverSystemPromptFile` / `discoverAppendSystemPromptFile` | `discoverPrompt` | 可信项目覆盖全局，各自只取一个文件 |
| `resolvePromptInput` | `resolveResourcePrompt` | 文件或字面值、空值、失败回退；显式相对路径使用进程 CWD |
| `getSystemPrompt` / `getAppendSystemPrompt` | `GetSystemPrompt` / `GetAppendSystemPrompt` | nil 与空文件、空列表区分；返回独立快照 |
| `getSystemPromptSource` / `getAppendSystemPromptSources` | 同名 Go getter | 文件绝对来源；字面值没有文件来源 |
| `main.ts: systemPrompt/appendSystemPrompt` | `misc.go → CreateHeadlessSession` | CLI 重复追加选项进入加载器 |
| `core/sdk.ts` / `agent-session-services.ts` | `sdk.go` / `session_services.go` | 默认、显式注入及 services 路径共享公开加载器契约 |
| `core/agent-session.ts: _buildSystemPrompt` | `headless.go: configureSessionPrompt` | 替换、追加、Context File、Tool 和工作目录组合 |
| `setActiveToolsByName` | `session_configuration.go: SetActiveToolsByName` | 更换 Tool 时保留资源快照 |

Go 的显式 `SystemPrompt` 使用 `*string` 区分未指定与空值，追加使用 nil/非 nil slice 区分自动发现和关闭发现。Go 路径名按 Pig 身份使用 `.pig`，不隐式读取 `.pi`；特殊文件采用 ADR-0010 的可诊断回退。

证据：[Pi Oracle](../../../parity/oracle/system-prompts.mjs)、[SDK Parity Case](../../../codingagent/issue101_prompt_test.go)、[信任与失败](../../../codingagent/issue101_trust_test.go)、[CLI 子进程](../../../cmd/pig/issue101_prompt_test.go)、[可运行示例](../../../examples/system-prompts/main.go)、[中文说明](../../learning/m5-system-prompts.md)。
