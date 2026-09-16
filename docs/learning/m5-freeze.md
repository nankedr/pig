# M5 本地资源集成与冻结

v0.5.0 按 [ADR-0034](../adr/0034-defer-package-ecosystem.md) 交付本地资源组合流程。Context File、system prompt、Prompt Template、Skill、Theme、优先级、冲突诊断和 Session 重载沿用固定 Pi 基线；扩展只发现入口，执行仍明确未实现。包 manifest、登记、npm/git、依赖和 lifecycle 由 [#99](https://github.com/nankedr/pig/issues/99) 保留为 V1 未完成范围，未排期，不自动进入 M7/M14。

## 运行组合流程

```sh
go run ./examples/m5-workflow
```

示例分别以可信项目、拒绝信任、显式资源路径运行公开 SDK：加载本地资源 → 调用模板及 Skill → 修改资源 → `AgentSession.Reload` → 核对查询和模型输入 → 保留原 Session 历史继续 → 重开 v3 文件 → 使用新主题导出 HTML。Faux Provider 检查真实生成输入，失败非零退出，无需在线凭证。

`internal/m5gate/cli_test.go` 使用真实 CLI 子进程和受控回环 Provider，空 PATH 中不存在 Node/npm/git。测试调用模板与 Skill、退出、修改资源、恢复同一 Session 并继续、导出新主题，同时检查 package manifest 没有触发安装或扩展副作用。CLI 在重开时加载资源；活动 Session 的重载由公开 SDK 提供，没有新增 RPC reload 命令。`get_commands` 支持模板和 Skill 查询；扩展命令未交付。

拒绝项目信任仍加载 Context File，项目 system prompt 不进入生成输入。显式资源路径按已交付契约处理，禁用自动发现仍可使用显式路径。Project Trust 不是 Tool 审批或 Sandbox；Skill 自带的辅助脚本可能需要外部工具。

## 门禁和范围

```sh
make m5-gate
make m5-freeze \
  PIG_PI_ORACLE_CHECKOUT=/path/to/prepared/pi \
  PIG_PI_SOURCE_CHECKOUT=/path/to/pristine/pi
```

`m5-gate` 继承 M1–M4 全量 Go 回归、race、vet、darwin/arm64 无 CGO 构建、示例、取消/回收和真实 RPC/七工具重复验证；增加 20 次重载、取消、信任及资源所有权回归和 5 次 CLI/SDK 集成回归。普通测试只消费已提交离线 fixture，回环 HTTP 不访问在线 Provider，不运行 Node/npm 或 Oracle。

`m5-freeze` 检查前后干净 checkout，复核八项本地资源 Oracle、已有 M1–M4 Oracle、source drift、Catalog/API snapshot、真实浏览器主题/CSP/XSS，以及必须提供凭证的 DeepSeek 冒烟。Code Baseline 为 `936aff00918de1187f085f123c2812d8f2d67745`，Catalog Baseline 为 `53fa77ccd8a279eb87e92294ef3687b03ff80112`。预装 rg/fd、支持 strip-types 且 Unicode 16.0 的 Node、锁定 Pi dist 和 Playwright/Chromium；准备方法见 [M0](m0-compatibility-skeleton.md#冻结门禁)与 [M4](m4-freeze.md#离线与完整冻结)。Node/npm 仅用于维护者复核 Pi Oracle，不是普通本地资源运行依赖。

| 范围 | 证据 |
| --- | --- |
| Context File / system prompt | `TestContextFilesSDKParity`、`TestSystemPromptsSDKParity`、真实 CLI 对应测试 |
| 模板 / Skill | `TestPromptTemplatesSDKParity`、`TestSkillsSDKParity`、`TestPigPromptTemplates`、`TestPigSkills` |
| Theme / HTML | `TestThemesColorsSDKParity`、`TestPigThemesHTMLParity`、`parity/export-html/check.mjs` |
| 来源 / 优先级 / 冲突 | `TestLocalResourcesSDKParity`、`TestPigLocalResourceDiagnostics` |
| 重载 / 并发 / 取消 | `TestSessionReloadSDKParity`、`TestSessionReloadCancelFailureAndRecovery`、`TestSessionReloadConcurrentQueries` |
| 扩展发现 / 无执行 | `TestLocalExtensionsSDKParity`、`TestLocalExtensionsNoSensitiveReads`、`TestPigLocalExtensionsNeverExecute` |
| CLI / SDK 组合与恢复 | `TestPigM5LocalWorkflow`、`TestM5Workflow` |

[Catalog](../../parity/catalog.jsonl) 保留逐项 partial；[冻结范围快照](../../internal/m5gate/testdata/catalog_scope.txt) 固定 M5 全部条目及产品入口的状态、target、partial SHA-256。auth/layout 迁移的 inventoried 条目和包管理/扩展 Stub 不因冻结而提升。现有 API snapshot 全量验证，本次只提升产品版本，不改变资源 API 或冻结扩展 ABI。

## 安装和发布制品

```sh
go install github.com/nankedr/pig/cmd/pig@v0.5.0
go install github.com/nankedr/pig/cmd/pig-ai@v0.5.0
go get github.com/nankedr/pig@v0.5.0
```

在完成提交的干净源码目录执行：

```sh
python3 scripts/m5-release.py /tmp/pig-v0.5.0-release
```

脚本对确切 commit 执行完整冻结，安装无 CGO CLI，在独立 module 中运行公开 SDK 组合示例，以安装后二进制重跑 CLI 组合和浏览器门禁。制品包括 CLI tar.gz、根及第三方许可证、本地主题及 HTML 资产声明、README/发布说明、freeze/SDK/CLI/browser 日志、`verification.json` 和 `SHA256SUMS`。记录双来源基线、平台、工具链、源码 commit、Catalog/API/制品哈希；任何失败不能作为发布成功证据。

给通过验证的同一 commit 创建 `v0.5.0` tag 后复核公开安装：

```sh
python3 scripts/m5-release.py --verify-published /tmp/pig-v0.5.0-release
```

此步骤使用全新 module cache，从公开版本安装两个 CLI 和 SDK，无本地 replace，再跑组合及浏览器验证，检查 Go module Origin.Hash 等于冻结 commit，补充 `published-install.json/log` 和 SHA256SUMS。Release 上传这些证据及归档，缺少此步骤只能声明本地候选验证。

发布硬平台仍为 darwin/arm64，六平台行为留待 M13。#108 不关闭父 #7、不自动推进 Milestone Frontier；由维护者另行决定。源码路径见 [TypeScript → Go](../mappings/typescript-to-go/m5-freeze.md)。
