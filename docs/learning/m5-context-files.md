# M5.1 Context File 进入模型上下文

[Issue #100](https://github.com/nankedr/pig/issues/100) 补齐 `DefaultResourceLoader → CreateAgentSession → Provider` 的本地 Context File 链路。`CreateHeadlessSession`、CLI 和 Session services 使用这条链路；启动、恢复会话时重新加载，后续生成沿用装配时的文件快照。注入 Faux Provider 也会收到这些指令。

每层按照 `AGENTS.override.md`、`AGENTS.md`、`AGENTS.MD`、`CLAUDE.md`、`CLAUDE.MD` 选择第一个可读取的普通文件。空文件也是有效选择，会遮蔽后续候选。缺失、断链和目录候选被跳过；读取失败记录 warning，继续尝试下一候选。文件系统大小写规则决定大小写候选是否指向同一文件。

顺序是全局 AgentDir 的文件在前，然后从文件系统根目录到 CWD 的祖先文件；不会在 `.git` 或 HOME 停止。返回绝对但保持词法路径的来源，目录与文件 symlink 可跟随读取，同一词法路径只加载一次，不按 inode 合并。有效嵌套 linked worktree 的同名文件会遮蔽主工作树文件；缺少 HEAD、普通目录、bare 布局不冒充有效 linked worktree。路径支持相对路径、`~` 与本地 `file:` URL。

`Session.ResourceLoader().GetAgentsFiles()` 提供独立的来源/内容快照。默认加载器的 `GetContextFileDiagnostics()` 返回读取诊断；Headless runtime 与 Session services 将诊断暴露为 warning，CLI 写到 stderr。空文件和缺失文件不报警。

构造默认加载器不读取 Context File，调用 `Reload(ctx)` 才更新文件和诊断快照。SDK 自动构造并加载默认实例；显式 `ResourceLoader` 由调用者准备，SDK 只使用 `GetAgentsFiles()`，不会调用其 Reload 或不相关资源方法。`CreateAgentSessionOptions.NoContextFiles`、Headless 的同名选项和 CLI `--no-context-files` / `-nc` 禁止加载与注入；若调用者自己提前加载了注入实例，SDK 无法撤销那次读取。

项目信任复用 M3：[ADR-0010](../adr/0010-trust-and-host-security.md) 规定先判断信任再读取敏感项目设置。拒绝信任仍加载 Context File；它们进入模型指令，不代表 Tool 审批或文件系统 Sandbox。

本票只实现 Context File。Skill、Prompt Template、Theme、system/append prompt 资源加载及 ExtendResources 保持显式未实现；现有 CLI 显式 system prompt 仍可使用。Opaque override 和扩展运行时等待 M7 决策。按照 [#99](https://github.com/nankedr/pig/issues/99) 延期包生态，不构造默认包管理器、不扫描 manifest 后隐式安装，也不运行 lifecycle。`Reload` 更新加载器快照，已有 Session 的实时重载编排另行交付。

运行离线示例：

```sh
go run ./examples/context-files
```

验证固定 Pi 快照与最高层公开行为：

```sh
node --experimental-strip-types parity/oracle/context-files.mjs /path/to/locked-pi --check
go test ./codingagent ./cmd/pig -run '^(TestContextFiles|TestPigContextFiles)' -count=1
```

Fixture 覆盖候选、搜索边界、路径、读取诊断和 worktree；SDK 使用 Faux Provider 验证实际收到的上下文及连续生成，CLI 真实子进程验证首次生成、修改文件后恢复、拒绝信任和禁用。POSIX 读取权限用例要求非 root 用户；大小写差异只在单独的大写候选用例中规范化。Catalog 的 `partial` 准确区分已支持的 Context File 与未实现资源分支。
