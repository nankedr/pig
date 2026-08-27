# Pig V1 路线图

Pig V1 使用双来源对等基线：Code Baseline 是 Pi `936aff00918de1187f085f123c2812d8f2d67745`，Catalog Baseline 是 Pi v0.84.1 官方 source tar（commit `53fa77ccd8a279eb87e92294ef3687b03ff80112`，39 个 Provider、1220 个 chat model）。两者相差 40 个 commit，因此这不是 fixed-run parity；详见 ADR-0014。主学习路线一次只推进一个里程碑前沿；并行支线必须重新集成到持续可运行的 `pig`。

当前 Milestone Frontier：**M1**。

| 阶段 | 可验收产物 |
| --- | --- |
| M0 | 锁定源码与 Catalog Snapshot；生成 Parity Catalog；建立七个包、命令、公开契约、Capability Stub、Oracle 和基础测试骨架；声明完整 Chat Completions 类型/options 与分阶段能力矩阵 |
| M1 / v0.1.0 | 实现能力矩阵中的 faux + DeepSeek/OpenAI Chat Completions 核心路径；完整 EventStream、SSE、partial JSON、Provider retry、Tool 参数管线、并行/串行顺序与 listener/update barrier；内存 AgentSession；text/json；完整 read |
| M2 / v0.2.0 | 扩展 legacy AI/Agent 未触达分支：thinking/signature、deferred、跨 Provider 转换、usage/cost/overflow/cache、Agent steering/follow-up 队列、proxy、Telemetry、compat/deprecated API |
| M3 / v0.3.0 | 本地持久化 Coding Agent：v3 JSONL、恢复/fork、全局 settings、最小 Project Trust 后的项目 settings、基础 model/auth runtime、完整 read/bash/edit/write |
| M4 / v0.4.0 | Coding Agent Headless 编排：grep/find/ls、用户消息投递与 steer/follow-up 集成、整轮 Provider error retry、压缩、分支、树导航、stats、model/tool 切换、JSONL RPC 模式、HTML export |
| M5 / v0.5.0 | 资源与包系统：AGENTS、system prompt、templates、skills、themes、完整 trust/resource 判定、npm/git package 与 lifecycle；扩展 entry 仅发现并明确未实现 |
| M6 / v0.6.0 | 文本 TUI 与 Interactive 模式：布局、编辑器、按键、滚动、主题、对话框、session/model selectors |
| M7 / v0.7.0 | 扩展系统：先专项设计并冻结运行时/ABI，再让 M5 已发现的扩展 entry 与完整 Extension Surface 可运行 |
| M8 / v0.8.0 | 忠实复刻 AgentHarness v4：memory/JSONL、reducer、compaction substrate、工具及上游相同的未实现操作 |
| M9 / v0.9.0 | Protocol/Client：CBOR、framing、transport、lease、snapshot、RemoteSession；Pig Client 与固定 server package 测试 host/受控 service、fake server 互操作，不实现 Pig Server |
| M10 / v0.10.0 | OpenAI 系与 Catalog：Responses、Azure、Codex；补齐不依赖图片体系的 Chat Completions Provider compat flags、高级 options 与剩余协议分支；生成器、校验、remote cache/overlay |
| M11 / v0.11.0 | 其余 Chat API、40 个 Provider、API key、OAuth、ambient auth、`pig-ai` 登录 CLI 与完整模型兼容矩阵 |
| M12 / v0.12.0 | 图片体系：image model/generation、消息图片、工具结果图片、处理与终端显示 |
| M13 / v0.13.0 | 平台与发布闭环：六目标构建、原生行为、剪贴板、外部命令、资产、安装包、更新与第三方声明 |
| M14 / v1.0.0 | 只关闭对等缺口：全量 Parity Catalog、测试、示例、文档、许可和发布门禁，不新增功能 |

