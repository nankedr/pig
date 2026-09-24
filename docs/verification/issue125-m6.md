# Issue #125 自动验收与发布边界

环境：darwin-arm64；Pi 对等基线 `936aff00918de1187f085f123c2812d8f2d67745`。这是工作区候选的自动验收记录，**不是人工终端验收、完整冻结或已发布版本的证明**。

新 Parity Case 先从固定 Pi 真实子进程提取，再由 Pig CLI 和公开 SDK 程序重放。普通/全屏两份 fixture 位于 `parity/oracle/fixtures/m6-workflow-*.json`。harness 的受控 loopback 服务只使用 synthetic key；记录主题热更新、设置保存、选择器取消、Bash、粘贴与外部编辑器、维护/取消/重载/导出、后续插话与队列的顺序，以及 Session、文件、退出和 termios。

红灯：`TestM6FreezePlan` 最初失败于缺少 `m6-freeze`；组合全屏测试在压缩与多次统计后收到了第四次请求，但终端未显示 RESPONSE_4。最小复现 `go test ./cmd/pig -run TestPigFullscreenReplyAfterNotices125 -count=1` 失败于未显示 REPLY_2。根因是通知始终追加在整个消息列表之后，旧通知占据视口底部。修复将通知锚定到产生时的消息位置，不进入模型上下文；同一 CLI 回归与原始组合已通过。

自动检查入口：

- `make m6-gate`：M1–M5 与 M6 离线回归、race、vet、无 CGO 构建、示例、清理与重复测试。
- `make m6-oracle PIG_PI_ORACLE_CHECKOUT=/prepared/pi`：全部切片和新增组合的固定 Oracle 复核。
- `make m0-source-drift PIG_PI_SOURCE_CHECKOUT=/pristine/pi`：上游文件、符号/成员及数据清单漂移。
- `PIG_BINARY=/installed/pig PIG_M6_SDK_BINARY=/installed/sdk PIG_M6_ARTIFACTS=/captures go test ./internal/m6gate -run Workflow -count=1 -v`：独立安装 CLI 与外部 module 编译 SDK；候选阶段允许明确的本地 replace，正式版本必须再用发布脚本验证无 replace 的公开安装。
- `PIG_BINARY=/installed/pig make m4-html-browser`：浏览器渲染、主题及 XSS 回归。
- `make m6-manual` 缺记录必定失败；发布脚本拒绝脏 checkout；人工记录校验的负例覆盖旧 commit、缺项、pending 和修改后的工件。

API snapshots、Catalog、证据文件的变动必须经过审计后更新 `internal/m6gate/testdata/catalog_scope.txt`。该快照覆盖 1,761 个 M6 条目（10 implemented、277 partial、841 scaffolded、633 inventoried），不把这些数量当作完整性证明。保持所有旧条目的能力状态；新增组合 contract 为 partial。

2026-09-24 工作区候选实测结果：

| 检查 | 结果 |
| --- | --- |
| `make m6-gate` | PASS；全仓普通/race 测试、vet、无 CGO 构建、M1–M5 回归与示例全部通过。M6 生命周期 20 轮通过（408s）；CLI/SDK 终端组合及相关回归 3 轮通过（343s / 319s）。 |
| `make m6-oracle` | PASS；固定 Pi commit 的所有切片与普通/全屏组合均一致。使用 Node 24.4.1（Unicode 16）。 |
| `make m0-source-drift` | PASS；1,457 个符号、7,429 个成员、30 个静态数据项及 110 个构造项复核通过。 |
| 独立安装 CLI 与外部 module SDK | PASS；无 CGO 构建，普通/全屏回放均通过。SDK 使用明确的本地 replace，不作为公开版本安装证据。 |
| 已安装 CLI 的 HTML 浏览器检查 | PASS；固定 Pi 渲染、主题、导航、离线资源与 XSS 回归通过。 |
| 发布前置条件负例 | PASS；缺人工记录及脏 checkout 均被拒绝；旧 commit、缺项、pending、缺失或被修改的工件校验通过。 |
| 人工真实终端验收 | 按用户 2026-09-24 的最新决定暂时跳过；未执行，非通过。 |
| 完整 `m6-freeze`、live smoke、tag/Release、公开版本安装 | 未执行；仍待完成。 |

本机日志保留于 `/tmp/pig125-gate-final.log`、`/tmp/pig125-all-oracle.log`、`/tmp/pig125-source-drift.log`、`/tmp/pig125-installed/install.log` 和 `/tmp/pig125-html-browser.log`；安装回放工件位于 `/tmp/pig125-installed/terminal-artifacts`。临时日志不替代发布时绑定确切 commit 的归档证据。

用户最新决定为“人工验收先跳过，可以继续更新issue状态，包括父issue”。已同步 #125 和父 #8 的自动验收清单及本轮人工验收跳过状态；[人工步骤](../learning/m6-manual-acceptance.md) 保留。完整 freeze（含受保护 live smoke）、tag/Release 与公开版本安装仍待完成，两张 issue 保持开放；#99/M7/M11/M12/M13 边界不变。本次仅更新验收进度，不改写人工证据或发布脚本的检查结果。

后续授权：用户要求“继续未完成项，直到关闭两个issue”。v0.6.0 允许上述人工验收豁免，其余冻结与公开安装要求不变。最终冻结 commit、日志、发布校验和及公开安装证明以 [Release 附件](https://github.com/nankedr/pig/releases/tag/v0.6.0) 为准；以上表格保留发布前工作区验收历史。
