# Pig V1/V2 路线图

依据 [ADR-0043](adr/0043-versioned-delivery-and-gates.md)，最终完整复刻固定 Pi 基线，V1 交付完整语义的明确子集，V2 关闭剩余兼容缺口。Code Baseline 为 `936aff00918de1187f085f123c2812d8f2d67745`；Catalog Baseline 为 v0.84.1 source tar（`53fa77ccd8a279eb87e92294ef3687b03ff80112`，39 个 Provider、1220 个 chat model）。两者相差 40 个 commit，仍按 ADR-0014 解释双来源。

当前 Milestone Frontier：**V1 release**。#130、#12、#14 均已关闭，#16 接通 V1 冻结、安装与发布；M0–M6（#2–#8）全部已关闭，v0.6.0 是已交付文本 TUI 的历史版本。新范围归属见 [Catalog 版本范围](../parity/delivery-scope.json)，状态和证据仍从 Catalog 读取。

| 版本与顺序 | Issue | 可验收产物 |
| --- | --- | --- |
| V1 范围同步 | #130 | ADR、规范、Catalog 归属与验收规则一致 |
| V1 Responses | #12，#131–#133 | DeepSeek 文本流与终态、工具 continuation、reasoning/历史重放、可恢复 SDK/headless/TUI/RPC；保留 Chat Completions |
| V1 图片输入 | #14，#134–#136 | 用户/工具图片、处理与上下文、持久化恢复、附件交互、终端预览及导出 |
| V1 收口 / v1.0.0 | #16 | V1 范围完整闭环、全部已交付能力回归、V2 Stub 边界、darwin-arm64 发布与受保护文本/工具/视觉 smoke |
| V2 OpenAI 与目录 | #129 | OpenAI 官方/Azure/Codex、剩余协议和 Chat Completions compat、高级 options、完整目录生成/校验/overlay/cache |
| V2 Provider 与认证 | #13 | 其余厂商、API key/OAuth/ambient auth、pig-ai 与兼容矩阵 |
| V2 图片剩余路径 | #128 | 图片生成/编辑、image model、剩余多模态兼容 |
| V2 Harness | #10 | 固定基线 v4 底座及相同未实现操作，承担自身图片集成 |
| V2 Remote | #11 | Protocol/Client、CBOR/framing、lease/snapshot、RemoteSession 及图片集成；不实现 Pig Server |
| V2 包生态 | #99 | manifest、本地资源包、npm/git、依赖及 lifecycle |
| V2 平台 | #15 | macOS/Linux/Windows amd64/arm64 与 Termux 的完整发布闭环 |
| V2 最后功能阶段 | #9 | 扩展运行时研究/grilling/ADR 先于 ABI；完整 Extension Surface，含 #96 转入的 RPC 分支摘要导航和图片集成 |
| V2 收口 / v2.0.0 | #127 | 关闭固定基线全量 Catalog 缺口与全链路集成，不新增功能 |

V2 以 #16 为版本排期门，内部依真实依赖拆分，不沿用 M7→M14 串行依赖或旧 v0.7.0–v0.13.0 绑定。共享图片先交付，各模块负责自己的后续集成，避免“扩展最后”与图片完整性互相阻塞。

## 当前冻结与发布规则

[V1/V2 验收规范](specs/versioned-release.md) 是当前发布规则。V1 只承诺 DeepSeek 和 darwin-arm64；协议适配、可编译或图片输入成功均不等于其他厂商、平台或图片生成已完成。V2 缺口继续保留原语义和未完成状态，不能隐式阻塞 V1，也不能计入已完成。

新能力必须同步 CLI、SDK、示例、中文学习资料、TypeScript → Go 导航、API snapshot 和 Catalog 证据。改变范围、顺序或冻结接口须另行决策；两版都不得降低错误、取消、并发、顺序、恢复和验证要求。

## M0–M6 历史门禁与复现入口

以下是已交付阶段的复现说明。旧 M10/M12 等编号只表示历史范围映射，当前交付归属以 ADR-0043 为准；旧“父 Issue 另行推进”等备注是当时状态，不是当前前沿。历史 release、验证记录和绑定版本的豁免不重写，也不自动迁移为 V1/V2 新验收结论。

