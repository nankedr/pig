# V1 Responses 工具与本地历史

#132 在 DeepSeek 的 Responses 路径上接通公开 Agent 的函数工具循环。先运行无凭证示例：

```sh
go run ./examples/responses-tools
```

模型输出 `function_call` 后，适配器累积 arguments delta，以 `call_id|item_id` 保存 ToolCall 标识。Agent 沿用已有 PrepareArguments、schema validation、执行模式和 listener/update barrier，执行本地工具并保存 ToolResult；下一次 `/responses` 请求将完整历史转换为 `function_call` 与 `function_call_output`，线协议上的 call_id 保持配对。多个工具可顺序或并行执行，结果历史仍按调用源顺序保存。

每次请求发送完整 `input` 和 `store:false`，不发送 `previous_response_id`、`conversation`、`background`。响应 ID 仅供观察。普通 SDK 的多轮和正式 v3 Session 的重开都会重放文本、调用、结果及适用 reasoning。应用侧不应把上一轮用户 Prompt 当作重试入口重新发送；Session 的整轮 retry 保留已完成的调用与结果，错误 Assistant 留在持久化历史，进入模型上下文时移除。取消后的新 Prompt 同样保留已完成工具，丢弃取消的 Assistant 片段，不重复执行已有副作用。

DeepSeek reasoning 使用 `reasoning_text` 明文。ThinkingContent 保存文本及 reasoning item ID，恢复后用明文 `content` 重建 input item。输出可能附带兼容 summary/encrypted_content，Pig 不用它们生成思考内容或重放加密上下文；显式 ReasoningSummary 和 redacted reasoning 输入仍返回 Capability Stub。不能把其他厂商的加密续接能力套到此服务。

历史中孤立或重复 ToolResult、重复/空 call_id 在 transport 前失败；缺失结果沿用 TransformMessages 的 `No result provided` 合成失败结果，不执行工具补偿。流中的 call_id/name/item_id 错配、重复调用、已结束后参数变化及 completed 中不完整的 JSON 参数以错误 Outcome 结束，保留 partial，不执行这些调用。正常 incomplete/max_output_tokens 保存 Length 与部分参数，由 Agent 返回截断失败结果后继续；跨轮复用已存在 call_id 在执行前失败。截断/取消后不会在 transport 内盲目重发；Session 只对可重试错误继续历史，Responses 已有 partial content 时保留现有禁止自动重试规则。

## 服务限制

官方资料查阅日期为 2026-10-08，来源为 [Responses 指南](https://api-docs.deepseek.com/guides/responses_api/)与[请求参考](https://api-docs.deepseek.com/api/create-response/)。DeepSeek 文本模型/API/endpoint 保持显式配置。

| 服务行为 | Pig 边界 |
| --- | --- |
| function tools 与 auto/none/required/指定函数可用 | 支持已有 typed ToolChoice；2026-10-08 实际服务在 thinking 模式拒绝指定函数，HTTP 400 被保留为错误 Outcome，不能假报成功 |
| parallel_tool_calls、max_tool_calls 被忽略，服务始终允许并行调用 | 不提供有效的服务端禁并行/调用数限制；本地执行模式是 Agent 自身的控制 |
| previous_response_id、conversation、store、background 等服务端状态参数不提供续接 | 本地完整历史为真源；只发送 store:false；Deferred lifecycle 仍为 Stub |
| reasoning.summary 不产摘要，reasoning input 不支持 summary/encrypted_content | 只回传明文 content；显式摘要选项为 Stub；输出兼容附带字段不成为支持证据 |
| custom apply_patch 有服务支持，其他 custom 不支持；hosted 工具被忽略 | Pig custom/hosted ToolChoice 和 constrained sampling 仍为 Stub，transport/hooks 前失败 |
| 图片与其他高级选项 | 由 #134/#129 继续；不因本票接通工具而提升状态 |

## 验证

普通门禁完全离线：

```sh
go test ./ai ./agent ./codingagent ./internal/parity -run 'TestResponses' -count=1
go test -race ./ai ./agent ./codingagent ./internal/parity -run 'TestResponses' -count=3 -shuffle=on
node --experimental-strip-types parity/oracle/responses-tools.mjs <locked-pi-checkout> --check
```

`responses-tools.json` 从固定 Pi commit `936aff00918de1187f085f123c2812d8f2d67745` 生成，比较函数定义/选择、参数 delta/done 补全、多个调用、截断部分结果和工具历史。DeepSeek 明文 reasoning、严格配对与异常参数检查来自独立 fake HTTP/SSE 服务用例；它们不冒充 Pi Oracle。

受保护 smoke 缺凭证会失败，默认测试会跳过：

```sh
PIG_REQUIRE_RESPONSES_TOOLS_LIVE=1 go test ./agent -run '^TestResponsesAgentDeepSeekLiveToolContinuation$' -count=1 -v
```

2026-10-08 使用 `https://api.deepseek.com`、`deepseek-v4-pro`、reasoning low/auto 工具选择，公开 Agent 完成一次本地工具执行、明文 reasoning 回传与模型继续回答。仅检查执行次数、回传结构及成功终态，不把模型自由回答作为精确 golden。`PIG_RESPONSES_SMOKE_MODEL` 可覆盖模型 ID；密钥使用现有 `DEEPSEEK_API_KEY`，不写入 fixture。

#133 继续负责真实 CLI/RPC/TUI 的可恢复 Coding Agent、跨进程/fork/压缩后续接；本票的 Session SDK 重开证据不代表这些入口已验收。公共 API 形状未变化，因此无需改动 API snapshot。
