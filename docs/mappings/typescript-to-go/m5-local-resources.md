# M5.6 本地资源：TypeScript → Go

| 固定 Pi 入口 | Pig 入口 | 观察点 |
| --- | --- | --- |
| `core/package-manager.ts`: resolveLocalEntries / addAutoDiscoveredResources / toResolvedPaths | `codingagent/prompt_templates.go`、`skills.go`、`theme_resources.go` | 本地来源顺序、禁用前去重、资源各异的遍历 |
| `core/resource-loader.ts`: mergePaths / updateSkillsFromPaths / updatePromptsFromPaths / updateThemesFromPaths | 各资源文件 + `codingagent/resources.go` | 显式路径合并、默认禁用、元数据及独立查询副本 |
| `core/skills.ts`: loadSkills | `codingagent/skills.go`: LoadSkills | 文件级真实路径去重、first-win |
| `core/diagnostics.ts`: ResourceCollision / ResourceDiagnostic | `codingagent/resources.go`、`headless.go` | 冲突双方路径；Pig 补充 scope:source 标签及 CLI 展示 |
| `AgentSession.promptTemplates`、技能清单、Theme.fg | 公开 AgentSession / FormatSkillsForPrompt / Theme.FG | 查询与实际模板展开、系统提示、主题效果一致 |

- Oracle：`parity/oracle/local-resources.mjs` → `fixtures/local-resources.json`，19 个受控场景。
- SDK：`codingagent/issue105_resources_test.go`；CLI：`cmd/pig/issue105_resources_test.go`。
- API snapshot：`codingagent/testdata/issue105_surface_golden.txt`。
- [可运行示例](../../../examples/local-resources/main.go) · [中文学习材料](../../learning/m5-local-resources.md)。
- Catalog：`contract:codingagent/local-resources`。`package-manager.ts` 仅作为本地规则的基线来源，未实现 Pi 包管理器或包来源优先级；包生态仍为 #99。
