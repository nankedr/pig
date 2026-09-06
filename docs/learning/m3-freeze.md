# M3 集成与冻结

Issue #82 收口 #71–#81 的本地持久化 Coding Agent。CLI `pig --version` 和 Go SDK `codingagent.Version` 统一为 `0.3.0`。已交付行为包括 v3 Session 持久化、历史恢复和双向互操作、继续/fork、全局与可信项目 settings、Context File、canonical 凭证、基础 ModelRuntime，以及默认 read/bash/edit/write 的可恢复编码任务。

## 离线门禁

```sh
make m3-gate
```

`m3-gate` 继承 `m2-gate`：全量 Go 测试、race、vet、darwin/arm64 无 CGO 构建、全部可运行示例和 M2 的 20 次随机顺序并发回归。它再执行 M3 的 20 次 Session 生命周期、文件修改队列和 Bash 取消/超时回归，以及 5 次 settings/trust/auth 跨进程锁、真实 CLI 四工具恢复/fork、信号取消和 pre-trust 零读取回归。超时、竞态或子进程未退出均视为失败。

普通 gate 不使用 Pi、Node 或真实凭证，只重放提交的 fixture、Faux 和本地受控 HTTP 服务。测试入口移除 live/drift 的 opt-in 环境变量；本地 fake server 仍需回环端口和宿主子进程权限。M3 不改变 M1/M2 的已冻结契约。

## 完整冻结

从干净 Pig checkout 执行，日志放在仓库外或忽略目录：

```sh
make m3-freeze \
  PIG_PI_ORACLE_CHECKOUT=/path/to/prepared/pi \
  PIG_PI_SOURCE_CHECKOUT=/path/to/pristine/pi
```

