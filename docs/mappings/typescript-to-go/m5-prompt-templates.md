# M5.3 Prompt Template：TypeScript → Go

| 固定 Pi 入口 | Pig 入口 | 行为 |
| --- | --- | --- |
| `core/prompt-templates.ts: PromptTemplate` | `codingagent/prompts.go: PromptTemplate` | 名称、描述、参数提示、正文及来源 |
| `utils/frontmatter.ts: parseFrontmatter/stripFrontmatter` | `ParseFrontmatter/StripFrontmatter` | 分隔符与换行、YAML 解析和正文提取 |
| `core/package-manager.ts: resolveLocalEntries/addAutoDiscoveredResources` | `prompt_templates.go: loadPromptTemplates/templateFiles` | 只移植本地路径发现、settings/ignore 筛选和优先级，不调用包管理器 |
| `core/resource-loader.ts: updatePromptsFromPaths/dedupePrompts` | `loadPromptTemplates` | 同路径去重、重名保留首项和诊断 |
| `getPrompts/reload` | `resources.go: GetPrompts/Reload` | 项目信任后读取、快照所有权与取消 |
| `parseCommandArgs/substituteArgs/expandPromptTemplate` | `prompts.go` 同名私有 helper | 引号、位置参数、默认值、切片和一次替换 |
| `AgentSession.promptTemplates/prompt/steer/followUp/sendUserMessage` | `session.go` / `session_messages.go` | 查询、默认展开、排队展开和显式绕过 |
| `main.ts: promptTemplates/noPromptTemplates` | `misc.go → CreateHeadlessSession` | CLI 与公开 Headless 选项 |
| `modes/rpc/rpc-mode.ts: get_commands` | `rpc_mode.go: get_commands` | 使用既有 wire 命令返回模板来源信息 |

来源自动目录为 `source=auto`，settings 路径为 `source=local`；scope 区分 project/user/temporary。Pig 项目目录使用 `.pig`；基线 fixture 只在比较时做身份路径映射。`*bool` 区分未指定展开开关和 false，不改变 SDK 的既有字符串元数据字段。

证据：[Oracle](../../../parity/oracle/prompt-templates.mjs)、[SDK](../../../codingagent/issue102_templates_test.go)、[CLI/RPC](../../../cmd/pig/issue102_templates_test.go)、[信任和重载](../../../codingagent/issue102_trust_test.go)、[API snapshot](../../../codingagent/testdata/issue102_surface_golden.txt)、[示例](../../../examples/prompt-templates/main.go)、[中文材料](../../learning/m5-prompt-templates.md)。
