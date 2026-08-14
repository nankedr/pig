# 运行时契约

## 1. 地位与兼容基线

本文是 `ai`、`agent`、`codingagent` 包及 `pig`、`pig-ai` 命令的规范性运行时契约。兼容基线固定为 Pi 提交 `936aff00918de1187f085f123c2812d8f2d67745`；上游当前分支、未来文档或设想中的 Harness API 不得覆盖该快照的实际代码语义。

Pig 追求功能和可观察语义的 1:1 兼容，而不是 TypeScript 实现或普通 JSON 字节的逐字复制：事件顺序、状态转换、默认值、错误分类和协议字段必须兼容；JSON 对象键顺序、等价转义和 Go 内部数据结构可以不同。协议明确规定的帧头、长度、换行和已发布 discriminator 属于字节或线格式契约，必须精确兼容。命令、目录和本地环境变量使用 Pig 身份；为与 Pi 对端互通而公开的 wire discriminator 保留 Pi 值。

每个里程碑都在同一组稳定分层和接口上补全实现。未到里程碑的能力必须以显式 stub 暴露，不能删除接口、伪造成功或偷偷换成缩减版协议。

## 2. 类型与序列化

### 2.1 消息和事件

- `ai.Message`、内容块、`AssistantMessageEvent` 和 `AgentEvent` 使用封闭的具名变体；JSON codec 按固定 discriminator 编解码，未知封闭变体报错。
- `agent.AgentMessage` 是扩展点。内置消息解成已知具体类型，未知消息解成 `RawAgentMessage` 并保留原始 JSON；只有显式 `ConvertToLLM` 才把 Agent 消息过滤或转换为模型消息。
- 流式事件中的快照不可变。实现可用单 owner accumulator 构造下一份快照，但已经发布给调用方的对象不得被后续 delta 修改。这是为消除 JavaScript 共享引用和竞态而接受的 Go 偏离，不改变内容与顺序语义。
- schema 的规范来源是原始 JSON Schema，不是 Go struct tag 的近似推断。对外可提供生成的 Go 类型和 helper，但验证必须遵循规范 schema。

### 2.2 absent、null、零值

只有指针或显式 `Optional[T]` 可以表达三态：字段缺失、字段显式为 `null`、字段有值。普通 Go 零值不得被解释成“未提供”。codec 必须在 round trip 中保留这三个状态。

合并语义按字段定义执行。典型的 header 覆盖规则是：缺失表示不覆盖默认值，`null` 表示删除默认 header，空字符串、`false`、`0` 和空集合是显式值。Hook 的返回也必须区分“保留旧值”和“用 null/零值替换”；公共形态统一为 `(value, replace, error)` 或等价的具名结果，不能用 `value == zero` 猜测是否替换。

## 3. EventStream、终态和取消

`EventStream[TEvent, TResult]` 是并发安全、无容量上限的 FIFO 事件流。核心操作为可取消的 `Next(ctx)` 和可重复调用的 `Result(ctx)`；它不向调用方承诺 channel 容量、生产者背压或丢弃策略。`Result` 在完成后每次返回同一最终结果。

Provider 失败和用户取消是模型流的业务终态：成功以 `done` event 携带最终 `AssistantMessage`，失败或取消以 `error` event 携带最终 `AssistantMessage`。取消保留已经生成的内容并设置 `StopReasonAborted`；Provider 失败使用 error stop reason 和 error message。普通 Go `error` 只用于等待方 context 结束、内部不变量破坏或协议损坏，不能代替正常 terminal event。

一次 Agent run 创建一个 `context.WithCancelCause` 子 context，并把它传给 Provider、工具、Hook 和被等待的 listener。所有派生工作共享这次 run 的取消原因；普通操作返回 `context.Cause`，不得把取消改写成无关错误。`agent_end` 是最后一个 loop event，但 run 只有在按序等待的 `agent_end` listener 全部返回后才 settled/idle。

## 4. Provider 与 API 注册

Provider 的 authoring API 保持强类型：模型、Provider 选项和 API adapter 可在编译期约束。异构注册表按 API ID 做类型擦除并保留运行时元数据；已知 API 解成对应 options，未知或扩展 API 使用动态 JSON options，不能因 Go 泛型无法装入异构集合而封闭扩展面。

