# M5：发现并使用本地 Skill

Skill 用目录中的 `SKILL.md` 保存可复用指令。运行示例：

```sh
go run ./examples/skills
pig -p --skill ./review/SKILL.md '/skill:review 检查本次修改'
```

frontmatter 的 `description` 必须是非空字符串；`name` 缺省时取父目录名。名称限定小写字母、数字和单个连字符，最多 64 个 UTF-16 单元；描述最多 1024 个单元。名称和长度问题产生 warning，仍保留技能；空描述和解析失败跳过技能。`disable-model-invocation: true` 隐藏自动调用描述，显式 `/skill:name 参数` 仍可用。

## 发现与来源

默认加载器按以下顺序保留首个同名技能：

1. 可信项目 settings 的 `skills` 路径和 `.pig/skills`。
2. 当前目录至最近 Git 根目录的 `.agents/skills`；没有 Git 根目录时继续至文件系统根。受项目信任控制。
3. 全局 settings 的 `skills` 路径和 `AgentDir/skills`。
4. 用户 `~/.agents/skills`。
5. 显式 `--skill` 或 `AdditionalSkillPaths`。

settings 相对路径以项目 `.pig` 或 AgentDir 为基准；显式路径以 CWD 为基准，也支持 `~` 和 `file://`。settings 支持已有本地资源通配筛选、`!pattern` 排除、`+path` 恢复、`-path` 排除；`SKILL.md` 可按父目录匹配。每项保留 SourceInfo，冲突诊断包含 winner/loser 路径，真实路径相同的 symlink 重复项静默去重。自动发现先保留首个候选的禁用状态，再过滤，避免已禁用技能从别名重新出现；显式路径仍可独立加载。

发现遇到 `SKILL.md` 就以该目录为技能根，不再递归；否则 `.pig`、AgentDir 和显式目录加载根层 `.md`，递归子目录只寻找 `SKILL.md`。`.agents` 只寻找 `SKILL.md`。跳过隐藏项、node_modules 和断开的 symlink，遵循 `.gitignore`、`.ignore`、`.fdignore`。Pig 跳过特殊文件及循环 symlink，避免阻塞。

`--no-skills` / `-ns` 关闭 settings 与默认目录发现，显式路径仍加载。项目读取先经过 trust gate；显式路径独立授权。`LoadSkills` 和 `LoadSkillsFromDir` 是低层读取 API，显式请求 `includeDefaults=true` 会读取项目默认目录；需要自动信任策略的调用者应使用 DefaultResourceLoader / 会话创建路径。

## 会话行为

默认会话只在 read Tool 可用时向 system prompt 添加自动调用清单，禁用自动调用的技能不进入清单。`/skill:name` 将正文和 `References are relative to ...` 路径说明包装为 skill block，参数放在块后，再走正常生成、历史记录和队列流程。技能内容在调用时重新读取；未知命令或读取失败按基线原样发送。当前扩展错误回调仍是 M7 Stub。

`PromptOptions.ExpandPromptTemplates=false` 同时跳过技能和模板展开；Steer/FollowUp 会展开，SendUserMessage 不展开。JSONL RPC `get_commands` 返回 `skill:name`、描述、`source: skill` 和 SourceInfo，包含禁止自动调用的技能。Pi 的 `enableSkillCommands` 是交互 UI 的命令开关，不限制这里的 SDK/RPC 显式调用。

通过 `session.ResourceLoader().GetSkills()` 查询技能与诊断。`DefaultResourceLoader.Reload(ctx)` 成功后原子发布新快照，返回值可由调用者修改而不影响加载器。完整 Session 实时 reload 编排留待后续切片。

## 验证和边界

```sh
node --experimental-strip-types parity/oracle/skills.mjs /path/to/locked-pi --check
go test ./codingagent ./cmd/pig -run 'Test(Skills|PigSkills|RPC103|PigUntrustedProjectHasNoSensitiveReadsOrEffects)' -count=1
```

固定基线为 `936aff00918de1187f085f123c2812d8f2d67745`。fixture 记录 Pi 的发现、来源、诊断、模型清单及命令展开；公开 SDK 验证会话生成，CLI 测试运行真实子进程连接本机 Provider fixture，另有首次启动 FIFO trust gate、畸形元数据、循环链接、队列和重载测试。

Skill 正文可以要求运行辅助脚本，执行仍使用宿主的 bash 和外部工具；不保证第三方技能只依赖 Go。按 ADR-0034，本票不读取 package manifest、不隐式安装包、不执行扩展或冻结扩展 ABI。Catalog 使用 partial，明确保留 #99 包生态、M7 override/扩展错误回调、完整 minimatch extglob 与通用 YAML 错误文本对等、M13 六平台运行验证的边界。
