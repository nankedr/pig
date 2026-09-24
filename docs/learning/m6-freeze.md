# M6 文本范围的集成与冻结

#125 汇总 #109–#124 的已交付文本切片。固定 Pi commit 为 `936aff00918de1187f085f123c2812d8f2d67745`，沿用 legacy AgentSession、v3 Session 和本地资源。**人工真实终端验收按用户 2026-09-24 的决定跳过，v0.6.0 使用显式人工验收豁免。** 用户授权在其余冻结、发布和公开安装验证完成后关闭 #125 与父 #8；最终结果见 Release 的确切 commit 及验证附件。

`python3 scripts/m6-audit.py` 逐条核对所有 M6 Catalog ID、状态、Go 映射、partial 边界和证据，另锁定上游文件/测试清单、符号/成员和 Go API snapshots。`internal/m6gate/testdata/catalog_scope.txt` 是生成的审计快照，Catalog 仍是唯一能力状态来源。inventoried/scaffolded 表示未完成，不能计为运行覆盖；partial 只证明 supported 分支。历史切片的 unsupported 描述不能覆盖后续独立 contract 的证据，审计时需同时阅读对应 contract。不按 issue 关闭数推断接口冻结。

```sh
make m6-gate
make m6-oracle PIG_PI_ORACLE_CHECKOUT=/prepared/pi
make m6-freeze \
  PIG_PI_ORACLE_CHECKOUT=/prepared/pi \
  PIG_PI_SOURCE_CHECKOUT=/pristine/pi \
  PIG_M6_MANUAL_EVIDENCE=/evidence/manual.json
```

`m6-gate` 继承 M1–M5 全量离线测试、race、vet、无 CGO darwin/arm64 构建、已有示例及重复回归；追加 20 次 SDK 取消/清理/迟到回调与 3 次 race 真实终端进程测试。需要预装 Go、Python 3、rg、fd 和本机 PTY/回环服务权限。普通门禁只重放锁定 fixture，不读 Pi、不访问真实模型。

`m6-oracle` 汇总所有 M6 SDK/CLI 提取器，包含此前没有纳入统一目标的编辑器、布局、对话框和组合场景。freeze 额外要求前后干净 checkout、人工证据或显式发布豁免、Unicode 16.0 的 Node、锁定 prepared/pristine Pi、source drift、浏览器 HTML 检查、受保护 DeepSeek 冒烟。缺少任一条件必须失败；环境准备见 [M0](m0-compatibility-skeleton.md) 和 [M5](m5-freeze.md)。

组合场景在普通/全屏同一 Session 内执行主题热更新保留草稿、设置保存、模型与 Session 选择器取消、排除上下文 Bash、多行粘贴和外部编辑器、统计、压缩成功/失败/取消、资源重载、模板/Skill 调用、HTML 导出错误/成功、维护后插话与 follow-up 排队、resize/翻页和继续生成，最后检查退出及 termios。长期滚动、协议、队列、信任与分支的细粒度语义继续由各切片 fixture 验证。组合只证明记录的动作与结果，不声称像素或所有键序列对等。

组合验收修复了多次维护通知遮挡全屏新回复的问题：通知按产生时的消息位置渲染，保留历史且不进入 Provider 上下文。真实 CLI 回归见 `TestPigFullscreenReplyAfterNotices125`。

公开 SDK 示例复用同一 Session 装配和 InteractiveMode，经相同受控服务/终端测试，无私有 helper：

```sh
go build -o /tmp/m6-sdk ./examples/m6-workflow
python3 parity/terminal/m6-workflow.py /tmp/m6-sdk --sdk
PIG_M6_ARTIFACTS=/tmp/m6-captures go test ./internal/m6gate -run Workflow -count=1 -v
```

工件包含 ANSI 终端输出、各阶段屏幕输出、Session 条目、设置、HTML 和语义 outcome；是自动化捕获，不是人工证据。自动证据与复现见 [#125 验收记录](../verification/issue125-m6.md)。真实终端步骤见 [人工验收](m6-manual-acceptance.md)。

确定发布候选时，先更新发布说明的候选状态并提交代码，再针对该 commit 完成人工验收，或按维护者明确授权提供豁免记录。随后运行 `python3 scripts/m6-release.py /tmp/pig-v0.6.0-release`（以上 freeze 环境变量需 export）。脚本冻结确切 commit，独立 module 编译 SDK、安装 CLI 并重跑交互组合，打包源码、darwin/arm64 无 CGO CLI、许可证、终端工件、人工记录或豁免、验证日志及 SHA256SUMS。全部通过后才能给同一 commit 建立 v0.6.0 tag 和 Release。再执行 `python3 scripts/m6-release.py --verify-published /tmp/pig-v0.6.0-release`，在新 module cache、无 replace 条件下验证公开版本及源码 Origin commit，上传新增安装证据。

剩余边界包括 Catalog 中所有 inventoried/scaffolded、partial unsupported、系统剪贴板、宿主分享、JSONL 导出、完整复杂 Markdown/布局/组件与罕见协议路径。包生态 #99 未排期、不阻塞本次文本切片；扩展 M7、完整认证 M11、图片 M12、六平台 M13 不标完成。导航见 [TypeScript → Go](../mappings/typescript-to-go/m6-freeze.md)。

本次人工豁免使用同一 `PIG_M6_MANUAL_EVIDENCE` 文件入口，记录 `status: "waived"`、`release: "v0.6.0"`、确切 `commit`、`platform: "darwin/arm64"`、`approved_by`、`approved_at`、`reason`、`authorization` 和空 `cases`。缺字段、旧 commit、其他版本或夹带已通过的人工用例均拒绝。日志输出 `WAIVED`，发布验证记录保留该状态；不得把豁免写成通过。
