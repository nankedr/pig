# M5.6：预测本地资源的来源与冲突

运行 `go run ./examples/local-resources`，查看同一会话的模板、Skill、Theme、来源及冲突。三个查询入口分别是 `GetPrompts()`、`GetSkills()`、`GetThemes()`；会话的 `PromptTemplates()`、模型可见技能清单和 `SelectTheme()` 使用相同获胜资源。

## 先来源、再路径、最后名称

固定 Pi 基线 `936aff00918de1187f085f123c2812d8f2d67745` 的本地资源顺序如下，越靠前越优先：

1. 可信项目 `.pig/settings.json` 登记的资源。
2. 可信项目自动发现的资源。
3. 全局 AgentDir 的 `settings.json` 登记的资源。
4. 全局自动发现的资源。
5. `Additional*Paths` / CLI 显式路径，按传入顺序追加。

同一层保留目录遍历顺序，不再按资源名称排序。解析路径及 symlink 后先去重，再按资源名称 first-win。同名不等于同文件：不同文件同名会产生冲突；显式目录与已加载文件重叠时，模板和 Theme 可能报告同路径冲突，Skill 则在文件展开后再按真实路径去重。这是基线中不同资源类型的区别。

禁用条目仍占据自动/设置发现阶段的路径身份，避免同一文件换个 symlink 名字后重新启用。随后合并显式路径时只对启用路径去重，因此可以显式重新启用已禁用文件。`NoSkills`、`NoPromptTemplates`、`NoThemes` 禁用默认加载，显式路径仍有效；解析到的来源元数据仍可用于显式资源。

## 不同资源不能共用一套遍历规则

| 来源 | Prompt Template | Skill | Theme |
| --- | --- | --- | --- |
| AgentDir、项目 `.pig` 自动目录 | 直接子级 `.md` | 根目录 `.md`；递归找 `SKILL.md`，找到后不再深入该目录 | 直接子级 `.json` |
| settings 登记目录 | 递归 `.md` | Skill 目录规则 | 递归 `.json` |
| 显式目录 | 直接子级，包含隐藏/被忽略文件 | Skill 目录规则 | 直接子级，包含隐藏/被忽略文件 |
| 祖先目录、home 的 `.agents/skills` | 不发现 | 仅 `SKILL.md` | 不发现 |

Skill 的项目 `.agents` 从 CWD 向上遍历，到最近 Git 根目录为止；没有 Git 根则到文件系统根。home 的 `.agents` 单独按用户来源加载，不能因为 home 恰好是项目祖先而变成项目来源。自动和 settings 目录遵循隐藏文件及 `.gitignore` / `.ignore` / `.fdignore` 规则。settings 的相对路径基于 `.pig` 或 AgentDir；显式路径基于 CWD。

settings 数组支持普通路径、glob 筛选、`!pattern` 排除、`+path` 精确启用和 `-path` 精确禁用；最后的精确禁用胜过启用。Skill 的精确规则也可指向包含 `SKILL.md` 的目录。完整 minimatch extglob 尚未验收。

## 来源查询与信任

`ResourceDiagnostic.Collision` 含 `ResourceType`、`Name`、`WinnerPath`、`LoserPath`、`WinnerSource` 和 `LoserSource`。后两个可选字符串按 `scope:source` 表示，例如 `project:local`、`user:auto`、`temporary:local`。CLI 同时显示双方路径和来源；祖先目录可直接从路径区分。查询返回独立副本，修改来源指针不会污染后续查询；成功重载会更新结果。

拒绝项目后，加载器不读取项目设置及自动资源；合法的全局资源和显式资源照常生效。显式项目路径仍独立授权。全局目录通过 symlink 指向项目时，仍属于全局授权来源：Project Trust 不是文件路径沙箱。首次加载安全边界见 [ADR-0010](../adr/0010-trust-and-host-security.md)。

Pi 的固定诊断已给出获胜/被覆盖路径，但没有填充本地来源标签。Pig 按 #105 补充标签，保留原有获胜者、诊断顺序和消息主体；该展示增强不宣称为 Pi 字节对等。

## 可复核证据

```sh
node --experimental-strip-types parity/oracle/local-resources.mjs /path/to/locked-pi --check
go test ./codingagent ./cmd/pig -run 'Test(LocalResource|PigLocalResource|Issue105)' -count=1
go run ./examples/local-resources
```

19 个受控多目录用例覆盖混合优先级、类型各异的递归、home/祖先、默认禁用与显式组合、筛选、symlink、trust 和来源。公开 SDK 会话实际向 Faux Provider 发送展开后的模板，并核对技能清单及主题 ANSI；真实 CLI 子进程验收 RPC 命令查询及诊断输出。新增用例先复现了禁用别名复活、禁用默认加载后的来源丢失、Theme 设置目录未递归三个缺口，再验证修复。

Catalog `contract:codingagent/local-resources` 精确记录支持范围与证据。包来源优先级、manifest/登记、npm/git、依赖和 lifecycle 仍由 [#99](https://github.com/nankedr/pig/issues/99) 延期跟踪；本实现不隐式安装包、执行扩展或冻结扩展 ABI。会话级热重载编排、完整六平台运行对等仍待后续阶段。
