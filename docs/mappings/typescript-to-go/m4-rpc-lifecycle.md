# RPC 生命周期的 TypeScript → Go 导航

固定基线：`936aff00918de1187f085f123c2812d8f2d67745`。

| Pi | Pig | 验证 |
| --- | --- | --- |
| modes/rpc/rpc-mode.ts 的 lifecycle/compaction 分派 | codingagent/rpc_mode.go | cmd/pig/issue96_rpc_test.go |
| modes/rpc/rpc-client.ts 的 newSession/switchSession/fork/clone | codingagent/rpc_client_lifecycle.go | 真实 RPCClient、独立进程重开 |
| getEntries/getTree 与 session-manager.ts 的 v3 条目 | marshalSessionEntry/decodeSessionEntry、私有 rpcTreeNode | 排他 since、树 leaf、Raw 和消息类型 |
| core/agent-session-runtime.ts | codingagent/session_runtime.go | 目标构建失败保护、取消旧工作、listener 重绑 |
| agent-session.ts 的 compact/setAutoCompactionEnabled | codingagent/session_compaction.go、session_auto_compaction.go | 手动取消、overflow 摘要与恢复 |
| rpc-mode.ts 扩展 command context 的 navigateTree | SDK AgentSession.NavigateTree 已实现；RPC 扩展入口未交付 | 基线无独立 wire，不新增命令 |
| test/rpc.test.ts 的持久化、树、压缩断言 | parity/oracle/rpc-lifecycle.mjs、rpc-lifecycle-child.mjs | 固定源码 runRpcMode 与公开 RpcClient fixture |

运行示例：`go run ./examples/rpc-lifecycle`。参见[学习材料](../../learning/m4-rpc-lifecycle.md)与[提交边界](../../adr/0032-rpc-session-lifecycle.md)。
