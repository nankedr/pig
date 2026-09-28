<!-- GENERATED FILE - DO NOT EDIT BY HAND -->
# Responses 能力矩阵（生成视图）

来源：`parity/responses-matrix.json` + `parity/catalog.jsonl`；官方资料查阅日期：2026-09-29。

Pig 状态来自 Catalog，不从 DeepSeek 支持状态推导。`inventoried` 为未实现行为，`scaffolded` 仅有契约/Stub；共享条目在实现前必须细分，不能将一个入口的证据推广到所有行。

| 能力 | 标准线协议 | DeepSeek 支持状态 / 行为 | Pig 状态 / Catalog ID | 归属 | 验证要求 |
| --- | --- | --- | --- | --- | --- |
| text | POST /responses；model、instructions、input；message 内容及角色 | supported：文本支持；developer 按 user 处理，是服务差异。 | inventoried / `symbol:ai/src/api/openai-responses-shared.ts#convertResponsesMessages` | V1 / #131 | 请求/角色转换本地 fixture；Pi 共同语义 Oracle；文本 live smoke |
| stream | 语义 SSE，output item/content/delta 和终态 | supported：使用 completed/incomplete/failed 结束，无 [DONE]；sequence_number 递增。 | inventoried / `symbol:ai/src/api/openai-responses-shared.ts#processResponsesStream` | V1 / #131 | 分片/断流/失败/取消、usage 与 partial outcome；不得将异常 EOF 判成功 |
| function-tools | function 工具、tool_choice、function_call、function_call_output 与 call_id | supported：支持函数工具与结果回传。 | inventoried / `symbol:ai/src/api/openai-responses-shared.ts#convertResponsesTools` | V1 / #132 | 参数逐字节前缀、多个 tool call、call_id 关联和真实 continuation |
| reasoning | reasoning options 与输出 item、历史重放 | partial：effort 与明文 reasoning 可用；summary 不产摘要，encrypted_content 不支持。 | inventoried / `symbol:ai/src/api/openai-responses-shared.ts#convertResponsesMessages` | V1 / #132 | thinking 事件、工具前后 reasoning、本地持久化恢复；服务差异与 Pi 共同语义分开 |
| history | 完整 input 重放或 previous_response_id/conversation 服务端状态 | partial：仅本地完整历史；previous_response_id/conversation 不提供续接，传入时服务忽略。 | inventoried / `symbol:ai/src/api/openai-responses-shared.ts#convertResponsesMessages` | V1 / #132 | 跨轮/跨进程恢复、分支/压缩；不能靠 response ID 冒充历史 |
| images | input_image 与工具结果中的图片内容 | supported：deepseek-flash 支持 user/developer 与工具 output 图片；system/assistant 图片报 400。 | inventoried / `symbol:ai/src/api/openai-responses-shared.ts#convertResponsesMessages` | V1 / #134 | 图片真实进入上下文、工具 continuation、恢复/展示；受保护视觉 smoke |
| parallel-limits | parallel_tool_calls、max_tool_calls | ignored：并行始终启用；这两个控制参数不起作用。 | inventoried / `symbol:ai/src/api/openai-responses.ts#stream` | V1 / #132 | 显式登记服务限制；不能把 false/limit 宣称有效；本地调度语义不变 |
| storage-background | store、background、metadata、include 等选项 | unsupported：不提供持久化/后台能力；服务可接受并忽略，store 始终 false。 | inventoried / `symbol:ai/src/api/openai-responses.ts#stream` | V2 / #129 | V1 必须明确能力边界；忽略不等于 Pig 实现，也不等于固定 Pi no-op |
| format-sampling | text.format、text.verbosity、temperature、top_p、max_output_tokens | partial：format 可用，verbosity 忽略；sampling 随 thinking 模式变化，详见官方参数表。 | inventoried / `symbol:ai/src/api/openai-responses.ts#stream` | V2 / #129 | 基础输出上限属 #131；其余高级参数逐字段取证，不从服务接受推导 Pig 已支持 |
| custom-tools | custom_tool_call/custom_tool_call_output | partial：仅 apply_patch custom 工具受支持，其他名称报 400。 | inventoried / `symbol:ai/src/api/openai-responses-shared.ts#convertResponsesTools` | V2 / #129 | 单独实现/验证 custom 分支；V1 函数工具通过不覆盖本行 |
| hosted-tools | web_search/file_search/code_interpreter/computer_use/mcp 等 | ignored：这些工具类型被忽略，不提供对应服务能力。 | inventoried / `symbol:ai/src/api/openai-responses-shared.ts#convertResponsesTools` | V2 / #129 | 不得回报工具成功；历史特殊 item 的服务转换单列 fixture |
| other-options | prompt、truncation、service_tier、cache 控制、context_management、stream_options 等 | unsupported：不支持的顶层参数可被静默忽略；上下文超限报 400，缓存由服务自动管理。 | inventoried / `symbol:ai/src/api/openai-responses.ts#stream` | V2 / #129 | 不隐式丢弃 Pig 尚未支持的非默认分支；服务差异通过独立 fixture 验证 |
