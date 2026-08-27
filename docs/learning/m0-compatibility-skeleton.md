# M0 兼容骨架

M0 的成果是可编译、可审计的兼容骨架，不是可运行的 AI Agent。它锁定 Parity Baseline、七个包的边界、两个命令、完整目标清单和 Capability Stub；真正的端到端能力从 M1 开始按 Milestone Frontier 实现。

## 如何理解双来源基线

- Code Baseline 是 Pi `936aff00918de1187f085f123c2812d8f2d67745`，负责源码表面、行为和 Oracle。
- Catalog Baseline 是 Pi v0.84.1 官方 source tar，来源 commit 为 `53fa77ccd8a279eb87e92294ef3687b03ff80112`，固定 39 个 Provider 和 1220 个 chat model。

两个 commit 相差 40 个 commit。`936aff0` 当次生成目录的 artifact 已过期且没有可恢复副本，所以 M0 选择最近的较早正式发布物，而不拿实时目录冒充历史结果。这能分别固定代码与目录证据，但不能称为 fixed-run parity。

## 如何观察 M0

运行 Go SDK 示例：

```sh
go run ./examples/m0-contracts
```

示例调用公开的 OpenAI Completions 边界。M0 不访问网络，也不返回伪造消息，而是由 `AssistantMessageEventStream.Result` 明确给出 `ErrNotImplemented`。输出中的 `true` 证明调用者可以稳定识别 Capability Stub。

七个公开包分别是 `ai`、`agent`、`codingagent`、`telemetry`、`tui`、`protocol` 和 `client`；命令是 `pig` 与 `pig-ai`。M0 只要求本机 `darwin-arm64` 的 CGO-free 编译，不包含 browser、Worker、WebAssembly 或六平台发布门禁。

## 冻结门禁

普通门禁只执行 Go 构建、测试和已提交 fixture 的离线重放，不需要 Pi checkout、Node 或网络：

```sh
make m0-gate
```

完整冻结另行执行 Pi differential Oracle 和源码漂移，并要求支持 TypeScript strip 的 Node `>=22.6.0`。先准备两个位于 Code Baseline 的独立 checkout：Oracle checkout 安装依赖并生成 `dist`，source checkout 不运行 npm、build 或生成器。门禁本身不会 fetch、install 或 build：

```sh
git clone https://github.com/badlogic/pi-mono /path/to/pi-oracle
git -C /path/to/pi-oracle checkout --detach 936aff00918de1187f085f123c2812d8f2d67745
npm --prefix /path/to/pi-oracle ci --ignore-scripts
npm --prefix /path/to/pi-oracle run build:offline

git clone https://github.com/badlogic/pi-mono /path/to/pi-source
git -C /path/to/pi-source checkout --detach 936aff00918de1187f085f123c2812d8f2d67745

npm ci --prefix parity/extract
make m0-freeze \
  PIG_PI_ORACLE_CHECKOUT=/path/to/pi-oracle \
  PIG_PI_SOURCE_CHECKOUT=/path/to/pi-source
```

Oracle checkout 必须 HEAD 精确匹配、tracked clean，并预先具备所需 `node_modules` 和 `packages/coding-agent/dist/cli.js`。source checkout 还会拒绝任何 untracked 或 ignored 文件，不能复用 Oracle checkout。源码漂移要求仓库侧 `parity/extract/node_modules/typescript` 精确为 `5.9.3`，并逐字节复核 Surface 与从 `IMAGE_MODELS` 导出的 image catalog。

门禁覆盖 Catalog、Inventory、公开 Surface、Chat Completions matrix、API 编译快照、首个 Parity Case、Capability Stub 副作用、race、vet、本机编译、示例和文档导航。Capability Status 仍只由 `parity/catalog.jsonl` 决定。

## 下一步

M1 是当前 Milestone Frontier。它会实现 faux 与 DeepSeek/OpenAI Chat Completions 核心路径，并在首个端到端可运行切片后冻结相应接口；M0 的 `inventoried`、`scaffolded` 和 `partial` 不能解释成已经具备这些行为。
