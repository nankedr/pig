# M5.7：重载资源后继续当前会话

运行 `go run ./examples/session-reload`。示例用公开 SDK 创建真实会话，通过离线 Faux Provider 生成一次回复，修改 system prompt 和模板，再调用 `session.Reload(ctx)` 并继续生成。第二次请求沿用历史，使用修改后的资源。

## 调用顺序

使用 `CreateAgentSession` 或 `CreateAgentSessionFromServices` 装配会话；仅通过 scaffolded `NewAgentSession` 直接构造、未装配 system prompt 的实例会明确拒绝 Reload。

`AgentSession.Reload(ctx)` 刷新 SettingsManager、同步 steering/follow-up 队列模式，调用 ResourceLoader.Reload，最后重新组装 system prompt。Context File、SYSTEM.md、APPEND_SYSTEM.md、模板、Skill 和 Theme 的新增、修改、删除都会在成功重载后生效。

- 查询模板：`session.PromptTemplates()`；查询技能和主题：`session.ResourceLoader().GetSkills()` / `GetThemes()`。
- 模板和 `/skill:name` 用新内容展开；删除后同名命令按未知命令保留原文，不再使用缓存的旧模板或 Skill。
- 模型可见的 system prompt 使用当前活动 Tool 和新资源。之后调整 Tool 也继续使用这份资源。
- Theme 查询返回新主题；持有旧 Theme 对象的调用者需要再次 `SelectTheme`。此切片不运行交互式 TUI，也不自动替换调用者持有的对象。

会话对象、历史消息、Session 文件/ID、树和分支叶节点、模型、thinking level、可用/活动 Tool 保留。重载本身不追加 Session entry；继续生成仍向原 Session 文件追加消息。

## 设置与信任

固定 Pi 基线 `936aff00918de1187f085f123c2812d8f2d67745` 重载时保持 `SettingsManager.projectTrusted`。Pig 同样重新加载此信任状态允许的设置和资源，禁止在信任前读取项目设置。改变 `defaultProjectTrust` 不会自动改变当前会话的信任结论；调用者可显式 `SetProjectTrusted` 后重载。Context File 仍是无需项目信任的例外，显式路径仍独立授权。

会话重载会清除文件 SettingsManager 的临时 `ApplyOverrides`，恢复磁盘配置；内存 SettingsManager 按自身存储语义重载。直接调用注入的 DefaultResourceLoader.Reload 仍消费已准备的设置，不擅自刷新，以保留 [ADR-0010](../adr/0010-trust-and-host-security.md) 的装配契约。自定义 ResourceLoader 负责自己的加载事务与私有设置刷新。

## 失败、取消与并发

Pi 的重载不是跨设置和资源的回滚事务。公开 SDK 注入失败的 ResourceLoader 后，设置和队列模式可能已更新，而资源和 system prompt 仍保留旧值。Pig 保持这一步骤语义，返回错误，绝不把失败当作成功。若自定义加载器已经发布资源，随后查询或取消失败，查询可能看到它发布的资源，session system prompt 仍为上次成功构建值；调用者修复错误后再次重载。不要在失败后假定所有阶段已经同步完成。

取消发生在入口时不改状态；加载过程收到取消会返回错误；不遵守 context 的自定义加载器返回后仍检查取消，不提交新的 system prompt。加载器必须响应 context，才能及时结束阻塞操作。

正在生成的请求不会因重载被取消或丢失。已经发送给 Provider 的输入不变；成功重载后下一次生成使用新 prompt。查询在加载器发布前看旧资源，发布后看新资源，返回值为独立快照。多个查询调用之间可能跨越一次发布，不代表一个联合事务。

Go 并发入口按 [ADR-0035](../adr/0035-session-resource-reload.md) 做明确限制：重载进行中拒绝另一次重载、新 Prompt/Steer/FollowUp、配置变更及会话替换；正在进行的生成继续运行。展开命令与接纳消息之间如果发生重载，返回错误，调用者可重试，防止旧命令搭配新 prompt。

## 验收与未实现边界

```sh
node --experimental-strip-types parity/oracle/session-reload.mjs /path/to/locked-pi --check
go test -race ./codingagent -run '^TestSessionReload' -count=1
go run ./examples/session-reload
```

Oracle 通过固定 Pi 公开会话提取新增/修改/删除、失败后的设置状态和生成中重载结果；Go 测试通过最高层公开 SDK 实际生成、核对输入和回复，并验证 Session 文件重新打开后的分支历史。POSIX 子进程用项目 settings FIFO 检测 pre-trust 意外读取；取消、查询失败和多 goroutine 查询属于 Go SDK 验收。

Catalog `contract:codingagent/session-reload` 精确记录本地资源范围。扩展 reload、trust hooks、资源 override/ExtendResources、扩展 ABI 仍明确未实现；包 manifest/登记、npm/git、依赖和 lifecycle 由 #99 延期跟踪。没有增加 JSONL RPC reload 命令；六平台运行对等仍待 M13。
