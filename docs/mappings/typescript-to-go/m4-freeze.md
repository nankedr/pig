# M4 TypeScript → Go 集成导航

固定 Pi Code Baseline：`936aff00918de1187f085f123c2812d8f2d67745`。先运行 `go run ./examples/m4-workflow`，观察一次完整编排如何跨越 Session、模型输入和 v3 持久化边界。

| Pi 源码 | Pig 入口 | 学习材料 |
| --- | --- | --- |
| `core/tools/{grep,find,ls}.ts` | `codingagent` 七工具定义和执行；真实子进程 `TestPigM4SevenToolsResumeAndFork` | [grep](../../learning/m4-grep-tool.md)、[find/ls](../../learning/m4-find-ls.md) |
| `core/agent-session.ts` 的消息投递与 retry | `AgentSession.SendUserMessage/Steer/FollowUp`、Session retry 生命周期 | [消息](../../learning/m4-session-messages.md)、[retry](../../learning/m4-turn-retry.md) |
| 同文件的模型、thinking、Tool 和 stats | `SetModel/SetThinkingLevel/SetActiveToolsByName/GetSessionStats` | [配置](../../learning/m4-session-configuration.md)、[stats](../../learning/m4-session-stats.md) |
| `core/compaction/*.ts` + `agent-session.ts` | `Compact/NavigateTree/GenerateBranchSummary`、自动压缩协调 | [手动压缩](../../learning/m4-manual-compaction.md)、[自动压缩](../../learning/m4-auto-compaction.md)、[树导航](../../learning/m4-session-tree-navigation.md)、[摘要](../../learning/m4-branch-summary.md) |
| `core/bash-executor.ts` + Session Bash | `ExecuteBash/AbortBash/RecordBashResult` | [Bash](../../learning/m4-session-bash.md) |
| `modes/rpc/{rpc-mode,rpc-client,rpc-types}.ts` | `RunRPCMode`、`RPCClient`、真实 `pig --mode rpc` | [framing](../../learning/m4-jsonl-rpc.md)、[控制](../../learning/m4-rpc-control.md)、[替换和压缩](../../learning/m4-rpc-lifecycle.md) |
| `core/export-html/` | `ExportFromFile`、`AgentSession.ExportToHTML`、RPC export | [HTML](../../learning/m4-html-export.md) |

Oracle 位于 `parity/oracle/`，相应 fixtures 位于其 `fixtures/` 目录；`make m4-oracle` 实际复核固定源码，HTML Oracle 还运行浏览器。`internal/m4gate` 固定 Catalog 的逐项状态、Go 映射和 partial 边界；每个切片的 `issue*_surface_test.go` 校验公开 Go API snapshot。

`RPCClient` 是 JSONL 子进程客户端，与 M9 的 `protocol/client` CBOR 远程会话协议分开。Pi 的扩展 command context 可以调用摘要树导航，但 Pig 此扩展入口归 M7/#9；M4 只交付公开 SDK 摘要导航和固定 Pi 已有的直接 RPC 命令。完整验收、范围表和发布复现入口见 [M4 冻结](../../learning/m4-freeze.md)。
