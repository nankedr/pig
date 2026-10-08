# V1 Responses 工具源码映射

固定 Pi Code Baseline：`936aff00918de1187f085f123c2812d8f2d67745`。#132 细分服务证据，不把完整 Pi symbols 提升为已完成。

| Pi 源码 / 契约 | Pig 源码 / 公开边界 | 已验收范围 |
| --- | --- | --- |
| `openai-responses-shared.ts#convertResponsesTools` | `ai/openai_responses.go#responsesTools`，公开 SDK/Agent 实际请求 | function 定义、schema；DeepSeek 默认省略 strict；constrained sampling 保持 Stub |
| `openai-responses-shared.ts#processResponsesStream` | `executeOpenAIResponses`，公开 AssistantMessageEventStream / Agent listener | arguments delta/done/item.done、call_id/item_id 配对、多调用与 ToolUse、partial 错误/取消 |
| `openai-responses-shared.ts#convertResponsesMessages` / `transformMessages` | `responsesInput` / `ai.TransformMessages`，公开 SDK 和正式 v3 Session | 文本、工具调用/结果、缺失结果修复；DeepSeek 明文 reasoning item ID 与 content 重建；孤立/重复历史拒绝 |
| Agent tool dispatcher | `agent/tool_runtime.go` / `agent/tool_validation.go` | 不改执行管线，复用参数准备/校验、顺序/并行、listener/update barrier 和失败结果 |
| production v3 Session / retry | `codingagent/session_manager.go` / `session_retry.go` | 不改持久化格式或 retry 策略；公开 Session 验证重开后历史与已完成副作用不重复 |

固定 Oracle：`parity/oracle/responses-tools.mjs`、`parity/oracle/fixtures/responses-tools.json`、`internal/parity/responses_tools_test.go`。服务 conformance：`ai/responses_tools_test.go`、`agent/responses_test.go`、`codingagent/responses_tools_test.go`。真实工具/明文 reasoning smoke：`agent/responses_live_test.go`。示例：`examples/responses-tools`。这些证据的来源与观察范围各自独立；图片、完整厂商选项与 #133 产品入口继续保留边界。
