# V1 Responses 文本与终态

#131 在现有 API Adapter / Provider 分层中加入 DeepSeek Responses 文本切片。Chat Completions 保持默认，公开 SDK、Models 和 headless CLI 使用同一累积逻辑。

## 配置与示例

无需凭证的本地 HTTP/SSE 示例：

```sh
go run ./examples/responses-text
```

真实 headless 文本：

```sh
export DEEPSEEK_API_KEY=...
go run ./cmd/pig --provider deepseek --model deepseek-v4-pro \
  --api openai-responses --thinking off --no-tools -p '你好'
```

`--mode json` 以 Session 事件报告相同的内容、usage 与终态。沿用固定 Pi 的 JSON 退出约定，Provider 失败在事件中报告；文本模式失败以非零码退出。`PIG_DEEPSEEK_BASE_URL` 可指向本地 fake server。CLI key 参数优先于环境/文件凭证，header 删除与覆盖、Fetch、OnPayload、OnResponse 沿用已有契约。

SDK 通过 `ai.Model{Provider: ai.ProviderIDDeepSeek, API: ai.APIOpenAIResponses, ID: "...", BaseURL: "..."}` 明确选择模型和协议。使用 `ai.StreamOpenAIResponses`、`ai.StreamSimpleOpenAIResponses`、`ai.Complete`、`ai.CompleteSimple` 或 `ai.BuiltinModels()` 的对应方法；Complete helpers 收集同一 SSE 的最终 Outcome，并非另发非流式 HTTP 请求。模型名不参与通用协议逻辑。历史 `GetModels(deepseek)` 快照保持原样；手动把历史 Chat Completions 模型改成 Responses 时，应清除该快照的 `Compat`，避免沿用另一协议的 flags。Headless 的 `CreateHeadlessSessionOptions.API` 自动选择协议并清除历史 Completions flags。

## 请求、事件与失败

系统提示词进入 `instructions`，用户文本转换为 `input_text`，已有 assistant 文本转换为 `output_text`。输出按 output item/content part 累积，reasoning 为 ThinkingContent；done item 与终态内容回填不会重复追加已经输出的文本。`completed` 为 stop，`incomplete.max_output_tokens` 为 length，content_filter、failed、异常 EOF、格式错误为 error。Responses 不消费 `[DONE]` 作为成功终态。

usage 从 input/output/total tokens 与 cached/cache-write/reasoning 计数映射；input 扣除 cache 计数，reasoning 缺失与显式零保留区别。取消和超时产生 aborted Outcome，错误和取消仍可重复读取部分内容。请求前 HTTP/网络错误按既有 MaxRetries 与 Retry-After 规则重试；开始消费 SSE 后不重试。Session 对已产生内容的 Responses 错误也禁止自动重发，防止重复输出。

当前切片只开放 DeepSeek 文本。函数工具、工具结果和 reasoning 历史重放由 #132 接续，跨进程恢复/压缩等由 #133 接续，图片由 #134 接续。官方 OpenAI Provider、Azure、Codex、高级 sampling/cache/session 参数与尚未实现的非默认选项继续显式 Stub；默认 auto 传输使用 SSE。Simple 的 Deferred、ThinkingBudgets 和 cacheNone 下的 SessionID 按固定 Pi 忽略，已用非默认参数 Oracle 验证。工具默认开启的 CLI 需显式 `--no-tools` 才能使用本票文本切片。

## 证据与来源

官方资料复核于 2026-10-03：[Responses 指南](https://api-docs.deepseek.com/guides/responses_api/)、[协议参考](https://api-docs.deepseek.com/api/create-response/)。当前服务的 `deepseek-flash` / `deepseek-v4-pro` 与历史模型快照分开；本票 CLI 与受保护文本 smoke 选择目录中已存在、当前官方仍支持的 `deepseek-v4-pro`。视觉 smoke 配置继续由后续票完成。

- `ai/openai_responses_runtime_test.go`：公开 SDK/helpers/Models、fake HTTP/SSE、逐字节 Unicode 分片、多个 part、reasoning、终态、取消/超时、HTTP/Retry-After、hooks、Stub、并发重复 Outcome。
- `cmd/pig/issue131_process_test.go`：真实 CLI 子进程、JSON 事件与 usage、断流后保留内容且不重试。
- `parity/oracle/responses-text.mjs` / `internal/parity/responses_text_test.go`：固定 Code Baseline 的单文本 output item、终态、usage/cost、事件、Simple/cache no-op 与无系统提示词请求；只比较声明的投影，时间和 textSignature 不在此观察范围。失败 fixture 不携带 usage；携带 failed usage 时的累积由独立 fake 服务断言部分文本、计数和 cost，不宣称 Pi 对等。多 part / 明文 reasoning 同样单独取证。

```sh
go test ./ai -run '^TestResponsesSDK' -count=1
go test ./cmd/pig -run '^TestIssue131Responses' -count=1
go test ./internal/parity -run '^TestResponsesTextFixedPiOracle$' -count=1
node --experimental-strip-types parity/oracle/responses-text.mjs .upstream/pi --check
PIG_REQUIRE_RESPONSES_LIVE=1 go test ./cmd/pig -run '^TestIssue131DeepSeekResponsesLiveText$' -count=1
```

普通门禁离线。受保护 smoke 缺密钥必须失败；自由输出只检查成功与非空，不用作精确 golden。2026-10-03 在 darwin-arm64 上以受保护凭证完成上述 SDK 与真实 CLI 文本 smoke（deepseek-v4-pro，均成功且文本非空）。本票没有完成工具或视觉 smoke，也不代表 V1 freeze/release 已通过。
