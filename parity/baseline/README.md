# Parity Baseline

本目录锁定 Pig V1 的**对等基线（Parity Baseline）**。基线由两件不可变制品共同组成，缺一不可：

1. **Pi 源码 commit** —— `upstream.lock.json` 记录仓库、固定 commit `936aff00918de1187f085f123c2812d8f2d67745`、许可与源码校验方式。
2. **Catalog Snapshot** —— `snapshot.manifest.json` 记录配套模型目录数据的 manifest（生成时间、输入来源、每个 artifact 的 SHA-256、Provider/model 数量、许可与归属）。

固定这两件制品而非追踪 Pi `main`，是为了让“1:1 完成”成为可复现、可审计的命题（见 `docs/adr/0001-baseline-and-oracle.md`）。

## Pi 不是依赖

Pi 只作为按需获取并校验的 Oracle。它**不是** submodule、复制进来的源码树，也不是 Pig 运行时依赖。Pi Oracle 按需检出到被忽略的 `.upstream/pi`（见仓库根 `.gitignore`），或验证用户提供的现有 checkout 恰好位于锁定 commit。

许可与归属见仓库根 `THIRD_PARTY_NOTICES` 及 `docs/adr/0012-license-and-upstream-attribution.md`：Pi 为 MIT，Copyright (c) 2025 Mario Zechner。

## 离线验证

普通 build/test 只读取本目录已锁定的制品，不联网、不需要 Node 或 Provider 凭证。离线验证器运行：

```sh
go test ./internal/baseline/...
```

验证器（`internal/baseline`）做结构与完整性校验：必填字段、`upstream.commit` / `source_verification.expected_commit` / `manifest.baseline_commit` 三者一致、`not_a_submodule` 与 `not_a_runtime_dependency` 为真、manifest 按 `status` 的完整性规则。commit、数据或 hash 不匹配时明确失败，不回退到 live fetch。`internal/baseline` 不被 `cmd/pig` 与 `cmd/pig-ai` 引用，命令二进制保持无 net/os/exec 依赖。

## Catalog Snapshot 当前为 pending-capture

`snapshot.manifest.json` 目前处于诚实的 **`pending-capture`** 状态。原因：Pi commit `936aff0` 没有提交完整的 chat-model 真实数据，发布过程通过网络生成模型 shard。要重现当时的 Provider、model ID、API、价格、context window、max tokens 和 compatibility metadata，必须用该 commit 的生成器进行一次受控的网络+Node 抓取。**该受控抓取尚未执行**，因此 manifest 不含任何 artifact、hash 或生成时间戳——这些一律不得伪造。

- `artifacts` 为空；`generation.generated_at` 为 `null`；`providers`/`models` 为 0。
- 受控抓取步骤见 `docs/specs/model-catalog.md`（§Snapshot 获取顺序、§Snapshot 内容）。
- 抓取完成后，manifest 应转为 `status: "captured"`，填入真实生成时间、输入来源与每个 artifact 的 SHA-256；届时验证器会强制校验磁盘文件 hash 一致。

验证器强制这一诚实性：`pending-capture` 若携带任何伪造的 hash、时间戳或 artifact，即判为失败。
