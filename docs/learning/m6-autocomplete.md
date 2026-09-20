# M6.7 命令、本地资源与文件补全

对应 #115，固定 Pi commit `936aff00918de1187f085f123c2812d8f2d67745`。

```sh
go run ./examples/autocomplete    # 本地模板补全、展开与 Faux 对话，无网络和凭证
go run ./examples/interactive-text # 真实终端中输入 /、路径或 @文件
```

输入 `/` 可搜索产品命令、Prompt Template、`skill:名称`。上下键选择，Tab 接受；Enter 接受命令后立即提交，文件候选的 Enter 只接受，再按一次才提交。Escape 关闭候选并保留草稿；Ctrl+C 清空草稿。接受补全可以撤销。光标右侧的文字保留，带引号路径会去掉重复的闭引号；目录保留 `/`，引号目录的光标停在闭引号前。

普通路径按当前目录逐层前缀匹配；Tab 可主动补全普通文件名。支持相对路径、绝对路径、`~/`、空格和 Unicode。`@` 使用已安装的 `fd`/`fdfind`（优先 AgentDir 的 `bin/fd`），传入固定基线的隐藏文件、忽略 `.git`、目录作用域和结果数量选项。缺少或无法运行工具时没有 `@` 候选，不下载工具，普通路径补全仍可用。普通路径不套用 `fd` 的 ignore 规则。

**文件引用保持文本语义**：`@file` 或 `"space file"` 原样进入 Provider，不在交互提交时自动读取内容；需要时仍由模型调用已有 Tool。这与启动参数中的文件附件不是同一入口。

`codingagent.NewSessionAutocompleteProvider(session, fdPath)` 复用已加载的资源快照。补全不重新扫描项目资源，不改变既有项目、全局、显式资源优先级和冲突诊断。未受信项目资源不可见；`--no-prompt-templates`、`--no-skills` 和 settings 的路径过滤在加载阶段生效；显式 CLI 资源保留独立授权。`enableSkillCommands=false` 隐藏 Skill 菜单，但不撤销已经显式授权的 Skill 调用能力。`disable-model-invocation` 不隐藏用户可选择的 Skill。菜单显示 `[u]`、`[p]`、`[t]` 来源标签。

选择模板或 Skill 后，由原有 `AgentSession.Prompt` 执行参数替换、Skill 包装并进入 v3 Session 与 Provider。`/quit` 退出，`/new` 使用已有新会话能力；其他未交付产品命令返回结构化 Capability Stub，不作为普通消息发送到 Provider。扩展命令注册与执行仍由 M7 的显式 Stub 限制。

## SDK 与并发约定

- `CombinedAutocompleteProvider.GetSuggestions` 使用 `context.Context` 取消；Go 光标列为 UTF-8 字节偏移，非法列返回错误。
- `Editor` 在后台请求建议，组件状态仅在串行的 `HandleInput`、`Render`、`IsShowingAutocomplete` 调用中更新；宿主通过 `EditorRuntime.RequestRender` 收到刷新通知。与其他 Editor API 一样，调用方必须串行访问组件。
- 输入、光标、Provider 或草稿变化会取消请求；过期返回无法覆盖新状态，即使自定义 Provider 忽略取消也不会发布旧结果。
- `SetAutocompleteProvider(nil)` 停止补全并取消请求；`TextUI.Stop` 自动取消。自定义 Provider 必须响应 context 才能及时释放自身任务。
- `SetAutocompleteMaxVisible` 限制在 3–20。终端渲染转义不可信候选文本，主题样式仍由宿主控制。

## 验证与尚未覆盖的范围

```sh
go test ./tui ./codingagent ./cmd/pig -run 115 -count=1
go test -race ./tui ./codingagent -run 115 -count=1
node parity/oracle/autocomplete.mjs /path/to/locked-pi --check
node parity/oracle/autocomplete-editor.mjs /path/to/locked-pi --check
node parity/oracle/autocomplete-cli.mjs /path/to/locked-pi --check
```

三个锁定 fixture 分别覆盖 Provider、Editor（含 OnChange/OnSubmit 序列）和真实 CLI/PTY。命令菜单仅在首行触发，后续行的 `/文字` 保留草稿语义。CLI 检查 Provider 收到的展开文本、文件引用、退出码及终端恢复；harness 持续排空 PTY 并等待候选出现，避免终端背压或 fd 调度改变输入时序。SDK 另测信任、资源冲突与禁用、忽略取消的 Provider 和 fd 子进程取消。新增 CLI 用例已在实现前的 commit `a0af900` 上重放并确认失败。

Parity Catalog 条目 `contract:tui/autocomplete` 保持 **partial**：模型/登录参数菜单及对应产品命令、完整 locale/Unicode 排序、所有多层目录/符号链接/异常 fd 输出组合、Pi 的精确防抖与请求调度、候选菜单像素布局尚未全面对等。fd 使用受控可执行程序验证协议及结果；不声称已验证每一版真实 fd 的忽略规则。M7 扩展、M11 完整认证、M12 图片、M13 六平台验收及未排期包生态保持原边界。
