# RPC 会话控制沿用 Session 状态边界

Issue #95 在固定 Pi `936aff00918de1187f085f123c2812d8f2d67745` 的 JSONL RPC 上接通文本 steer/follow_up、队列模式、模型与 thinking 的设置/轮换/查询、重试开关与取消、Bash 与取消、统计。`prompt` 仍复用 #94 的接纳响应与 streamingBehavior；最后回复沿用 #94/#88。

RPC 分派复用 AgentSession 的 admission、配置提交、重试和 Bash 生命周期。ADR-0018 规定 idle、结束或取消后的投递失败；ADR-0025 规定活动 Prompt 期间模型/thinking 修改返回 busy。这些既有 Go 偏离同样适用于 RPC，不模仿 Pi 的宽松入队或绕过 Session 写状态。无效队列模式/thinking 明确失败。切换模型先从可用目录查找 provider + modelId，再进行原有认证/适配器校验与持久化。进程的 Provider endpoint 覆盖作用于同 Provider 的每次请求，切换模型不会恢复到默认线上地址。这里只沿用已有 DeepSeek 运行时适配器。

线协议 `set_model` 返回完整 Model；Go 客户端保留已发布的 ModelInfo 投影。`cycle_model` 的 null 映射为 ModelCycleResult 零值（Model.ID 为空），`cycle_thinking_level` 的 null 映射为空 ThinkingLevel。统计的 `tokens.total` 映射到 SDK `Tokens.TotalTokens`；它只包含五个 token 计数，费用单独在顶层 cost。查询返回描述与统计，不解析或返回 Provider 凭证。

Bash 响应在执行完成并交给 Session 记录后发送；运行期间结果仍按 #93 先暂存，结束时归入历史。`excludeFromContext` 仅从模型输入排除，历史与统计仍保留。ExecuteBash 和 RPC 共用同一个私有执行入口；RPC 用输出回调保留任意 JSON id，并向其他 Session 订阅者发送原有 SDK 事件；只跳过 RPC 自己的转发订阅，避免重复线事件。公开 SDK 事件继续使用既有 *string ID，非字符串 wire id 在 SDK 事件中省略。客户端生成的字符串 id 因而可直接解码。并发 Bash 和 AbortBash 沿用 Session 的取消所有活动 Bash 语义。请求 Context 取消只释放本地等待；AbortBash/AbortRetry 才执行远端控制，AbortRetry 仅取消重试等待。

固定 Pi 没有 set_active_tools/get_active_tools 等 RPC wire，不能把 SDK Tool 方法扩造成自创命令。资源、扩展/UI、图片、会话替换/树/导出以及 compact/set_auto_compaction 的 RPC 控制仍明确失败。`RPCClient.Bash(ctx, command)` 的签名保持不变，excludeFromContext 只通过原始 wire 使用。RPCCommand.Data 仍不是通用的扁平化命令发送器。

证据由固定 Pi 源码的 runRpcMode 和公开 RpcClient 产生，真实 pig/RPCClient 验证投递、后续请求、错误、并发 ID、事件、Bash 上下文、重试取消与持久化。此次未宣称 dist 重建通过：固定基线的 build:offline 在 cloudflare-ai-gateway.ts 有既有 TypeScript 类型错误；源码 Oracle 可以直接运行。

steer/follow_up 投递、队列模式、thinking、重试开关和查询等短同步命令按输入行顺序分派。prompt、Bash、模型切换和等待 abort 的长操作仍并发执行。输出由独立 FIFO writer 排空，避免同步命令因 stdout 背压阻塞 EOF 处理；退出仍给 writer 最多 1s 写入时间。200 组流水设置→查询经固定 Pi 与真实 pig 验证；缺失模型字段保持 undefined 错误文本，非字符串身份不被强转后接受。