两个 checkout 都必须固定在 Code Baseline `936aff00918de1187f085f123c2812d8f2d67745`。prepared checkout 预装锁定依赖和 dist；pristine checkout 不得有 tracked、untracked 或 ignored 状态。edit Oracle 的规范化基线要求 Node 携带 Unicode 16.0（本次使用 Node 24.4.1）；`m3-node-preflight` 在耗时验证前拒绝不匹配版本，`node -p process.versions.unicode` 可自行核对。Node 及锁定 TypeScript 提取器的准备见 [M0 冻结门禁](m0-compatibility-skeleton.md#冻结门禁)。gate 不自动下载、安装或构建 Pi。

冻结依次执行干净工作区检查、离线集成、全部 M0–M3 Pi Oracle、source inventory / TypeScript API / 图片目录 drift 和受保护 DeepSeek 冒烟，最后再次检查 Pig 工作区。`session-interop.mjs --check` 实际运行 Pig writer，并由固定 Pi 正式 reader 消费；Pi writer 的历史 fixture 由 Go reader 重放，因此覆盖两个方向。互操作示例显式使用空 Tool 集合和空系统提示词，防止默认工具及提示词改变 Faux usage；四工具集成单独由 coding-task 示例和真实 CLI 验证。M3 的 Go API snapshots 和 Catalog 精确证据检查随全量测试运行。

`DEEPSEEK_API_KEY` 通过既有受保护环境提供。`m1-live-smoke` 强制 `PIG_REQUIRE_LIVE=1`，缺少凭证必须失败；它只验证小 token 文本流和真实 read continuation，不记录 key、请求、响应或文件内容。Code Baseline 和独立 Catalog Baseline `53fa77ccd8a279eb87e92294ef3687b03ff80112` 沿用 [ADR-0014](../adr/0014-dual-source-parity-baseline.md)，不声称 fixed-run 目录对等。

## 可追溯证据和边界

[Parity Catalog](../../parity/catalog.jsonl) 是能力状态的唯一权威。`internal/m3gate` 固定 M3 的 ID 与状态快照；对已交付行为契约逐项要求完整、可定位的执行证据和精确 partial 范围。各切片已有门禁进一步检查 fixture/hash、API snapshot 和对应公开行为。快照包含 API 映射的 `scaffolded` 与 TypeScript 投影的 `inventoried`，这些状态只表示映射，不能解释为行为已经完成。`contract:config/models-json` 明确保留 inventoried：自定义 models.json、Provider 定义、overlay、缓存与刷新等待 M10。

| 验收内容 | 可复现证据 |
| --- | --- |
| pre-trust 零读取，拒绝信任不加载项目 settings | `cmd/pig/issue75_process_test.go#TestPigUntrustedProjectHasNoSensitiveReadsOrEffects` 使用受控读取探针；`codingagent/issue75_settings_test.go` |
| Context File 例外、worktree 路径去重 | `codingagent/issue75_context_test.go`、`cmd/pig/issue75_process_test.go` |
| Pig/Pi 状态隔离 | `cmd/pig/issue71_process_test.go#TestPigExplicitPiSessionDoesNotMigrateAdjacentPiState`、`codingagent/issue75_trust_test.go`、`codingagent/issue76_credentials_test.go` |
| 凭证权限、跨进程锁、命令引用、取消及脱敏 | `codingagent/issue76_credentials_test.go`、`ai/issue76_redaction_test.go`、`cmd/pig/issue76_process_test.go` |
| 默认零隐式控制面外联 | `codingagent/issue77_runtime_test.go#TestModelRuntime77EmbeddedSnapshotAndNoControlPlane`、`cmd/pig/issue77_process_test.go` |
| v3 双向互操作与跨重启 | `parity/oracle/session-interop.mjs`、`internal/parity/session_interop_test.go`、`cmd/pig/issue72_process_test.go` |
| 四工具、失败结果、恢复/fork 与信号取消后重开 | `cmd/pig/issue81_process_test.go`、`cmd/pig/issue80_signal_unix_test.go` |

Project Trust 只控制项目配置和资源加载，不是 Tool 审批或 Sandbox。Context File 即使未信任也能进入提示词；工具按宿主权限访问文件和运行 shell。显式推理与 Bash 网络行为不属于隐式控制面外联。文件锁本期验收 Darwin/Linux，发布硬门为 darwin/arm64，六平台验收仍在 M13。

资源/skills/templates/themes 和包管理等待 M4，TUI 与交互式 `--resume` 等待 M5；RPC、扩展和 Harness 仍按 M6/M7/M8 推进。其余 Provider 协议、模型配置/刷新、OAuth/ambient auth、图片分别保留 M10/M11/M12 边界。AgentSession 编排式树导航、自动压缩、grep/find/ls 等以各自 Catalog 条目为准。冻结不批量提升这些条目的状态，也不更新或关闭父 Issue #5。

## 安装、学习与发布制品

Go 1.24 或更新版本：

```sh
go install github.com/nankedr/pig/cmd/pig@v0.3.0
go install github.com/nankedr/pig/cmd/pig-ai@v0.3.0
pig --version
# 在使用 SDK 的 Go module 中执行：
go get github.com/nankedr/pig@v0.3.0
```

离线学习从 `go run ./examples/coding-task` 开始，再依次查看 [M3 源码导航](../mappings/typescript-to-go/m3-freeze.md) 中各切片的示例、Pi 源码与验证。`pig-ai` 可安装，但其未实现命令继续返回明确 Capability Stub。

发布顺序：审查候选变更 → 提交 → 在干净 checkout 上通过 `m3-freeze` → 对同一 commit 构建并验证安装制品 → 创建 `v0.3.0` tag 和 Release。发布附件包含脱敏后的 `m3-freeze.log`、`verification.json`、本机无 CGO CLI 压缩包和 SHA-256 清单；验证记录应绑定完整 commit、审查基线、Go/Node/平台、双来源基线、Catalog/API/日志/二进制校验值及门禁退出码。日志或凭证不提交到源码。未通过完整 freeze 不得创建发布 tag。发布范围见 [v0.3.0 发布说明](../releases/v0.3.0.md)。
