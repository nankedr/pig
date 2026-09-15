# 会话重载保持基线阶段语义并明确 Go 并发接纳边界

2026-09-15，依据 #106：本地资源通过既有 `AgentSession.Reload(context.Context)` 刷新。保留固定 Pi 的设置 → 队列模式 → 资源 → system prompt 顺序，错误不隐式回滚已完成阶段，也不返回伪成功；历史、持久化位置、分支、模型和 Tool 不重建。扩展 runtime、trust hooks 和 ABI 不在此决策范围。

活动生成与重载可并行，已交给 Provider 的请求不受影响。Go 可以从不同 goroutine 同时调用 SDK，因此重载期间拒绝另一重载、新消息接纳、配置修改、会话替换及新的独立压缩/Bash 操作；读取允许继续，加载器的单次发布受锁保护。命令展开跨越重载时拒绝接纳，调用者可重试。该接纳限制是显式 Go 并发偏离，不宣称固定 Pi 对竞争调用也返回 busy。

取消采用阶段语义：入口取消无副作用；后续阶段取消保留已完成的设置/资源阶段，最终 system prompt 仅在查询、context 和存活检查均通过后发布。注入加载器必须合作响应 context，并负责自己的发布原子性；Pig 不用 goroutine 丢弃后台加载来伪造及时取消。

本决策细化 ADR-0010：会话重载显式刷新相关 SettingsManager 并保持已确定的信任状态；直接调用注入的 DefaultResourceLoader 仍使用已准备设置。它不重新运行 trust hooks，也不会因全局默认策略改变而偷偷授信。
