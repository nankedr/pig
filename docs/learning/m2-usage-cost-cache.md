# M2.2 Usage、cost 与 cache 核算

M2.2 让调用者从公开 `AssistantMessage.Usage` 读取统一的 token 与成本结果。OpenAI Chat Completions 的 Provider 计数会映射为互斥的 input、output、cache read、cache write 和 reasoning；Faux 则提供离线、确定性的 session cache 行为。

## 核算规则

OpenAI 的 `prompt_tokens` 是 gross input。Pig 先取 `prompt_tokens_details.cached_tokens`，缺失时才回退到 `prompt_cache_hit_tokens`，并单独读取 `cache_write_tokens`：

```text
input = max(0, promptTokens - cacheRead - cacheWrite)
total = input + output + cacheRead + cacheWrite
```

`completion_tokens` 已包含 reasoning，不能再次累加；`completion_tokens_details.reasoning_tokens` 只作为 breakdown。Go 的 `Optional[int64]` 还保留了 reasoning 缺失和显式零的区别。固定 Pi 当前会把缺失折叠为零，因此这项增强由 Go 契约测试单独验证，不伪装成上游等价。

`CalculateCost` 以每百万 token 的费率计算四个分项。定价 tier 按 `input + cacheRead + cacheWrite` 选择严格超过阈值的最高一档，与声明顺序无关。`CacheWrite1H` 是 `CacheWrite` 的子集：普通写入使用 cache-write rate，一小时写入固定使用当前 tier input rate 的两倍。

## Faux session cache

Faux 将 system prompt、消息和 Tool 序列化为稳定文本，并在同一个 Provider 实例中按 `SessionID` 保存上一条 prompt：

- 首次请求把完整 prompt 记为 cache write；
- 同 session 重复请求把共同前缀记为 cache read，新增后缀记为 cache write；
- 不同 session 相互隔离；
- `CacheRetentionNone` 不读取也不更新 cache；
- token 估算按 JavaScript UTF-16 code unit 长度除以 4 向上取整。

流式取消只核算已发出的 output；预先取消不会产生 usage 或污染 cache。Provider 失败也不会伪造完整请求用量。done 事件、重复 `Result` 和 `Models.Complete` 返回同一份终态 usage/cost。

## 离线运行

公开 Go SDK 示例无需密钥或网络：

```sh
go run ./examples/usage-cost-cache
```

固定 Pi 和 Go 重放命令：

```sh
node --experimental-strip-types parity/oracle/usage-cost-cache.mjs .upstream/pi --check
go test ./internal/parity -run '^TestUsageCostCacheParity$' -count=1
go test ./ai -run '^(TestFaux.*Usage|TestOpenAIUsage|TestOpenAIFailure|TestOpenAICancellation)' -count=1
```

fixture 覆盖 chunk/choice usage 来源、详细/旧式 cache read、cache write、缺失/显式零/非零计数、input clamp、基础/阶梯/一小时 cache write 成本，以及 Faux 的 session cache 与 Stream/Complete 一致性。逐字段状态仍以 `parity/catalog.jsonl` 为准。
