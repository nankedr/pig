# M4 编排集成与冻结

M4/v0.4.0 集成七个内置 Tool、消息投递、整轮 retry、模型/thinking/Tool 切换、统计、手动/自动压缩、SDK 树导航和分支摘要、Coding Agent JSONL RPC、安全 HTML export。默认 Tool 集仍为 read/bash/edit/write，grep/find/ls 需显式选择。

## 从组合示例开始

```sh
go run ./examples/m4-workflow
```

此公开 SDK 示例用 Faux 和临时 v3 Session，执行一次失败后的整轮 retry，消费 steering/follow-up，切换模型/thinking/Tool，创建两个分支，保存离开分支摘要，压缩后继续，再重开核对消息与 stats。它检查实际模型输入和持久化结果，任何环节失败都以非零状态退出。

`TestPigM4SevenToolsResumeAndFork` 在真实 `pig` 子进程分别运行 text/json：write → read → edit → bash → 失败的 bash → ls → find → grep，退出后继续并 fork。受控 HTTP Provider 检查工具结果、消息历史和提示词；v3 reader 验证历史、失败记录与源文件隔离。真实 RPC 的进程重启、压缩、替换和继续由 `TestRPC96ClientLifecyclePersistence` 验证。

## 离线与完整冻结

```sh
make m4-gate
make m4-freeze \
  PIG_PI_ORACLE_CHECKOUT=/path/to/prepared/pi \
  PIG_PI_SOURCE_CHECKOUT=/path/to/pristine/pi
```

`m4-gate` 继承 M1–M3 的全量 Go 测试、race、vet、darwin/arm64 无 CGO 构建、示例及并发回归，再执行 20 次 M4 Session 生命周期、取消、进程回收和随机顺序回归，以及 5 次真实 RPC/七工具子进程测试。门禁要求预装 `rg` 和 `fd`，缺失会立即失败；普通 `go test` 可跳过依赖真实二进制的七工具集成。准备工具后离线 gate 不访问 Pi 或真实 Provider，回环 HTTP 和宿主进程权限仍需可用。