M1 的真实链路是 DeepSeek Provider 复用 OpenAI Chat Completions API adapter，并同时提供确定性的 Faux Provider。`stream` 是事件真源，`Models.complete`、`completeSimple` 及相应 compat helper 消费该事件流得到相同最终消息；Pig 不虚构一个名为 `ChatCompletion` 的上游 helper。其余 Provider 先注册接口和元数据，调用未实现操作时返回本文第 8 节规定的错误。

## 5. Agent 事件和 turn 边界

无工具的典型 run 顺序为：

1. `agent_start`、`turn_start`；
2. 输入消息的 `message_start`、`message_end`；
3. Assistant 的 `message_start`、零到多个 `message_update`、`message_end`；
4. `turn_end`；
5. 无待处理工作时发布 `agent_end`。

含工具的 turn 在 Assistant `message_end` 后执行工具，随后为每个最终 ToolResult 发布 `message_start`、`message_end`，再发布 `turn_end`；需要自动跟进时开始下一个 `turn_start`。Agent 类必须把 Assistant `message_end` 的所有 listener 当成 barrier：它们完成、Agent 状态已包含发起调用的 Assistant 消息之后，才进入任何工具 preflight。

Listener 按注册顺序串行等待。它们不是 fire-and-forget 回调，也不能在 run 已报告 idle 后继续改变可见状态。

`shouldStopAfterTurn` 在 `turn_end` 发布且当前 Assistant/工具全部正常完成后运行；返回 true 时直接发布 `agent_end`，不再轮询 steering/follow-up queue，也不开始下一次 Provider request。它不取消已经结束的 Provider stream、不取消工具，也不修改 Assistant stop reason。

## 6. 工具契约

### 6.1 参数流水线

工具参数只允许这一条流水线：

```text
原始 JSON
  -> prepareArguments
  -> Pi 兼容的 coercion 与 JSON Schema validation
  -> typed decode
  -> Execute
```

`prepareArguments` 先于校验，可做工具专属正规化。coercion 必须复刻 Pi 对 primitive、object、array、`anyOf`/`oneOf` 等 schema 的行为；校验失败生成工具错误结果，不进入 `Execute`。Typed decode 只消费已经准备并验证的值，不能再施加一套不兼容规则。

固定快照中，coding-agent extension 的 `tool_call` 事件可以替换 `event.input`，替换后不会再次执行 schema validation；Pig 保留该可观察语义，并把 extension 视为受信任宿主代码。不得悄悄增加二次校验造成兼容差异。

### 6.2 partial JSON

每收到一个 `toolcall_delta` 都必须：

1. 把 delta 原样追加到该 content block 的 raw argument accumulator；
2. 对当前完整前缀做修复/容错解析；
3. 把当前能恢复出的 object 写入 partial AssistantMessage；
4. 立即发布包含该 partial 快照、`contentIndex` 和原始 delta 的事件。

不同 content block 的事件可以交错，消费者必须依赖 `contentIndex`，不能假设某个 block 的 start/delta/end 连续出现。结束时对完整 raw JSON 做严格解析，再走工具参数流水线。只为增量解析使用的 raw/scratch 字段必须在最终消息和持久化前移除。兼容测试要覆盖真实 Provider chunking，并对完整参数的每一个字节前缀做 differential test；不能只测试“刚好按字段切块”的样例。

### 6.3 preflight、并发和顺序

一个 Assistant 消息中的调用先按源码顺序完成全部 preflight。默认 `parallel` 模式对所有允许调用同时启动，不设置 worker 数或隐藏并发上限；任何 per-tool `executionMode: sequential` 或全局 sequential 配置都会让整个 batch 串行执行。

在默认模式下：

- `tool_execution_start`/preflight 保持 Assistant 源顺序；
- 工具效果可以重叠；
- `tool_execution_end` 按真实完成顺序发布；
- ToolResult 消息、transcript 和 `turn_end.toolResults` 按 Assistant 源顺序写入。

