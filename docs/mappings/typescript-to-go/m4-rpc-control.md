# M4.13：RPC 会话控制导航

| 固定 Pi | Pig |
| --- | --- |
| modes/rpc/rpc-mode.ts 的 handleCommand | codingagent/rpc_mode.go 的 rpcCommand |
| modes/rpc/rpc-client.ts 的控制方法 | codingagent/rpc_client_control.go |
| AgentSession.steer / followUp / 队列模式 | codingagent/session_messages.go |
| AgentSession.setModel / cycleModel / thinking | codingagent/session_configuration.go |
| AgentSession.executeBash / abortBash | codingagent/session_bash.go |
| AgentSession 重试与统计 | codingagent/session_retry.go、session_stats.go |
| rpc.test.ts 的投递、Bash、配置用例 | cmd/pig/issue95_rpc_test.go |
| 固定源码可执行观察 | parity/oracle/rpc-control.mjs、rpc-control-child.mjs、fixtures/rpc-control.json |

Go 保留既有 RPCClient 方法签名，以 Context 控制请求等待。原始 Bash id 通过 RPC 输出回调保持任意 JSON 值；Go SDK 的 ID 仍为 *string。Session 的配置和队列 admission 不因 RPC 放宽。wire tokens.total、nullable cycle 结果与 Go 字段的对应见 [学习材料](../../learning/m4-rpc-control.md)和 [ADR-0031](../../adr/0031-rpc-session-control.md)。运行示例：`go run ./examples/rpc-control`。