`m4-freeze` 要求源码前后干净，加入固定 Pi Oracle、source/API/catalog drift、正式 v3 双向互操作、Go API snapshot、精确 Catalog 范围审计、HTML 实际浏览器渲染/XSS、受保护 DeepSeek 发布冒烟。缺少真实凭证必须失败。两个 Pi checkout 均固定 `936aff00918de1187f085f123c2812d8f2d67745`；prepared checkout 带锁定依赖和 dist，pristine checkout 无 tracked/untracked/ignored 变更。Node 需支持 strip-types 且 Unicode 为 16.0（例如 24.4.1）。准备方式见 [M0](m0-compatibility-skeleton.md#冻结门禁)。独立 Catalog Baseline 仍是 `53fa77ccd8a279eb87e92294ef3687b03ff80112`，不宣称 fixed-run 模型目录对等。

浏览器预装 Playwright 与 Chromium；模块不可直接解析时设置 `PIG_PLAYWRIGHT_MODULE` 为 Playwright `index.mjs` 绝对路径，`PIG_CHROMIUM` 为浏览器可执行文件。`make m4-html-browser` 对真实 CLI 导出运行 Markdown、代码高亮、分支切换及 XSS 攻击测试，并断言没有自动 HTTP 请求。浏览器 Oracle 入口是 `parity/oracle/export-html.mjs`。

## 逐项范围与证据

[Catalog](../../parity/catalog.jsonl) 是唯一权威；[冻结快照](../../internal/m4gate/testdata/catalog_scope.txt) 按 ID 固定每个 M4 条目和产品入口的状态、Go target、精确 partial 内容的 SHA-256。`inventoried/scaffolded` 是静态映射，不能当作行为已实现。`contract:rpc/command-union` 保留 inventoried，RPCCommand.Data 也不是可用的扁平命令序列化器。

| 兼容面 | 可执行证据 | 精确边界 |
| --- | --- | --- |
| grep/find/ls + 默认四工具 | `TestGrepToolSessionParity`、`TestFindLsSessionParity`、`TestPigM4SevenToolsResumeAndFork` | fd/rg 预装；无自动下载；平台细节见 ADR-0022/0023，六平台待 M13 |
| SendUserMessage/Prompt、Steer/FollowUp、队列模式/查询/ClearQueue | `TestSessionMessagesParity`、`TestTurnRetryPreservesQueuedMessages` | 图片 M12；模板/skills M5；扩展 M7；手动压缩期投递及 retry backoff 新入队不支持 |
| retry/settings/AbortRetry | `TestTurnRetryParity`、`TestTurnRetryHeadlessCancellation` | 已实现 Provider adapters；畸形 wire 不伪装成可重试成功 |
| Set/CycleModel、thinking、SetActiveToolsByName | `TestSessionConfigurationParity`、`TestSessionConfigurationRuntimeAndRestore` | 运行时忙拒绝；固定 Pi 无 active-tool RPC；models.json/overlay/新 Provider 待 M10/M11 |
| stats/context usage/last assistant | `TestSessionStatsParity`、`TestSessionStatsRuntimeAndRestore` | 保留未知 usage 与运行中快照语义 |
| Compact、自动阈值/溢出恢复 | `TestManualCompactionParity`、`TestAutoCompactionParity` | 扩展自定义摘要/hooks 待 M7；上下文恢复与持久历史分开 |
| NavigateTree、GenerateBranchSummary、AbortBranchSummary | `TestSessionTreeNavigationParity`、`TestBranchSummaryParity`、`TestM4Workflow` | SDK 树导航已交付；未标记导航不持久化游标；摘要队列 draining 未交付 |
| ExecuteBash/AbortBash/RecordBashResult | `TestSessionBashParity`、`TestSessionBashAbortKillsProcessTree` | 注入 Operations 可移植；本地执行沿用 Darwin/Linux，交互式 TUI M6、扩展 M7 |
| RPC framing/ID/events/errors/process lifecycle | `TestRPC94ParityDispatch`、`TestRPC94ClientProcessFailures`、`TestRPC94AbortAndConcurrentEvents` | 字符串 typed client ID；raw wire 接受任意 JSON ID；本地 Context 取消等待不等于远端 Abort |
| RPC 运行时控制 | `TestRPC95ParityControl`、`TestRPC95ClientRunningControl`、`TestRPC95ClientBashAndCancellation` | raw Bash 支持 excludeFromContext，既有 SDK Bash 签名不增加参数 |
| RPC 替换/查询/压缩 | `TestRPC96ParityLifecycle`、`TestRPC96ConcurrentCompactionReplacement`、`TestRPC96AutoCompactionAfterReplacement` | 新工厂失败保留旧会话，旧事件不得污染新绑定；固定 Pi 无 navigate_tree/abort_compaction wire |
| CLI/SDK/RPC HTML export | `TestIssue97Export*`、`TestRPC97*`、`parity/export-html/check.mjs` | 消息图片 M12；自定义 theme M5；扩展 HTML renderer M7；ExportToJSONL SDK 仍为 Stub |

RPC 已交付命令逐项对应：`prompt/abort/get_state/get_messages/get_last_assistant_text`；`steer/follow_up/set_steering_mode/set_follow_up_mode`；`set_model/cycle_model/get_available_models/set_thinking_level/cycle_thinking_level/get_available_thinking_levels`；`set_auto_retry/abort_retry/bash/abort_bash/get_session_stats`；`new_session/switch_session/fork/clone/get_fork_messages/get_entries/get_tree/set_session_name/compact/set_auto_compaction`；`export_html`。`get_commands`、extension/UI 路径仍未交付。

通过 RPC 扩展 command context 触发同一 Session 树的导航、离开分支摘要、后续生成及竞争验证，明确属于 **M7 / #9**。直接 RPC 能力与公开 SDK 摘要导航不能证明完整 Extension Surface 已实现。资源/包系统 M5、TUI M6、扩展 M7、Harness M8、CBOR Remote Session Protocol/Client M9、其余模型与 Provider M10/M11、图片 M12、六平台 M13 继续按各自 Catalog 边界验收。JSONL RPC 不代表 CBOR 协议已完成。

## 安装与验证制品

```sh
go install github.com/nankedr/pig/cmd/pig@v0.4.0
go install github.com/nankedr/pig/cmd/pig-ai@v0.4.0
go get github.com/nankedr/pig@v0.4.0
```

审查并提交候选变更，在 darwin/arm64 干净 checkout 设置上述依赖与受保护环境后执行：

```sh
python3 scripts/m4-release.py /tmp/pig-v0.4.0-release
```

脚本执行完整 freeze，然后安装无 CGO CLI、在独立 Go module 中验证 SDK、以安装的二进制重跑浏览器门禁，生成 tar.gz、`m4-freeze.log`、`installed-html-browser.log`、`verification.json` 与 `SHA256SUMS`。验证记录包含源码 commit、工具链、双来源基线及 Catalog/API/日志/二进制校验值；任意失败返回非零，不得据此创建发布 tag。对通过验证的同一 commit 创建 `v0.4.0` tag/Release，并上传这些制品；tag 发布后再次验证版本化 CLI/SDK 安装。

Project Trust 不是 Tool 审批或 Sandbox，工具继承宿主权限。发布硬门仍为 darwin/arm64。#98 不修改或关闭父 Issue，里程碑前沿由父级维护流程单独推进。学习路径见 [源码导航](../mappings/typescript-to-go/m4-freeze.md)，产品范围见 [发布说明](../releases/v0.4.0.md)。
