# Parity Baseline

本目录锁定 Pig V1 的**对等基线（Parity Baseline）**。基线由两件不可变制品共同组成，缺一不可：

1. **Pi 源码 commit** —— `upstream.lock.json` 记录仓库、固定 commit `936aff00918de1187f085f123c2812d8f2d67745`、许可与源码校验方式。
2. **Catalog Baseline 与 Snapshot** —— `upstream.lock.json` 另行锁定 Pi v0.84.1 官方 source tar；`snapshot.manifest.json` 记录其来源 commit、hash、无损派生、Provider/model 数量、许可与归属。

固定这两件制品而非追踪 Pi `main`，是为了让“1:1 完成”成为可复现、可审计的命题（见 `docs/adr/0001-baseline-and-oracle.md` 与 `docs/adr/0014-dual-source-parity-baseline.md`）。

## Pi 不是依赖

Pi 只作为显式准备并校验的 Oracle。它**不是** submodule、复制进来的源码树，也不是 Pig 运行时依赖。普通 `make m0-gate` 只重放已提交 fixture，不读取 `.upstream/pi`、不安装依赖，也不联网。专门的 differential/freeze 检查只接受调用者通过 `PIG_PI_ORACLE_CHECKOUT` 与 `PIG_PI_SOURCE_CHECKOUT` 提供且恰好位于锁定 commit 的 checkout；门禁不会自动 fetch、install 或 build。

许可与归属见仓库根 `THIRD_PARTY_NOTICES` 及 `docs/adr/0012-license-and-upstream-attribution.md`：Pi 为 MIT，Copyright (c) 2025 Mario Zechner。

## 离线验证

普通 build/test 只读取本目录已锁定的制品，不联网、不需要 Node 或 Provider 凭证。离线验证器运行：

```sh
make m0-gate
```

其中 Go 全量测试会重放已提交的 Oracle fixture。验证器（`internal/baseline`）校验双来源 commit 与 release hash、artifact 与 shard hash、JSON schema、Provider 引用、Provider/model 唯一性、39/1220 与 1/42 计数，以及派生结果确为 source shard 的无损扁平化。commit、数据、provenance 或 hash 不匹配时明确失败，不回退到 live fetch。`internal/baseline` 不被 `cmd/pig` 与 `cmd/pig-ai` 引用，命令二进制保持无 net/os/exec 依赖。

## 冻结复核

完整冻结需要 Node `>=22.6.0`，以及两个互不复用的 Code Baseline checkout：Oracle checkout 可以包含安装的依赖与构建产物，source checkout 必须保持原始状态，连 untracked/ignored 文件也不能出现。

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

门禁先验证两个 checkout 的 HEAD。Oracle checkout 还必须保持 tracked clean，并预先具备所需 `node_modules` 与 `packages/coding-agent/dist/cli.js`；source checkout 必须对 tracked、untracked、ignored 三类状态都为空。源码漂移会使用精确的 `typescript@5.9.3` 重提取 Surface，并从上游 `IMAGE_MODELS` 执行 manifest 声明的 `node-import-and-json-stringify`，逐字节比较已提交 image catalog。

## Catalog Snapshot 已锁定

`snapshot.manifest.json` 当前为 **`captured`**；权威 Parity Catalog 用 `contract:baseline/catalog-snapshot` 和 `contract:baseline/image-catalog-snapshot` 两条 verified 记录分别绑定 chat、image 来源。Code Baseline 仍是 `936aff00918de1187f085f123c2812d8f2d67745`；chat Catalog Baseline 独立锁定 Pi v0.84.1 官方 source tar（commit `53fa77ccd8a279eb87e92294ef3687b03ff80112`，比 Code Baseline 早 40 个 commit）：

- 官方 source tar SHA-256：`294d8067eb42327be0db4792d3be792daff588d8fc22549270a972ec9e5407e7`；
- `catalog/chat/source/providers/` 保留 39 个发布 shard，共 1220 个 chat model；
- `catalog/chat/models.json` 只移除 API 分组层级并确定性排序，字段和值不变，`semantic_overlays` 为 0；
- `catalog/image/models.json` 从 Code Baseline 已提交的 image model 源码导出，包含 1 个 Provider、42 个 model；
- 所有制品均排除凭证、cookie、账号标识与请求 header。

`936aff0` 当次生成的 chat artifact 已不可恢复，因此这是明确记录的双来源 baseline，不是 fixed-run parity，也不声称 v0.84.1 数据由 `936aff0` 生成器产出。升级规则见 `docs/specs/model-catalog.md`。
