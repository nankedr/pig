# 对等验证规范

## 目的

本规范定义 Pig 如何证明自己与固定版本 Pi 对等。实现数量、测试通过率或人工印象都不能单独证明完成；每个结论必须能追溯到固定 Parity Baseline、Parity Catalog 和验证证据。

## 固定基线

Pig V1 的 Parity Baseline 由两部分共同组成：

1. Pi 源码 commit `936aff00918de1187f085f123c2812d8f2d67745`；
2. 与该源码配套的不可变 Catalog Snapshot。

仓库提交 upstream lock，记录仓库 URL、commit、许可证和校验信息。Pi Oracle 按需检出到被忽略的 `.upstream/pi`，或验证用户提供的现有 checkout 恰好位于该 commit。Pi 不是 submodule、复制进来的源码树或 Pig 运行时依赖。

## Parity Catalog 是唯一权威

Parity Catalog 是范围、映射、状态、偏离和证据的唯一机器可读权威来源。路线图 Issue 管理工作，ADR 记录决策，学习文档解释原理，生成报告便于阅读；它们都不能单独改变 Capability Status。

Catalog 必须覆盖：

- 七个模块的每个 in-scope 源文件；
- 技术可达 export、公开成员和目标 Go 映射；
- 私有 helper 的去向，但不强迫其变成 Go interface；
- 测试、test helper、fixture 和断言行为；
- 示例、CLI、文档体现的隐含契约；
- schema、协议字段、配置字段、模型字段和资产；
- 已知怪癖、批准偏离、许可与来源；
- 验证命令、fixture、golden、Oracle 输出或人工证据。

M0 使用锁定版本的 TypeScript Compiler API 提取文件、entry、export、member 和 test，生成版本化 manifest 与规范化 JSONL Catalog。Go 工具校验重复项、遗漏、非法状态、映射和证据，并生成非权威 Markdown 报告。普通 Pig build/test 不依赖 Node；只有基线或提取规则变化时才重跑提取器。

M0 Freeze Gate 前建立以下 canonical artifact；版本号写入 schema 与 manifest，不靠文件名猜测：

```text
parity/catalog.jsonl
parity/catalog.schema.json
parity/catalog.manifest.json
parity/reports/                 # 非权威生成视图
```

Catalog 的条目类型必须能直接表达 CLI command/flag/exit/signal、RPC union、Remote protocol version/schema、v3/v4 行结构与恢复、settings/auth/models 格式和迁移、资源搜索与冲突优先级。规范文档只链接或解释这些项，不维护第二份手工状态表。

## Capability Status

状态按以下路径推进：

```text
inventoried -> scaffolded -> partial -> implemented -> verified
                   \-> deferred
```

- `inventoried`：已发现并记录上游来源、范围和目标里程碑。
- `scaffolded`：Go 契约或 Capability Stub 已存在并可编译。
- `partial`：只支持部分行为，必须同时列明已支持和未支持分支。
- `implemented`：目标行为已完成，但尚未绑定足够的对等证据。
- `verified`：已绑定可重复证据并通过当前门禁。
- `deferred`：仅用于明确决策，必须指向 ADR 和目标阶段。

状态不能由源码扫描自动推断为完成，也不能只凭一个单元测试升级为 `verified`。V1 最终不允许无解释的 `partial` 或 `deferred`；固定快照明确未实现的 Harness 操作可以保留，并必须验证相同的未实现结果。

## Chat Completions 逐字段 matrix

M0 必须建立 OpenAI Chat Completions 的逐字段 matrix，并把它作为 Parity Catalog 记录或由 Catalog 生成的只读视图。不得维护第二份手工完成度表。

matrix 至少覆盖，并明确区分“上游公开 API”与“仅供 adapter wire/fixture 使用的内部字段”：

- 公开 Message、Content、Tool、Tool Choice、Stream Event、Usage、Error 和 Options 类型；
- request body、header、URL、认证、timeout、retry、signal 和 transport 相关字段；
- system/developer/user/assistant/tool 消息转换及图像、thinking/signature、tool call/result 分支；
- SSE event、delta、partial Tool JSON、finish/stop reason、usage 与 error 映射；
- `onPayload`、`onResponse`、自定义 fetch、header 删除和 payload 替换语义；
- Model compat 与 Provider compatibility flags；
- absent、null、false、零值、空数组和默认值的差异；
- Pi 来源符号/字段、Go 名称/类型、适用方向、目标阶段、Capability Status 和证据。

阶段责任固定如下：

| 阶段 | 要求 |
| --- | --- |
| M0 | 完整声明全部公开类型与 options；公开和内部 wire 字段分别进入 matrix；未实现分支显式 Stub，基线规定的 ignore/no-op 则验证其无操作语义 |
| M1 | 验证 DeepSeek/faux 所需核心 Chat Completions 请求、SSE、Tool、usage、retry、cancel 和 callback 路径 |
| M2 | 完成 thinking/signature 等跨 API 的通用 AI 能力及其 Chat Completions 分支 |
| M10 | 完成不依赖图片体系的 Provider compatibility flags、高级 options 与剩余 Stub，不破坏 M1 冻结接口 |
| M12 | 完成消息图片、工具结果图片及其 Chat Completions wire 分支 |

