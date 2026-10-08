# V1 Responses 可恢复 Coding Agent

#133 将已有 Responses 文本、reasoning 和工具循环用于 production v3 Coding Agent。无凭证示例会启动本地假服务、读取文件、重开会话、压缩并继续生成：

```sh
go run ./examples/responses-session
```

真实 DeepSeek 使用 canonical auth.json 或 DEEPSEEK_API_KEY：

```sh
pig --provider deepseek --model deepseek-v4-pro --api openai-responses
pig --provider deepseek --model deepseek-v4-pro --api openai-responses -p "读取并修改当前项目"
pig --provider deepseek --model deepseek-v4-pro --api openai-responses --mode rpc
pig --session /absolute/session.jsonl -p "继续"
pig --fork /absolute/session.jsonl -p "从此分支继续"
```

也可在已受信任的设置中保存：

```json
{"defaultProvider":"deepseek","defaultModel":"deepseek-v4-pro","defaultAPI":"openai-responses"}
```

项目设置仍须 Project Trust；AGENTS.md 等 Context File 沿用既有例外。模型列表只把已交付且有可用认证的 Provider 作为候选；显式选择其他 Provider 会返回结构化 Stub。完整 models.json 自定义目录/远端刷新仍由 #129 负责，不能通过目录中的厂商名称推断可运行能力。

SDK 用 CreateHeadlessSessionOptions.API 选择协议；生产 CreateAgentSession 也接受显式 Responses Model，或从 Session/settings 恢复。会话保留 function_call 的 call_id/item_id、参数、结果以及 DeepSeek 明文 reasoning 的 item ID 和文本。v3 model_change.api 与 Assistant.api 让跨进程恢复保留协议；缺少新字段的旧文件保持原默认。模型选择与摘要使用实际生效模型，不修改共享 Snapshot。

每次请求重放本地完整 input 和 store:false。fork 不修改来源文件；树导航选择历史分支，摘要和压缩创建正式 v3 条目，之后生成使用摘要及保留历史。RPC 沿用已有 fork/switch_session/compact，树导航与分支摘要使用公开 SDK/TUI；没有新增 navigate_tree 命令。

## 离线验证

```sh
go test ./codingagent ./cmd/pig -run TestResponses -count=1
PIG_TEST_RACE=1 go test -race ./codingagent ./cmd/pig -run TestResponses -count=3 -shuffle=on
go test ./internal/parity -run 'Responses|Session|Compaction|BranchSummary|RPC' -count=1
go test ./internal/catalog -count=1
```

同一“将 sentinel.txt 的 before 改为 after”任务经真实 CLI、RPC 子进程和 PTY 终端运行，检查磁盘结果、正式 reader 恢复的 reasoning/调用/结果、usage/stats、跨进程续接与 fork 来源隔离。公开 SDK 检查树导航、分支摘要、压缩后生成；RPC 检查 steer/follow-up、并发查询、busy 模型切换、abort 的 partial 与流中会话替换。子进程在 PIG_TEST_RACE=1 时也启用 race。

parity/services/responses-codingagent.json 是独立服务协议 fixture，不能冒充 Pi 对等证据。既有固定 Pi 文本/工具、Session、配置、导航、压缩及 RPC Oracle 继续约束共同语义。受保护 live smoke 使用已有文本和工具续接门禁；未执行的 live 分支不记为已验证，本票不宣称 V1 freeze/release。

本票另提供 `make responses-session-live-smoke`，用受保护 DEEPSEEK_API_KEY 验证真实 Coding Agent 读取一次临时文件、重开正式 v3 历史并继续生成，不比较模型自由回答的精确内容。默认测试跳过，显式要求 live 而缺少密钥时失败。

2026-10-08，经授权使用 https://api.deepseek.com、deepseek-v4-pro、reasoning low 的该门禁通过：一次 read 工具执行，正式 v3 reader 重开恢复 Responses API 与历史，随后回答成功且未重复工具。该记录是当前服务证据，不替代固定 Pi Oracle，也不构成 V1 freeze/release。

范围外的图片、其他厂商、高级 Responses 选项、Harness v4、Remote Session Protocol、扩展运行时及包生态保持原边界。源码映射见 [v1-responses-codingagent](../mappings/typescript-to-go/v1-responses-codingagent.md)。
