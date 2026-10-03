# V1 Responses 文本源码映射

固定 Code Baseline：`936aff00918de1187f085f123c2812d8f2d67745`。本票 #131 的范围由 `parity/responses-matrix.json` 与 `parity/delivery-scope.json` 细分，完整 Pi symbol 保持 partial。

| Pi 源码 / 契约 | Pig 源码 / 公开边界 | 本票范围 |
| --- | --- | --- |
| `ai/src/api/openai-responses.ts#stream` / `streamSimple` | `ai/openai_responses.go`；`OpenAIResponsesAPI`、compat helpers、Models | DeepSeek 文本、输出上限、认证/headers/hooks、取消/超时、HTTP 请求前重试 |
| `ai/src/api/openai-responses-shared.ts#convertResponsesMessages` | `responsesInput`，由公开 helpers 的实际 payload 验收 | instructions、用户文本、assistant 文本；服务语义单列，完整 Pi 系统角色/signature 转换不宣称完成 |
| `ai/src/api/openai-responses-shared.ts#processResponsesStream` | `executeOpenAIResponses`，公开 AssistantMessageEventStream / Stream Outcome | 单/多 output item/content part、文本和明文 reasoning、usage/cache、completed/incomplete/failed 与部分结果 |
| `ai/src/providers/deepseek.ts` / Provider API 分派 | `ai/model_catalog.go#newBuiltinProviderAPIs` | 增加 Responses API 所有权，历史 Chat Completions 目录不修改 |
| production v3 Session / print mode | `codingagent/headless.go`、`codingagent/misc.go`、`codingagent/session_retry.go`、真实 `cmd/pig` 子进程 | `--api` 与 `CreateHeadlessSessionOptions.API`；文本/JSON 观察同一 Outcome；已输出的 Responses 不自动重试 |

固定 Oracle 位于 `parity/oracle/responses-text.mjs` 与 `internal/parity/responses_text_test.go`；当前 DeepSeek 协议、多个 part、reasoning 和 HTTP/SSE 分片证据位于 `ai/openai_responses_runtime_test.go`，真实 CLI 证据位于 `cmd/pig/issue131_process_test.go`，受保护文本 smoke 位于 `cmd/pig/issue131_live_test.go`。两类证据独立记录。未实现分支继续按 #132/#133/#134/#129 跟踪。