“字段存在于 Go struct”不等于已实现。传入尚未支持的非默认字段时必须返回可识别的 `ErrNotImplemented`，不能静默丢弃；但固定快照明确要求 Provider 忽略的 `transport`、metadata、samplingParams 等组合必须实现并验证相同的 ignore/no-op，而不能误报未实现。matrix 中只有绑定逐字段或组合行为证据后才能标记 `verified`。

## Pi Oracle

Pi Oracle 用于回答：相同输入进入固定版本 Pi 与 Pig 后，是否得到等价的值、事件、状态和副作用。

Oracle 流程：

1. 读取 upstream lock 并验证 commit；
2. 向 Pi 输入确定性 Parity Case；
3. 保存原始输出与必要运行元数据；
4. 向 Pig 输入同一 case；
5. 只归一化 case 明确声明的不稳定字段；
6. 比较语义值、事件顺序、错误、持久化和副作用；
7. 把 fixture、期望结果、hash 和命令绑定回 Catalog entry。

默认测试使用已提交 fixture，保持快速、离线且不要求 Node。专门的 differential job 才运行 Pi Oracle，用于重新生成或核验 fixture。真实 Provider 输出不稳定，不能作为 Oracle 期望值。

## 每项能力的实现循环

每个 Capability 都从失败的 Parity Case 开始：

```text
读取 Pi 实现和测试
        -> 提取确定性 Parity Case
        -> 先让等价 Go 测试失败
        -> 实现最小但完整的目标行为
        -> Pi/Pig 语义比较
        -> 更新 Catalog 证据
        -> 更新学习与 TypeScript 到 Go 导航
```

“最小”只指本次 case 所需的实现增量，不得把公开分支做成静默部分实现。上游每个 test specification 都必须映射到 Go test 或明确的等价证据，即使测试文件不逐个翻译。

## 测试层次

| 层次 | 用途 | 默认联网 |
| --- | --- | --- |
| Go unit/property | 算法、codec、状态机、边界与不变量 | 否 |
| fixture/golden | 快速重放固定 Pi 结果 | 否 |
| 本地假服务 | HTTP、SSE、重试、取消、header、partial JSON | 否 |
| faux Provider | Agent Loop、Tool、事件和失败注入 | 否 |
| Pi Oracle differential | 固定快照语义比较和 fixture 审计 | 否，首次获取 Oracle 除外 |
| conformance | Telemetry、Provider、Transport、Store 等可替换契约 | 否 |
| DeepSeek live smoke | M1 真实流式文本和一次 Tool continuation | 是，受保护密钥 |
| 原生平台/人工 TUI | 终端、信号、剪贴板、视觉和平台命令 | 按 case 声明 |

普通 PR 不使用真实 Provider 密钥。`DEEPSEEK_API_KEY` 只存在于本地或受保护 CI；M1 Freeze Gate 与 release 必须执行受限 token 的 live smoke。模型协议的确定性细节由本地假服务验证。

## 关键不变量

- EventStream 的事件序列和 Stream Outcome 分开比较。
- Provider cancel 必须保留 partial content，并结束为 aborted outcome。
- listener 按注册顺序等待；并行 Tool 的完成事件按完成顺序，transcript 结果按 call 顺序。
- Tool update barrier 停止接受新 update 后，先排空已接受 update，再发送最终事件；迟到 update 被忽略。
- partial Tool JSON 对完整参数的每个字节前缀进行差分。
- `raw JSON -> prepareArguments -> coercion/validation -> typed decode -> Execute` 每一层都有错误 case。
- 已公开的 Capability Stub 无网络、持久化或成功事件副作用。
- `go test -race` 覆盖已实现并发路径；不得以 JavaScript 共享引用为理由接受 data race。

## 证据与完成门禁

一条有效证据至少记录：baseline、case ID、输入/fixture hash、执行方式、期望语义、实际结果、适用平台和对应 Catalog entry。人工证据还要说明无法自动化的原因和复现步骤。

每个 Runnable Milestone 必须同时满足：

- 新能力端到端可运行，之前能力无回退；
- Catalog 状态、mapping、偏离和证据完整；
- 到达 Freeze Gate 的 API snapshot 已生成；
- CLI、Go SDK、示例和学习导航同步；
- 没有未声明的网络、密钥或平台依赖；
- `partial` 与 Capability Stub 准确描述，不冒充成功。

Catalog 仍有未覆盖 artifact、无证据的 `implemented`、无 ADR 的 `deferred` 或未解释 `partial` 时，不得宣布 V1 完成。