M0 的普通集成入口是纯离线的 `make m0-gate`，只重放已提交 fixture，不读取 Pi checkout。完整冻结使用 `make m0-freeze PIG_PI_ORACLE_CHECKOUT=/path/to/prepared/pi PIG_PI_SOURCE_CHECKOUT=/path/to/pristine/pi`；前者预装依赖并构建 `dist`，后者必须没有 tracked、untracked 或 ignored 状态，不能互相复用。准备命令见 [M0 兼容骨架](learning/m0-compatibility-skeleton.md#冻结门禁)。

## M1 冻结门禁

M1 必须在冻结首批接口前完整验证本阶段触达的核心契约：

- faux Provider 覆盖文本、Tool、取消、Provider 错误和最终 Stream Outcome。
- 本地 OpenAI Chat Completions 假服务覆盖请求、SSE 分片、partial JSON、可重试与不可重试错误、取消和超时。
- Tool 参数严格经过 `raw JSON -> prepareArguments -> coercion/validation -> typed decode -> Execute`。
- 文本到 Tool、ToolResult、继续生成的完整闭环可运行；并行/串行顺序、listener barrier 与 Tool update barrier 有确定性测试。
- DeepSeek 真实冒烟覆盖基础流式回复和一次 Tool continuation；普通 PR 不需要真实密钥。普通 Go 测试缺 `DEEPSEEK_API_KEY` 时明确 skip；`make m1-live-smoke` 强制要求密钥；`make m1-freeze`（含 `PIG_REQUIRE_LIVE=1`）缺密钥时必须失败。
- `read` 对真实文本文件完整可用；另有仅用于确定性测试的 Tool。
- 内存 AgentSession 的 text/json 两种 Headless 路径端到端可运行，其他未实现路径返回结构化 `ErrNotImplemented`。
- Pi Oracle 语义差分、`go test -race` 和本机 darwin-arm64 编译通过；不包含 browser、WebAssembly 或其他平台门禁。
- 对应 Go SDK、可运行示例、API snapshot、Parity Catalog 和学习文档同步完成。

M2 只扩展 M1 未触达的 legacy 分支，不重定义上述已冻结契约。M10 补齐不依赖图片体系的 Chat Completions Provider 兼容标志、高级选项和剩余协议分支，M12 再关闭图片相关分支。

## M2.1 thinking/signature 门禁

Issue #60 完成 M2 的第一条可运行切片：

- Faux 离线流发送 thinking start/delta/end，并在 done、取消和 Provider error 终态保留 partial thinking、signature 与 redacted 元数据。
- OpenAI Chat Completions 覆盖 reasoning level/effort、thinking budget、九种 M2 thinking format、同模型 history replay 与跨模型 thinking-as-text 转换。
- SSE 覆盖 `reasoning_content`、`reasoning`、`reasoning_text` 优先级，encrypted reasoning detail 的前置/后置 ToolCall 绑定，reasoning usage 与按 content 顺序收尾。
- 固定 Pi Oracle `openai-completions-thinking.json` 与 Go parity test 对齐请求、转换、事件和终态；API snapshot 冻结首次执行的公开字段和入口。
- `go run ./examples/thinking-signatures` 提供不需要网络与凭证的 SDK 示例；学习文档和 TypeScript→Go 导航记录边界。

未在 fixture 中穷举全部 absent/null/zero 状态的矩阵行只提升为 `partial`。deferred handles 由 M2.3 切片实现；post-M2 thinking formats 仍保持精确 `ErrNotImplemented`。

## M2.2 usage/cost/cache 门禁

Issue #61 完成统一核算切片：

- OpenAI Chat Completions 规范化 input、output、reasoning、cache read/write、total 和 cost，Go 公开契约保留 reasoning 缺失与显式零。
- `CalculateCost` 覆盖基础费率、按总输入量选择的最高匹配 tier，以及一小时 cache write 使用当前 input rate 两倍的规则。
- Faux 按 session 确定性模拟 prompt cache；跨 session 隔离、禁用 cache、取消与失败均有明确边界。
- 固定 Pi Oracle `usage-cost-cache.json` 与公开 Go 重放覆盖 Stream、Result、Complete 和成本计算；`go run ./examples/usage-cost-cache` 提供离线 SDK 示例。

Pi 的 OpenAI mapper 会把缺失 reasoning 计数折叠为零；Pig 通过 `Optional[int64]` 保留 absent/zero，Catalog 将这项额外 Go 契约证据与固定 Pi 对等证据分开记录。

M0 的 Chat Completions 能力矩阵必须区分公开 API 与内部 wire 字段，并逐字段、逐 option、逐 compat flag 指明归属 M1、M2、M10 或 M12。M1 范围外的未实现项均为可调用但明确失败的 Capability Stub；固定快照明确规定为 ignore/no-op 的 option 则实现并验证该行为，不能把两者混淆。M2/M10/M12 依次关闭已登记缺口，不改变 M1 已冻结的公共类型与核心语义。

## M2.3 deferred response 门禁

Issue #62 通过公开 Faux、Provider 和 Models API 完成提交、pending/final 轮询与取消：

- Deferred Handle 保留身份与显式零轮询提示；重复读取共享稳定 final 或脚本错误。
- 保留认证、请求转换、telemetry context、response hook、请求取消与 Stream Outcome 语义。
- 并发 fetch/cancel、提交快照和取消终态顺序通过公开 SDK 与 race 测试；严格 handle 校验等 Pi 差异见 ADR-0015。
- 固定 Pi `deferred-lifecycle.json` 与 Go 重放验证顺序生命周期，离线示例为 `go run ./examples/deferred-response`。

真实网络适配器的 deferred 支持继续按各自里程碑实现，完整范围见 [M2.3 学习文档](learning/m2-deferred-response.md)。

## M2.4 deferred tools 门禁

- `ai.SplitDeferredTools` 按规范化名称去重当前 Tool 集，保留最后定义与首次出现顺序，根据 transcript 标记和调用记录拆分。
- 关闭时全部 immediate；开启时仅未调用的动态 Tool deferred，后续调用恢复 immediate 的基线差异见 ADR-0016。
- 固定 Pi fixture、公开 Agent/Faux continuation、API snapshot 和 `go run ./examples/deferred-tools` 覆盖完整 SDK 路径。

具体 API Adapter 的 deferred-tool wire 方言继续留给 M10，见 [M2.4 学习文档](learning/m2-deferred-tools.md)。

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

## M2.5 Telemetry 门禁

Issue #64 实现内存 span 生命周期、独立快照、并发父子关系和被动记录；全部 9 个 adapter conformance case 可执行。固定 Pi fixture、公开 SDK 示例、race、Catalog 和 API snapshot 覆盖该切片，默认 NOOP，无 exporter 或全局当前 span。见 [M2.5 学习文档](learning/m2-telemetry.md)。

## M2.6 compat 与 Session Resource 门禁

Issue #65 让 compat 与全部 deprecated aliases 复用同一注册表，验证 source 所有权、覆盖顺序、builtin 恢复与并发 reset；Faux 构造、注册、队列和注销与直接 Provider 观察一致。Session Resource 按注册快照顺序清理，失败汇总并继续执行，重复调用与并发注册均有测试。固定 Pi 共同 fixture、独立偏离 fixture、API snapshot、离线示例和 Catalog 同步；协议、ambient auth 与图片继续按 M10/M11/M12 推进。见 [M2.6 学习文档](learning/m2-compat-session-resources.md)与 [ADR-0017](adr/0017-compat-registry-and-resource-cleanup.md)。

## M2 冻结门禁

`make m2-gate` 提供全部已交付链路的离线回归、race、vet、darwin/arm64 无 CGO 构建、示例与 20 次随机顺序并发验证。`make m2-freeze` 必须从干净 Pig checkout 运行，再校验固定 Pi Oracle、source drift 和要求真实凭证的 M1 DeepSeek 冒烟。M2 Catalog ID、执行证据、明确的 partial 范围与 CLI/SDK 版本由 `internal/m2gate` 检查。

详细范围与发布顺序见 [M2 集成与冻结](learning/m2-freeze.md)。本票只收口 #60–#69，不改变后续 Adapter、认证、图片和 broad contract 的未实现边界。

## M3 集成验收

`make m3-gate` 继承 M1/M2 离线回归并重复验证 M3 的跨进程持久化、锁、四工具恢复/fork 与取消。`make m3-freeze` 在干净 checkout 上追加固定 Oracle、source/API drift、v3 双向互操作和受保护 DeepSeek 冒烟。Catalog 静态映射与已交付行为分别验收，后续里程碑缺口继续保持可见。发布范围及制品见 [M3 集成与冻结](learning/m3-freeze.md)和 [v0.3.0](releases/v0.3.0.md)。父 Issue 和里程碑前沿由阶段维护流程另行推进。

## M4 集成验收

`make m4-gate` 集成七工具真实进程恢复、SDK 编排与并发 RPC；`make m4-freeze` 追加固定 Oracle/source drift、浏览器 XSS 与受保护 DeepSeek 冒烟。逐项范围与安装制品见 [M4 冻结](learning/m4-freeze.md)和 [v0.4.0](releases/v0.4.0.md)。RPC 扩展上下文的分支摘要导航仍归 M7/#9；本票不推进父 Issue 或 Milestone Frontier。
