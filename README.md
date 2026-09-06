# Pig

Pig 是 Pi 固定版本的 Go 语义兼容实现。v0.3.0 集成 M3 的本地持久化 Coding Agent：v3 Session 恢复与互操作、继续/fork、全局与可信项目 settings、canonical 凭证、基础 ModelRuntime，以及默认四工具的可恢复编码任务。

M3.8 支持显式 write 创建/覆盖文件并继续对话，见 [write 与回读](docs/learning/m3-write-tool.md)。
M3.9 支持显式 edit 精确/模糊多区域替换、准确 diff 和回读，见 [edit 与回读](docs/learning/m3-edit-tool.md)及 [TypeScript → Go](docs/mappings/typescript-to-go/m3-edit-tool.md)。
M3.10 支持显式 bash 执行宿主命令、进程树取消与完整输出保留，见 [bash 与回读](docs/learning/m3-bash-tool.md)及 [TypeScript → Go](docs/mappings/typescript-to-go/m3-bash-tool.md)。
M3.11 默认启用 read/bash/edit/write，支持跨进程恢复和 fork 编码任务，见 [可恢复编码任务](docs/learning/m3-coding-task.md)及 [TypeScript → Go](docs/mappings/typescript-to-go/m3-coding-task.md)。
M4.4 支持 Provider 错误后的整轮重试，见 [重试与取消](docs/learning/m4-turn-retry.md)及 [TypeScript → Go](docs/mappings/typescript-to-go/m4-turn-retry.md)。

- [M3 集成与冻结](docs/learning/m3-freeze.md)
- [M3 TypeScript 到 Go 导航](docs/mappings/typescript-to-go/m3-freeze.md)
- [v0.3.0 发布说明](docs/releases/v0.3.0.md)
- [文档导航](docs/README.md)
- [M3.6 canonical 凭证恢复](docs/learning/m3-credentials.md)
- [M3.6 TypeScript 到 Go 导航](docs/mappings/typescript-to-go/m3-credentials.md)
- [M3.5 Project Trust](docs/learning/m3-project-trust.md)
- [M3.5 TypeScript 到 Go 导航](docs/mappings/typescript-to-go/m3-project-trust.md)
- [M2 集成与冻结](docs/learning/m2-freeze.md)
- [M2 TypeScript 到 Go 导航](docs/mappings/typescript-to-go/m2-freeze.md)
- [v0.2.0 发布说明](docs/releases/v0.2.0.md)
- [M1 Headless text 与 JSON](docs/learning/m1-headless-text.md)
- [M3.4 全局设置](docs/learning/m3-global-settings.md)
- [M3.4 TypeScript 到 Go 导航](docs/mappings/typescript-to-go/m3-global-settings.md)
- [M3.1 v3 Session 持久化](docs/learning/m3-session-persistence.md)
- [M3.1 TypeScript 到 Go 导航](docs/mappings/typescript-to-go/m3-session-persistence.md)
- [M1 TypeScript 到 Go 导航](docs/mappings/typescript-to-go/m1-headless-text.md)
- [M2.2 Usage、cost 与 cache](docs/learning/m2-usage-cost-cache.md)
- [M2.2 TypeScript 到 Go 导航](docs/mappings/typescript-to-go/m2-usage-cost-cache.md)
- [M2.3 Deferred response 生命周期](docs/learning/m2-deferred-response.md)
- [M2.3 TypeScript 到 Go 导航](docs/mappings/typescript-to-go/m2-deferred-response.md)
- [M2.4 Deferred tools 与动态 Tool 定义](docs/learning/m2-deferred-tools.md)
- [M2.4 TypeScript 到 Go 导航](docs/mappings/typescript-to-go/m2-deferred-tools.md)
- [M2.5 内存 Telemetry 与 adapter conformance](docs/learning/m2-telemetry.md)
- [M2.5 TypeScript 到 Go 导航](docs/mappings/typescript-to-go/m2-telemetry.md)
- [M2.6 compat 与 Session Resource](docs/learning/m2-compat-session-resources.md)
- [M2.6 TypeScript 到 Go 导航](docs/mappings/typescript-to-go/m2-compat-session-resources.md)
- [M2.9 Legacy Agent 队列](docs/learning/m2-legacy-agent-queues.md)
- [M2.9 TypeScript 到 Go 导航](docs/mappings/typescript-to-go/m2-legacy-agent-queues.md)
- [M0 兼容骨架](docs/learning/m0-compatibility-skeleton.md)
- [M0 TypeScript 到 Go 导航](docs/mappings/typescript-to-go/m0.md)

Headless Coding Agent 支持最终文本输出和 session-first JSONL 事件流，并默认把 v3 Session 写入 `~/.pig/agent/sessions/`。JSON 模式第一行是 v3 Session header，之后每行是一个 `AgentSessionEvent`；stdin 始终作为 Prompt，不是 RPC command。使用 `--session <path>` 重开，或用 `--no-session` 明确关闭持久化：

```sh
export DEEPSEEK_API_KEY=...
go run ./cmd/pig --provider deepseek --model deepseek-v4-flash --mode json "Explain this package"
```

无需凭证或网络的确定性示例：

```sh
go run ./examples/headless-json
go run ./examples/session-persistence
go run ./examples/global-settings
```

```sh
make m3-gate
```

`m3-gate` 只重放仓库内已提交的 fixture，全程离线且不需要 Pi checkout。需要重新对照上游源码时，按 [M0 兼容骨架](docs/learning/m0-compatibility-skeleton.md#冻结门禁) 准备两个独立 checkout 后运行 `m0-freeze`。

M1 冻结门禁在 `m0-freeze` 之上追加受保护的真实 DeepSeek live smoke。普通 PR 缺 `DEEPSEEK_API_KEY` 时 `make m1-live-smoke` 明确 skip；`make m1-freeze`（含 `PIG_REQUIRE_LIVE=1`）缺密钥时必须失败。

M2 完整冻结使用 `make m2-freeze`：要求干净 Pig checkout，追加全部 M2 Oracle、source drift 与真实 DeepSeek 冒烟。准备方式与 Catalog 剩余边界见 [M2 集成与冻结](docs/learning/m2-freeze.md)。

M3 完整冻结使用 `make m3-freeze`，包含 M1/M2 回归和 M3 的真实子进程恢复、四工具、信任与凭证验收。安装 CLI：`go install github.com/nankedr/pig/cmd/pig@v0.3.0`；SDK：`go get github.com/nankedr/pig@v0.3.0`。复现与制品说明见 [M3 集成与冻结](docs/learning/m3-freeze.md)。
