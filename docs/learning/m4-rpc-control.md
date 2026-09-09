# M4.13：通过 RPC 控制运行中的会话

运行 `go run ./examples/rpc-control`。示例编译真实 pig 子进程，使用本地 Provider，先执行 Bash，再在生成期间投递 follow-up，等后续回合结束并读取统计，无需真实凭证。

`RPCClient.Prompt` 返回表示会话已接纳消息，生成仍通过事件继续。收到接纳响应后，可以用 Steer 或 FollowUp 投递文本；先设置 SetSteeringMode/SetFollowUpMode 决定一次消费一条还是全部。队列变化通过 queue_update 观察，GetState 的 pendingMessageCount 是尚未消费的展示队列长度。idle 投递明确报错，这是既有 Session 的 Go 约定。

| 目的 | JSONL command | RPCClient |
| --- | --- | --- |
| 运行中投递 | steer / follow_up，message | Steer / FollowUp |
| 消费模式 | set_steering_mode / set_follow_up_mode，mode | SetSteeringMode / SetFollowUpMode |
| 模型 | set_model，provider + modelId；cycle_model；get_available_models | SetModel / CycleModel / GetAvailableModels |
| 推理强度 | set_thinking_level，level；cycle_thinking_level；get_available_thinking_levels | SetThinkingLevel / CycleThinkingLevel / GetAvailableThinkingLevels |
| 重试 | set_auto_retry，enabled；abort_retry | SetAutoRetry / AbortRetry |
| Bash | bash，command + 可选 excludeFromContext；abort_bash | Bash / AbortBash |
| 统计/回复 | get_session_stats / get_last_assistant_text | GetSessionStats / GetLastAssistantText |

模型/thinking 在会话生成期间会返回 busy；先 WaitForIdle，再修改，下一次 Prompt 使用新配置。RPC 与 SDK 共用配置持久化事务。模型切换不会丢失进程设置的 Provider 地址。Bash 可在生成期间执行，其结果先暂存，在运行结束后入历史，随后 Prompt 可以看到结果。

每个请求用独立 id 关联响应，事件可以插在任意两个响应之间。短同步控制和查询按输入行顺序执行；输出单独排队，不阻塞输入的 EOF 处理。Bash update 携带原始请求 id；原始 wire 允许任意 JSON 值，公开客户端用递增字符串。重试失败消息写入 Session，AbortRetry 取消等待并产生 auto_retry_end，最终 agent_settled 才表示整轮结束。普通 Abort 和 AbortBash 的作用不同；取消 Go 请求 Context 不自动取消远端操作。

统计的 wire 字段是 `tokens.total`，Go 对应 `Tokens.TotalTokens`；cycle_model 返回 null 时，Go ModelCycleResult.Model.ID 为空，cycle_thinking_level 返回 null 时 Go level 为空。客户端 Bash 暂不提供 excludeFromContext 参数，原始 wire 可以使用该字段。图片、资源/扩展/UI、会话替换、树、导出与压缩 RPC 控制保留明确 Stub；固定 Pi 没有 Tool 配置 wire。

验证：`go test -race ./cmd/pig -run '^TestRPC95' -count=1`。目录条目为 `contract:rpc/session-control`；固定源码 Oracle 可通过 `node --experimental-strip-types parity/oracle/rpc-control.mjs <locked-pi-checkout> --check` 重跑。此次源代码 Oracle 验证通过，dist 重建受固定基线既有类型错误影响。详细边界见 [ADR-0031](../adr/0031-rpc-session-control.md)。
