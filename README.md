# Pig

Pig 是 Pi 固定版本的 Go 语义兼容实现。当前 Milestone Frontier 是 M1；M0 已建立七包、双命令、Parity Catalog、Capability Stub 与冻结门禁骨架。

- [文档导航](docs/README.md)
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