M0 的普通集成入口是纯离线的 `make m0-gate`，只重放已提交 fixture，不读取 Pi checkout。完整冻结使用 `make m0-freeze PIG_PI_ORACLE_CHECKOUT=/path/to/prepared/pi PIG_PI_SOURCE_CHECKOUT=/path/to/pristine/pi`；前者预装依赖并构建 `dist`，后者必须没有 tracked、untracked 或 ignored 状态，不能互相复用。准备命令见 [M0 兼容骨架](learning/m0-compatibility-skeleton.md#冻结门禁)。

## M1 冻结门禁

M1 必须在冻结首批接口前完整验证本阶段触达的核心契约：

- faux Provider 覆盖文本、Tool、取消、Provider 错误和最终 Stream Outcome。
- 本地 OpenAI Chat Completions 假服务覆盖请求、SSE 分片、partial JSON、可重试与不可重试错误、取消和超时。
- Tool 参数严格经过 `raw JSON -> prepareArguments -> coercion/validation -> typed decode -> Execute`。
- 文本到 Tool、ToolResult、继续生成的完整闭环可运行；并行/串行顺序、listener barrier 与 Tool update barrier 有确定性测试。
- DeepSeek 真实冒烟覆盖基础流式回复和一次 Tool continuation；普通 PR 不需要真实密钥。
- `read` 对真实文本文件完整可用；另有仅用于确定性测试的 Tool。
- 内存 AgentSession 的 text/json 两种 Headless 路径端到端可运行，其他未实现路径返回结构化 `ErrNotImplemented`。
- Pi Oracle 语义差分、`go test -race` 和本机 darwin-arm64 编译通过；不包含 browser、WebAssembly 或其他平台门禁。
- 对应 Go SDK、可运行示例、API snapshot、Parity Catalog 和学习文档同步完成。

M2 只扩展 M1 未触达的 legacy 分支，不重定义上述已冻结契约。M10 补齐不依赖图片体系的 Chat Completions Provider 兼容标志、高级选项和剩余协议分支，M12 再关闭图片相关分支。

M0 的 Chat Completions 能力矩阵必须区分公开 API 与内部 wire 字段，并逐字段、逐 option、逐 compat flag 指明归属 M1、M2、M10 或 M12。M1 范围外的未实现项均为可调用但明确失败的 Capability Stub；固定快照明确规定为 ignore/no-op 的 option 则实现并验证该行为，不能把两者混淆。M2/M10/M12 依次关闭已登记缺口，不改变 M1 已冻结的公共类型与核心语义。

## 状态与凭证边界

- Pig 默认只使用 `.pig`、`~/.pig` 和 `PIG_*`，不隐式读取 `.pi`、`PI_*` 或当前目录中的 Pi 文件。
- `pig-ai` 与 `pig` 共用默认 `~/.pig/agent/auth.json`、锁和 `0600` 文件权限；其他文件只能通过显式 `--auth-path` 使用。
- 上游 `pi-ai` 使用 `./auth.json` 的行为作为已接受偏离登记到 Parity Catalog，并建立独立 Parity Case。
- Pi 数据只在既有操作显式接收路径时交换，绝不自动迁移 trust 或凭证；通用 migration CLI 不属于当前承诺。

## 每个里程碑的共同门禁

- 新能力端到端可运行，既有能力无回退。
- Capability Status 与 Parity Catalog 已更新。
- Oracle、golden、conformance 或人工验证证据齐全。
- 到达 Freeze Gate 的接口已生成 API snapshot。
- 学习文档与 TypeScript 到 Go 导航已更新。
- 对应 CLI 能力、Go SDK 能力和所属示例同步完成。
- 未声明的联网、凭证和平台依赖均视为失败。

## 调整规则

可以拆分里程碑或子任务，但不能通过移出 V1、隐藏 Capability Stub 或降低验收标准来完成阶段。改变功能顺序、Freeze Gate 或公开承诺前必须重新决策。M7 必须先完成扩展运行时专项调查、grilling 与 ADR，再冻结扩展 ABI。

GitHub 以一个 V1 roadmap Issue 管理 M0～M14 子 Issue，并用原生 sub-issue 和 dependency 表达顺序。阶段内部只按可独立验收的子系统继续拆分；文件、符号和测试级进度仅进入 Parity Catalog。
