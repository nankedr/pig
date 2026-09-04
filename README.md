# Pig

Pig 是 Pi 固定版本的 Go 语义兼容实现。M1 已通过冻结门禁并发布 v0.1.0：faux 与 DeepSeek/OpenAI Chat Completions 核心、EventStream、SSE、partial JSON、retry、Tool 管线、内存 AgentSession 与完整 read 的 text/json Headless 路径端到端可运行。当前 Milestone Frontier 是 M2。

- [文档导航](docs/README.md)
- [M1 Headless text 与 JSON](docs/learning/m1-headless-text.md)
- [M1 TypeScript 到 Go 导航](docs/mappings/typescript-to-go/m1-headless-text.md)
- [M2.2 Usage、cost 与 cache](docs/learning/m2-usage-cost-cache.md)
- [M2.2 TypeScript 到 Go 导航](docs/mappings/typescript-to-go/m2-usage-cost-cache.md)
- [M2.3 Deferred response 生命周期](docs/learning/m2-deferred-response.md)
- [M2.3 TypeScript 到 Go 导航](docs/mappings/typescript-to-go/m2-deferred-response.md)
- [M2.4 Deferred tools 与动态 Tool 定义](docs/learning/m2-deferred-tools.md)
- [M2.4 TypeScript 到 Go 导航](docs/mappings/typescript-to-go/m2-deferred-tools.md)
- [M2.5 内存 Telemetry 与 adapter conformance](docs/learning/m2-telemetry.md)
- [M2.5 TypeScript 到 Go 导航](docs/mappings/typescript-to-go/m2-telemetry.md)
- [M0 兼容骨架](docs/learning/m0-compatibility-skeleton.md)
- [M0 TypeScript 到 Go 导航](docs/mappings/typescript-to-go/m0.md)

Headless Coding Agent 支持最终文本输出和 session-first JSONL 事件流。JSON 模式的第一行是内存 v3 Session header，之后每行是一个 `AgentSessionEvent`；stdin 始终作为 Prompt，不是 RPC command：

```sh
export DEEPSEEK_API_KEY=...
go run ./cmd/pig --provider deepseek --model deepseek-v4-flash --mode json "Explain this package"
```

无需凭证或网络的确定性示例：

```sh
go run ./examples/headless-json
```

```sh
make m0-gate
```

`m0-gate` 只重放仓库内已提交的 fixture，全程离线且不需要 Pi checkout。需要重新对照上游源码时，按 [M0 兼容骨架](docs/learning/m0-compatibility-skeleton.md#冻结门禁) 准备两个独立 checkout 后运行 `m0-freeze`。

M1 冻结门禁在 `m0-freeze` 之上追加受保护的真实 DeepSeek live smoke。普通 PR 缺 `DEEPSEEK_API_KEY` 时 `make m1-live-smoke` 明确 skip；`make m1-freeze`（含 `PIG_REQUIRE_LIVE=1`）缺密钥时必须失败。