`beforeToolCall` 在参数准备和校验完成、`tool_execution_start` 之后运行，可阻止执行并直接给出结果。`afterToolCall` 在 Execute settle 后、最终 `tool_execution_end` 和 ToolResult 消息之前运行，可覆盖最终结果。

### 6.4 update/final barrier

工具 Execute settle 后立即关闭 update admission：此后到达的 update 被忽略。已经被接受的 update 必须全部按注册顺序 dispatch 并等待完成，然后才能发布最终 `tool_execution_end` 和 ToolResult；因此 final 永远不会越过已接受 update。实现必须能处理 update listener 与 Execute 完成并发发生的竞态。

工具、被阻止的 `beforeToolCall` 或 `afterToolCall` 覆盖结果都可设置 `terminate: true`。只有 batch 中每一个最终结果都为 `terminate: true` 时才停止自动 follow-up；混合 batch 继续。它只影响下一次模型调用，不撤销已经运行的工具。

## 7. Session、客户端请求与资源上限

生产 coding-agent 继续使用 `Agent + AgentSession + v3 JSONL`；Harness v4 是独立架构和独立格式，不能拿未来设计文档补齐固定快照中不存在的操作。M1 为内存 Session，不读写会话文件；M3 实现 v3；M8 再实现固定快照的 v4。

Pi 固定快照接受的 v3/v4 数据默认没有总文件大小、单行长度或 entry 数量上限，Pig 也不得引入隐式默认上限。实现可用 streaming/index 降低内存占用；若未来面向不可信输入增加限制，必须是显式、可配置且记录为新的安全策略，不能冒充兼容行为。

固定 Pi Client 没有 request cancellation API 或隐藏 timeout；Pig 的 Go API 仍不注入 timeout，但允许调用方 context 取消本地 waiter，并把 request ID 留作 tombstone，直到迟到 response 被消费或连接断开，防止 ID 重用和错配。这是已批准的 Go 偏离，不能描述成上游原行为；超时只能由调用方 context 或显式 option 提供。

Bash 返回给 UI/模型的输出可按 Pi 规则截断；一旦截断，完整 stdout/stderr 写入带 Pig 前缀的 OS 临时文件。运行时不隐式删除该文件，也不以 Session 大小策略删除它，生命周期交给用户或操作系统。

## 8. 未实现能力和错误

所有占位能力返回导出的 sentinel `ErrNotImplemented`，并包装结构化 `NotImplementedError{Module, Operation}`；`errors.Is(err, ErrNotImplemented)` 必须成立。CLI 把它映射为稳定、可识别的错误文本和非零退出码。

Stub 的调用不得发起网络、写文件、改变 Session、发布成功事件或返回看似有效的零值。接口存在不代表能力可用；能力探测必须能区分 implemented 和 stub。Provider 失败、模型拒绝、工具业务错误不是 `ErrNotImplemented`。

## 9. Telemetry 契约

Telemetry schema 在运行时以动态 schema 注册和验证，同时由同一规范生成 typed helper；两者必须通过 conformance test 证明字段名、类型、必填性和枚举一致，禁止维护两套手写真源。参考/测试 memory backend 是无容量上限的 append-only 收集器，不得静默丢样本；生产 exporter 的背压、批量和上限属于其显式配置。

默认事件必须 content-free、secret-free：不得包含 prompt、completion、工具参数/输出、文件内容、Provider payload/header 或凭证。只有未来显式启用且经过单独敏感数据策略的字段才能承载内容。

## 10. 里程碑约束

- M1：Faux、DeepSeek + OpenAI Chat Completions、`complete`、完整接口、完整 `read` 和确定性测试工具、内存 Session；`text`/`json` 可完整运行。
- M3：落地生产 v3 Session 语义。
- M4：本地 JSONL RPC。
- M7：才执行 extension；此前只发现/盘点入口并返回显式 stub。
- M8：固定快照的 Harness v4。
- M9：Remote Protocol Client；以固定 server package 的测试 host/受控 service 与 fake server 做互操作，V1 不提供 Pig Server。

安全、网络、CLI、存储和平台边界分别以 [security-and-network.md](security-and-network.md) 与 [cli-storage-and-platform.md](cli-storage-and-platform.md) 为准。
