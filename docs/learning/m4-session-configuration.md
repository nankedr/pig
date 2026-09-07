# M4.5：在同一会话中切换模型、thinking 和工具

运行 `go run ./examples/session-configuration`。示例使用离线 ModelRuntime、临时认证和注入的 Faux stream，展示两次请求读取不同配置，不访问 Provider 网络。

```go
err := session.SetThinkingLevel("high")
err = session.SetActiveToolsByName([]string{"read", "write"})
err = session.SetModel(nextModel)
result, err := session.CycleModel(ctx, codingagent.ModelCycleBackward)
```

实际程序应逐次处理 error。修改在方法成功返回后生效，随后调用 Prompt 即使用新配置。不要在生成中、重试等待中或 thinking 事件回调中修改配置；这些调用返回 busy。AgentSession.State 返回完整快照，修改返回的 Model、ScopedModels 或 GetAllTools 不会改写内部配置。

模型轮换默认读取 ModelRuntime.GetAvailableSnapshot。SetScopedModels 限制顺序和每个模型可选的 thinking 偏好，不改变当前模型，也不持久化。轮换时过滤当前不可用模型；剩余零个或一个返回 nil。当当前模型不在候选中时，以索引 0 为起点再前进或后退。认证在真正切换时重新解析，因此目录快照过期不会绕过缺失认证检查。后续请求再次解析认证，能使用更新的密钥。

GetAvailableThinkingLevels 和 SupportsThinking 反映当前模型能力。合法等级是 off/minimal/low/medium/high/xhigh/max；模型不支持的合法等级按 Pi 算法向上、再向下裁剪。CycleThinkingLevel 在非 reasoning 模型上返回空字符串；扩展等级只在模型映射显式支持时参与轮换。非法等级直接失败。

SetModel 和有效 thinking 变化写入 v3 Session；重开使用 ModelRuntime 恢复这些条目。thinking 变化发出 thinking_level_changed，重复设置无事件。切到非 reasoning 模型时使用 off，但保留 settings 中的偏好以便切回来。模型默认值写入 settings。

工具允许/排除列表限定可重新启用的名称。默认四工具可反复启用或停用；空列表关闭所有工具。设置 tools=[read] 后不能通过动态切换启用 write；noTools=all 同样不能事后启用工具。noTools=builtin 仅关闭初始工具集。未知、被排除、未注册或重复的名称会让整次操作失败。工具说明、建议及请求 Tool schema 一起切换，创建时读取的项目上下文保持在 system prompt 中。工具启用集不随 Session 文件恢复。

验证命令：

```sh
node --experimental-strip-types parity/oracle/session-configuration.mjs /path/to/locked-pi --check
go test -race ./codingagent -run '^TestSessionConfiguration' -count=1
go run ./examples/session-configuration
```

证据登记为 contract:codingagent/session-configuration；快照是 codingagent/testdata/issue87_surface_golden.txt。Oracle 比较固定 Pi 的公开 createAgentSession 源码与构建产物，再由 Go SDK 重放。真实本地 HTTP 测试验证请求模型、动态认证、thinking、工具说明及重开恢复。

本切片保留既有 Adapter 范围。扩展 hook、运行中 next-turn 工具修改、RPC 配置命令、TUI 模型选择、动态 Provider 注册及 models.json 覆盖保持未实现。与 Pi 的失败和并发规则差异见 [ADR-0025](../adr/0025-session-configuration.md)，源码导航见 [TypeScript → Go](../mappings/typescript-to-go/m4-session-configuration.md)。
