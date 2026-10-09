# V1 范围冻结、安装与发布

[#16](https://github.com/nankedr/pig/issues/16) 收口已完成的 #130、#12、#14，不新增功能。范围遵循 [ADR-0043](../adr/0043-versioned-delivery-and-gates.md) 和[版本规则](../specs/versioned-release.md)。

`parity/v1-acceptance.json` 按 retained、Responses、image-input、release 分支关联独立 Catalog 契约、最高层公开边界与 V2 跟踪票。`scripts/v1-audit.py` 锁定 134 个范围锚点、partial 边界及证据文件哈希；旧总条目不能覆盖后续独立契约，低层字段 scaffolded/inventoried 不代表没有公开图片运行证据。审计不会提升任何 Capability Status。新增证据/范围变化必须阅读差异后再生成审计快照。

```sh
make v1-gate
```

普通门禁清除所有 live 开关与凭证，继承完整 M6 回归、race、vet、无 CGO 构建、示例和重复生命周期检查，并追加 Responses/图片最高层进程 race 三轮、范围审计与合成 SDK 示例。需 Go 1.24、Python 3、rg/fd、本机 PTY/回环服务权限；无需 Pi checkout 或真实 Provider 凭证。

冻结需干净的确切候选 commit、原生 darwin-arm64、Unicode 16.0 的 Node（如 24.4.1）、锁定 prepared/pristine Pi、TypeScript 5.9.3、Playwright/Chromium 和受保护 `DEEPSEEK_API_KEY`。Pi 准备方式见 [M0](m0-compatibility-skeleton.md#冻结门禁)。若使用外部浏览器依赖，可设置 `PIG_PLAYWRIGHT_MODULE`（入口绝对路径）和 `PIG_CHROMIUM`（浏览器可执行文件）。

```sh
export PIG_PI_ORACLE_CHECKOUT=/prepared/pi
export PIG_PI_SOURCE_CHECKOUT=/pristine/pi
export PIG_V1_EVIDENCE_DIR=/tmp/pig-v1-live-evidence
make v1-freeze
```

`v1-freeze` 执行全量普通/race、历史重复回归、新能力组合 race、全部固定 Pi Oracle、图片 Oracle、source/API drift、浏览器 HTML/XSS、双 API 图片导出。随后 `scripts/v1-live.py` 依次强制执行 Chat Completions 工具、Responses 文本/工具/恢复、双 API 用户视觉/恢复、工具视觉、RPC 图片任务和原生剪贴板。每项保留 JSONL 日志，必须出现指定顶层 Test 的 run/pass，任何子项 skip/fail 或未运行均失败。原生剪贴板用合成数据并恢复原 pasteboard，PTY 验证 termios、取消/signal/外部进程和预览关闭。测试不是人工验收，不继承 M6 历史 waiver。

生成发布制品：

```sh
python3 scripts/v1-release.py /tmp/pig-v1-release
```

脚本重新执行冻结，记录成功 commit 后安装无 CGO CLI；在独立 module 中编译 SDK、运行 V1 合成图片工作流及普通/全屏 PTY，再用安装二进制检查图片入口/RPC/终端恢复、signal 和浏览器。输出二进制/源码归档、许可证、完整日志、终端工件、受保护 smoke 语义结果与 SHA256SUMS。日志只含合成输入与断言，不记录 secret/header。

全部通过后向同一 commit 建立 `v1.0.0` tag。公开安装验证必须在全新 module cache、无 replace 情况完成：

```sh
python3 scripts/v1-release.py --verify-published /tmp/pig-v1-release
```

公开 CLI/SDK 版本、Go module Origin 必须与冻结 commit 一致；复验 V1 SDK 图片/恢复、CLI/SDK PTY、安装后图片/RPC/signal 和浏览器，再上传 `published-install.json`、日志与校验和后正式发布。失败时保留诊断，不宣布完整 freeze，不关闭 #16。

[发布矩阵](../releases/v1.0.0.md)限定 DeepSeek 和 darwin-arm64；V2 尚未实现分支继续返回结构化 ErrNotImplemented，完整对等由 #127 验收。源码导航见 [TypeScript → Go](../mappings/typescript-to-go/v1-freeze.md)。
