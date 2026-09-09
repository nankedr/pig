# M4.12：JSONL RPC 导航

固定 Pi：`936aff00918de1187f085f123c2812d8f2d67745`。

| Pi 文件/入口 | Pig 文件/入口 |
| --- | --- |
| `modes/rpc/rpc-mode.ts#runRpcMode` | `codingagent/rpc_mode.go#RunRPCMode` |
| `modes/rpc/jsonl.ts` | `rpc_mode.go#readRPCLines`、`modes.go#jsonLineWriter` |
| `modes/rpc/rpc-client.ts#RpcClient` | `codingagent/rpc_client.go#RPCClient` |
| `modes/json-event.ts#toJsonEvent` | `modes.go#projectJSONAgentSessionEvent`、`rpc_events.go#decodeRPCEvent` |
| `main.ts` RPC 分支 | `codingagent/misc.go#runHeadlessMain` |
| `rpc-jsonl.test.ts`、`rpc-prompt-response-semantics.test.ts`、`rpc-client-process-exit.test.ts` | `cmd/pig/issue94_rpc_test.go` 与 `parity/oracle/rpc.mjs` |

Go 的请求用 Context 等待，用 id 匹配 response；每个事件订阅使用 FIFO，解码后的消息复用 agent/Session 联合类型。`RPCSessionState` 新增明确 JSON tag；公共签名快照在 `codingagent/testdata/issue94_surface_golden.txt`。

运行 `go run ./examples/rpc-chat`；生命周期和公共偏离见 [学习材料](../../learning/m4-jsonl-rpc.md)与 [ADR-0029](../../adr/0029-jsonl-rpc.md)。未交付命令保留明确失败，不因 Session SDK 已有相近方法而自动宣称 RPC 对等。
