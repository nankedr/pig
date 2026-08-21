# Pig 文档导航

Pig 文档按“术语与范围 -> 决策 -> 设计与规范 -> 路线图 -> 学习与证据”组织。阅读代码或设计任务前，先确认固定 Parity Baseline 和当前 Milestone Frontier。

## 首次阅读

1. [领域术语与范围](../CONTEXT.md)：统一 Pi、Pig、Parity、Compatibility Surface、Parity Catalog、Capability Stub 等术语。
2. [总体架构](design/architecture.md)：七个包、单一 Go module、依赖方向、双 Agent 和接口冻结规则。
3. [兼容性设计](design/compatibility.md)：语义兼容、Wire Identifier、兼容怪癖和批准偏离。
4. [V1 路线图](roadmap.md)：M0 至 M14 的 Milestone Frontier 与共同门禁。

## 规范

- [对等验证规范](specs/parity-verification.md)：Parity Catalog 唯一权威、Pi Oracle、Parity Case、测试层次和 Chat Completions 逐字段 matrix。
- [运行时契约](specs/runtime-contracts.md)：消息、EventStream、Agent turn、Tool、Session、Client 和 Telemetry 的可观察语义。
- [CLI、存储与平台契约](specs/cli-storage-and-platform.md)：命令模式、RPC/Remote 区分、路径、Session、Shell 和平台边界。
- [安全与网络契约](specs/security-and-network.md)：Project Trust、无 sandbox 的宿主模型、凭证、Offline 和外联边界。
- [模型目录规范](specs/model-catalog.md)：Catalog Snapshot 缺口、获取与校验、运行时 overlay 和生成管线。
- [扩展系统规范](specs/extensions.md)：Extension Surface、早期 Stub、M5 package 边界和 M7 ABI 决策门禁。

## 决策记录

[ADR 目录](adr/)保存已经接受、难以逆转的取舍。设计文档和规范负责汇总当前有效决策，但不能静默覆盖 ADR。遇到冲突时，应先检查 ADR 的 `status`，再通过新的决策明确 supersede 关系。

常用主题：

- 基线与证据：ADR-0001。
- 语义兼容、Go API 与运行时：ADR-0002、0006、0007。
- 模块范围、Go module 与冻结门禁：ADR-0003、0004、0005。
- Pig 身份、状态、服务和宿主安全：ADR-0008、0010。
- 扩展运行时延期：ADR-0009。
- 平台与发布目标：ADR-0011。
- 许可证与上游归属：ADR-0012。
- Provider refresh 的 generation commit 与同步重入边界：ADR-0013。

## 信息的权威归属

| 信息 | 权威位置 |
| --- | --- |
| 领域术语和范围定义 | `CONTEXT.md` |
| 难以逆转的设计决策 | `docs/adr/` |
| 里程碑顺序和门禁 | `docs/roadmap.md` |
| 文件、symbol、字段、状态、偏离和证据 | Parity Catalog |
| 架构与 Compatibility Surface 的当前汇总 | `docs/design/` |
| 可执行验收规则 | `docs/specs/` |
| 任务拆分、依赖和进度协调 | GitHub Issues |

Markdown 报告、Issue 勾选、测试数量或文档中的阶段描述都不能替代 Parity Catalog 的 Capability Status。

## 学习与源码导航

后续里程碑按能力所属阶段建立两类材料：

- `docs/learning/`：Agent 原理、执行链、状态机、失败场景和实验；
- `docs/mappings/typescript-to-go/`：Pi 文件、symbol、测试到 Pig 实现的导航。

学习材料解释“为什么”和“如何运行”，mapping 帮助对照源码；二者都不记录权威完成度。每个 Runnable Milestone 必须同步代码、Go SDK、示例、Parity Catalog、学习材料和 mapping。

固定快照中的 13 个 SDK 示例按 Capability 所属阶段迁移为可编译、可运行的 Go 示例，并与对应 CLI/SDK 同步验收。Extension example 等待 M7 完成运行时与 ABI 决策后再迁移；在此之前只进入 Parity Catalog。Pi changelog 仅用于 Pi Oracle、行为发现和来源追踪，不作为 Pig changelog 或 Pig 发布历史。

Telemetry 文档需要区分两个概念：`telemetry` Supporting Module 是默认 NOOP、无 exporter/endpoint 的显式诊断契约；install telemetry 是 `codingagent` 产品层的可选外联面，默认不得连接 Pi 运营服务。

开始实现一个 Capability 前，应先阅读对应 Pi 实现与测试，建立失败的 Parity Case，再编写 Go 实现；不能从文档摘要反推或猜测上游行为。
