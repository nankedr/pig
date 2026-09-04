# Pig 领域词汇

Pig 是 Pi 的 Go 语言移植项目。完整性以固定上游快照、明确的兼容面和可复核证据判断，而不是以代码相似度判断。

## Language

**Pi**:
定义 Pig 目标能力与语义的上游参考产品。
_Avoid_: 原项目、TypeScript 版本

**Pig**:
依据 Pi 验证能力和语义的 Go 产品与 SDK。
_Avoid_: 精简版 Pi、仿 Pi

**对等基线（Parity Baseline）**:
一次 Pig 发布用于验收的不可变 Pi 源码快照及其配套数据快照。
_Avoid_: 最新 Pi、当前上游

**对等（Parity）**:
在约定兼容面、能力范围和设计映射上，由证据证明 Pig 与对等基线等价。
_Avoid_: 大致相似、功能借鉴

**兼容面（Compatibility Surface）**:
Pig 与 Pi 之间需要保持的可观察契约，包括命令行为、配置、持久化数据、协议和运行结果。
_Avoid_: 源码 API 完全相同

**语义兼容（Semantic Compatibility）**:
Pig 与 Pi 能以等价含义和行为生产、交换或消费数据，不要求序列化字节完全一致。
_Avoid_: 字节完全一致、宽松近似解析

**可运行里程碑（Runnable Milestone）**:
已实现能力能够端到端运行，尚未支持的路径会明确失败的一次迭代交付。
_Avoid_: 仅能编译的 Stub、伪成功演示

**核心模块（Core Module）**:
Pig V1 主要完整复刻的 `ai`、`agent`、`coding-agent` 模块。
_Avoid_: 目标包

**支撑模块（Supporting Module）**:
因核心模块依赖其契约和行为而完整移植的 `telemetry`、`tui`、`protocol`、`client` 模块。
_Avoid_: 薄适配器、部分 Shim

**扩展面（Extension Surface）**:
扩展可观察或拦截 Agent，并增加 Tool、命令、Provider、资源或 UI 的能力全集；其语言绑定、加载方式和 ABI 尚未确定。
_Avoid_: JavaScript 扩展 ABI、Go Plugin API

**冻结门禁（Freeze Gate）**:
一个子系统首次被可运行切片充分覆盖并稳定其接口的验收点；后续不兼容修改需要新的决策和迁移。
_Avoid_: 首次编译、全局一次冻结

**能力 Stub（Capability Stub）**:
已登记目标边界但尚不可用的能力映射；调用时返回结构化未实现错误，不产生网络、持久化或成功事件。
_Avoid_: 占位成功、隐式忽略

**能力矩阵（Capability Matrix）**:
按字段、选项和兼容分支列出某一里程碑实现范围与显式 Stub 的机器可验收清单。
_Avoid_: 高级功能、以后再补

**Provider**:
拥有模型目录、认证、默认端点，并将一个或多个 API 分派给相应适配器的命名模型来源。
_Avoid_: API、HTTP Client

**API 适配器（API Adapter）**:
实现一种模型服务线协议的复用模块，例如 OpenAI Chat Completions 或 Anthropic Messages。
_Avoid_: Provider、模型目录

**Agent 消息（Agent Message）**:
Agent 历史中的一项，既可为模型消息，也可为应用自定义消息；只有显式上下文转换才能把它变为模型输入。
_Avoid_: LLM 消息

**流结果（Stream Outcome）**:
Assistant 或 Agent 事件流累积出的最终值；生成以错误或取消结束时仍保留已有部分内容。
_Avoid_: Go error、最后一个事件

**延迟响应句柄（Deferred Handle）**:
标识 Provider 已接受的延迟响应，供调用者后续查询结果或取消该响应。
_Avoid_: Stream Outcome、最终回复

**Pi Oracle**:
按需运行的固定对等基线，用于生成 fixture 并比较 Pi 与 Pig 行为。
_Avoid_: Pig 运行时依赖、跟随上游移动的 checkout

**对等目录（Parity Catalog）**:
唯一权威的机器可读清单与证据库，登记每个范围内 Pi 制品、Pig 映射、能力状态、偏离和验证证据。
_Avoid_: Parity Ledger、人工 Checklist、生成报告

**目录快照（Catalog Snapshot）**:
与源码 commit 配套锁定的不可变模型目录数据，使对等基线可复现。
_Avoid_: 在线模型目录、生成的 Go 源码

**能力状态（Capability Status）**:
一个对等目录条目的证据化生命周期：`inventoried`、`scaffolded`、`partial`、`implemented`、`verified` 或显式 `deferred`。
_Avoid_: 完成百分比、单一 done 标记

**Headless Coding Agent**:
无需 TUI、以文本或 JSON 事件运行的 Coding Agent 产品路径。
_Avoid_: Agent loop、测试 Harness

**里程碑前沿（Milestone Frontier）**:
主学习路径上当前唯一正在追求验收的下一个可运行里程碑。
_Avoid_: 所有并行工作流、普通 Backlog

**线标识（Wire Identifier）**:
参与互操作的序列化判别值或标识；即使包含 Pi 字样也可能必须原样保留。
_Avoid_: Go 类型名、用户可见品牌名

**项目信任（Project Trust）**:
是否加载并执行项目提供的设置、资源、包和扩展；Context File 是例外。它不是 Tool 审批、路径边界或 Sandbox。
_Avoid_: 命令审批、安全工作区

**上下文文件（Context File）**:
即使未授予项目信任也会进入模型上下文的仓库指令，例如 `AGENTS.md` 或 `CLAUDE.md`。
_Avoid_: 可信资源、可执行扩展

**离线模式（Offline Mode）**:
一致关闭全部可选网络路径的进程级状态。
_Avoid_: 部分离线、断开 Provider 调用

**对等用例（Parity Case）**:
从 Pi Oracle 得到的确定性输入和预期语义结果，用来验证一项 Pig 行为。
_Avoid_: 在线 Provider 冒烟、实现单元测试

**Coding Agent JSONL RPC**:
`pig` 进程通过标准输入输出收发 JSONL 命令、响应与会话事件的产品模式。
_Avoid_: Remote Session Protocol

**远程会话协议（Remote Session Protocol）**:
`protocol` 与 `client` 模块使用 CBOR framing、快照和 lease 与远程 service 交互的协议。
_Avoid_: Coding Agent JSONL RPC、Pig Server
