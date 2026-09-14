# M5.2 本地 system prompt 与项目信任

[Issue #101](https://github.com/nankedr/pig/issues/101) 将 `DefaultResourceLoader` 的替换/追加提示词接入 CLI、Headless 和公开 SDK 的实际模型请求。运行 `go run ./examples/system-prompts` 可离线观察最终指令及文件来源。

| 输入 | 替换提示词 | 追加提示词 |
| --- | --- | --- |
| CLI | `--system-prompt <文本或路径>` | 重复 `--append-system-prompt <文本或路径>` |
| SDK 加载器选项 | `SystemPrompt *string` | `AppendSystemPrompt []string` |
| 自动发现 | 可信 CWD 下 `.pig/SYSTEM.md`，否则 `AgentDir/SYSTEM.md` | 可信 CWD 下 `.pig/APPEND_SYSTEM.md`，否则 `AgentDir/APPEND_SYSTEM.md` |

项目与全局文件是覆盖关系，不累加；只在 CWD 的 `.pig` 查找，不向祖先搜索。默认 AgentDir 是 Pig 用户目录，也可显式指定。显式输入独立于项目信任，即使路径位于未信任项目，用户明确指定后仍会读取。未指定替换/追加输入时才自动发现。

固定 Pi 的输入规则有几个细节：

- 已存在的路径读取为文件内容，其他字符串原样当作提示词。因此缺失路径、`~/...`、`file:...` 不自动报错或展开；相对显式路径相对于进程工作目录，而非加载器 CWD。来源返回绝对路径。
- `SystemPrompt == nil` 允许发现；指向空字符串则屏蔽发现，并使用默认内建提示词。空文件也使用默认提示词，但保留文件来源。
- `AppendSystemPrompt == nil` 允许发现；非 nil 空列表屏蔽自动追加。显式列表替换自动发现结果，按顺序用两个换行连接；空字符串元素过滤，空文件内容保留。CLI 可用 `--append-system-prompt ''` 屏蔽自动追加。
- 读取失败返回 warning 并保留输入字符串和来源，不回退到低优先级文件。Pig 对非普通文件也如此处理，避免 FIFO/设备阻塞，见 [ADR-0010](../adr/0010-trust-and-host-security.md)。

没有替换内容时，内建提示词描述当前 Tool；存在替换内容时，它替换内建主体。两条路径都依次加入追加内容、Context File 和工作目录。`SetActiveToolsByName` 重建提示词时继续使用会话装配的资源快照。`--no-context-files` 只禁用 Context File；`--no-extensions`、`--no-skills`、`--no-prompt-templates`、`--no-themes` 不禁用 system prompt。

项目信任在首次读取前确定：CLI 显式批准/拒绝优先，随后采用已保存决定和全局 `defaultProjectTrust`。无法询问的 Headless 对 `ask` 关闭项目加载；项目 settings 不能自授信任。全局资源照常可用，Context File 仍可在未信任时进入模型上下文。使用 ModelRuntime 的默认 SDK 路径也遵循同一首次加载顺序。显式注入 StreamFunction/Provider 的 SDK 路径保留内存 settings，默认不信任项目；调用者可注入已确定信任的 `SettingsManager` 或已加载资源。注入的 settings 由调用者负责刷新，加载器不会重复刷新并清除 `ApplyOverrides` 临时覆盖。

`ResourceLoader.GetSystemPrompt()`、`GetAppendSystemPrompt()` 返回加载内容；对应的 `GetSystemPromptSource()`、`GetAppendSystemPromptSources()` 返回文件来源，文本值没有文件来源。`DefaultResourceLoader.GetSystemPromptDiagnostics()` 返回独立诊断快照，Session services/Headless 汇总它，CLI 写入 stderr。空文件、缺失路径作为文本都不产生 warning。

SDK 显式注入加载器时，由调用者先 `Reload(ctx)`，然后传入 `CreateAgentSessionOptions.ResourceLoader`；Session 消费其公开 getter，加载错误直接返回。`CreateAgentSessionServices` 接收 `ResourceLoaderOptions` 并共享实例。加载器构造时不读资源，取消 Reload 保留旧内容；成功 Reload 更新加载器，已有 Session 的实时资源重载编排留待后续 M5。

本地资源不依赖包管理器、不扫描 manifest、不安装或执行包。`SystemPromptOverride`、`AppendSystemPromptOverride`、`ResolveProjectTrust`、`LoadProjectTrustExtensions` 仍是明确 M7 Stub。它们不代表完整扩展参与的 trust 流程。包生态按 [#99](https://github.com/nankedr/pig/issues/99) 延期。

```sh
node --experimental-strip-types parity/oracle/system-prompts.mjs /path/to/locked-pi --check
go test ./codingagent ./cmd/pig -run '^(TestSystemPrompts|TestPigSystemPrompts|TestPigUntrustedProjectHasNoSensitiveReadsOrEffects)' -count=1
go run ./examples/system-prompts
```

fixture 由固定 Pi commit `936aff00918de1187f085f123c2812d8f2d67745` 提取。SDK 用真实 AgentSession 与 Faux Provider 校验模型输入、来源、连续生成和 Tool 切换；CLI 以真实子进程连接本地 HTTP Provider，额外用 FIFO 证明未信任项目资源未被读取。目录/权限失败用例要求 POSIX 非 root；Catalog 的 `contract:codingagent/system-prompts` 保留精确的已支持及延期范围。
