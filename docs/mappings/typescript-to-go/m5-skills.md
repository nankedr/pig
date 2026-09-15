# M5 Skill：TypeScript → Go

| Pi 固定基线 | Pig 入口 | 观察点 |
| --- | --- | --- |
| `core/skills.ts`: loadSkillsFromDir / loadSkills | `codingagent/skills.go`: LoadSkillsFromDir / LoadSkills | 本地扫描、frontmatter 校验、真实路径去重、冲突 |
| `core/package-manager.ts`: 本地资源发现 | `DefaultResourceLoader.loadSkills` | 全局、可信项目、祖先 `.agents`、settings 筛选；包生态延期 |
| `core/resource-loader.ts`: getSkills / reload | `codingagent/resources.go`: GetSkills / Reload | 来源、诊断、独立快照、信任决定 |
| `core/system-prompt.ts` + formatSkillsForPrompt | `codingagent/headless.go` + `prompts.go` | read Tool 门控、隐藏自动调用技能、XML 路径说明 |
| `core/agent-session.ts`: prompt / steer / followUp | `codingagent/session.go` + `skills.go` | 显式 skill block、参数、普通生成与消息历史 |
| `modes/rpc/rpc-mode.ts`: get_commands | `codingagent/rpc_mode.go` | 技能命令与 SourceInfo |
| CLI `--skill` / `--no-skills` | `codingagent/misc.go` + `headless.go` | Headless 真实进程 |

- 对等用例：`parity/oracle/skills.mjs` → `fixtures/skills.json`。
- 公开 SDK：`codingagent/issue103_skills_test.go`、`issue103_trust_test.go`。
- CLI/RPC：`cmd/pig/issue103_skills_test.go`。
- API snapshot：`codingagent/testdata/issue103_surface_golden.txt`。
- 可运行示例：`examples/skills`；中文说明：[发现并使用本地 Skill](../../learning/m5-skills.md)。
- 精确范围：Catalog `contract:codingagent/skills`；宿主执行/trust 见 ADR-0010，包生态延期见 ADR-0034。
