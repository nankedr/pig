# Responses Coding Agent 源码映射

基线为 Pi commit 936aff00918de1187f085f123c2812d8f2d67745。V1/#133 的服务协议与本地 API 恢复偏离见 ADR-0044。

| Pi 兼容面 | Pig 实现 | 公开验证 |
| --- | --- | --- |
| core/sdk.ts、model-registry.ts | codingagent/sdk.go、headless.go、headless_settings.go、model_runtime.go | codingagent/responses_runtime_test.go；显式/设置/恢复 API |
| core/session-manager.ts | codingagent/session_manager.go、session_tree.go | 正式 v3 writer/reader、CLI 跨进程恢复与 fork |
| core/agent-session.ts 模型配置 | codingagent/session_configuration.go、session_configuration_persistence.go | TUI 模型选择、RPC set_model 状态与实际 Responses 请求 |
| core/compaction/compaction.ts、branch-summarization.ts | codingagent/session_compaction.go、session_branch_summary.go、session_tree_navigation.go | SDK 分支摘要和压缩；RPC compact 后续接 |
| modes/interactive/interactive-mode.ts | codingagent/interactive.go、interactive_models.go | cmd/pig/responses_terminal_test.go；真实 PTY 编码任务和终端恢复 |
| modes/rpc/rpc-mode.ts、rpc-client.ts | codingagent/rpc_mode.go、rpc_client*.go | cmd/pig/responses_process_test.go、responses_control_test.go；真实 JSONL 子进程 |

函数工具及 usage 的共同协议仍由 responses-text.json、responses-tools.json 固定 Pi fixture 验证。CLI/RPC/终端使用独立服务 fixture，不声称 Pi 运行过当前 DeepSeek 明文 reasoning 或 Pig 可选 model_change.api 字段。公共 API snapshot 保留原方法签名，并增加 SessionEntry.API、SessionModel.API 和 Settings.DefaultAPI。
