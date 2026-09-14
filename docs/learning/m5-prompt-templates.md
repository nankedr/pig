# M5.3 本地 Prompt Template

[Issue #102](https://github.com/nankedr/pig/issues/102) 将 Markdown 模板接入正常会话。运行 `go run ./examples/prompt-templates`，可离线观察模板查询、展开后的模型输入和后续回复。

把下面的文件保存为 `~/.pig/agent/prompts/review.md`，或可信项目的 `.pig/prompts/review.md`：

```markdown
---
description: 审查代码
argument-hint: <file> [focus]
---
请审查 $1，重点关注 ${2:-正确性}。
```

```sh
pig -p '/review "src/my file.go" 安全性'
pig -p '/review main.go' --no-prompt-templates --prompt-template ./review.md
```

名称来自文件名，frontmatter 的 `name` 不改名。`description` 和 `argument-hint` 使用字符串；没有描述时取正文首个非空行，按固定 Pi 的 60 个 UTF-16 单元截断并追加 `...`。无 frontmatter 的正文保留空白；有完整分隔符时，移除 frontmatter 并修剪正文两端空白。CRLF/CR 归一化为 LF。无效 YAML、重复键及读取失败按基线跳过；未找到附加路径产生 error 诊断，缺失默认目录不报错。YAML 日期是字符串，`<<` 是普通键，不自动合并。

加载顺序决定重名时的胜者：

1. 可信项目 settings 的 `prompts` 路径。
2. 可信项目 `.pig/prompts/`。
3. 全局 settings 的 `prompts` 路径。
4. 全局 `AgentDir/prompts/`。
5. CLI `--prompt-template` 或加载器 `AdditionalPromptTemplatePaths`。

重名保留先加载者，并返回 winner/loser 路径。settings 相对路径分别以项目 `.pig`、全局 AgentDir 为基准；附加相对路径以 CWD 为基准，支持 `~` 和 `file://`。默认目录和附加目录只扫描当前层 `.md`；settings 中的目录递归扫描。自动/settings 发现跳过隐藏项和 node_modules，并读取 `.gitignore`、`.ignore`、`.fdignore`；显式附加目录允许隐藏 `.md`，不应用 ignore 文件。settings 支持通配筛选、`!pattern` 排除、`+path` 精确恢复、`-path` 精确排除，后者优先。普通通配模式筛选已收集路径，不凭空发现额外目录。完整 minimatch extglob 语法不在当前证据范围。

`--no-prompt-templates` / `-np` 关闭默认目录和 settings 发现，显式附加路径仍然加载。项目 settings 和自动模板读取前必须先完成项目信任；CLI `--approve`、保存的决定、全局 `defaultProjectTrust` 沿用现有规则，Headless 的 ask 关闭项目读取。显式指定文件独立于项目信任，Context File 的例外也不改变。见 [ADR-0010](../adr/0010-trust-and-host-security.md)。

模板调用支持 `$1`、`$2`、`$@`、`$ARGUMENTS`，`${N:-默认值}`、`${@:-默认值}`，以及 `${@:N}`、`${@:N:L}`。参数可用单双引号分组；空引号不产生参数，未闭合引号取剩余文本，反斜杠不实现 shell 转义。替换只进行一次，参数/默认值中的 `$1` 不再展开；未知命令原样进入模型。切片从 1 起算，0 按 1 处理。模板内容不会作为 shell 命令执行。

SDK 调用 `ResourceLoader.GetPrompts()` 查询内容、来源和诊断，或 `AgentSession.PromptTemplates()` 查询当前模板。`DefaultResourceLoader.Reload(ctx)` 成功后更新独立快照；取消不替换旧快照。默认 SDK 路径使用已确定信任的 settings；显式注入 StreamFunction 时仍使用原有内存 settings 规则，调用者可注入 settings 或已加载的 ResourceLoader。`CreateAgentSessionServices` 的 `ResourceLoaderOptions` 支持同一组输入。

`AgentSession.Prompt` 默认展开，`PromptOptions.ExpandPromptTemplates` 指向 false 时原样发送。Steer/FollowUp 也展开，`SendUserMessage` 跳过模板处理。会话保留展开后的用户消息，并继续走正常生成和持久化路径。JSONL RPC 已有的 `get_commands` 返回 `name`、`description`、`source: "prompt"`、`sourceInfo`，不新增命令、不依赖扩展注册。

本票不安装包、不执行扩展。包生态按 [#99](https://github.com/nankedr/pig/issues/99) 延期；opaque override、扩展命令和 `ExtendResources` 仍为显式 Stub；Session 的完整实时 reload 编排仍待后续切片。Catalog 的 `contract:codingagent/prompt-templates` 使用 partial，区分本地证据、通用 YAML/extglob 未验证范围和延期能力。

```sh
node --experimental-strip-types parity/oracle/prompt-templates.mjs /path/to/locked-pi --check
go test ./codingagent ./cmd/pig -run 'Test(PromptTemplates|PigPromptTemplates|RPC102|PigUntrustedProjectHasNoSensitiveReadsOrEffects)' -count=1
go run ./examples/prompt-templates
```

fixture 来自固定 Pi commit `936aff00918de1187f085f123c2812d8f2d67745`，SDK 观察真实 AgentSession 的模型输入与回复；CLI 使用真实 pig 子进程连接本地 HTTP Provider；FIFO 回归证明首次启动未读取未信任项目设置和资源。没有用私有参数替换 helper 的单元测试代替对等验收。
