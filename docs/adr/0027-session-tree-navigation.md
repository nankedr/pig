# 同一 Session 树的导航与提交边界

Issue #91 遵循固定 Pi 936aff0 的 `AgentSession.navigateTree`：当前 leaf 是目标时直接返回；用户消息和 custom_message 选择其父节点并返回纯文本 EditorText，父节点为空表示空上下文；其他节点直接成为 leaf。非空 label 标记目标节点，但 label 记录自身成为新 leaf，后续消息接在它后面。无摘要路径忽略 CustomInstructions/ReplaceInstructions。

导航重新构建消息上下文，沿用 SessionManager 对 compaction、branch_summary、配置和 label 的既有解释。固定 Pi 不在这里恢复历史 model/thinking，也不修改 settings、工具或 system prompt。当前会话配置保持不变，SessionContext 仍表示所选路径的历史配置；下一条 assistant 的模型身份会进入新路径，thinking 历史仅由 thinking_level_change 表示。因此重开时 thinking 可能与导航前的实时值不同，这是基线行为，不自动补写配置条目。

无标签导航只移动内存 leaf，不新增持久化 cursor。直接重开会选文件中最后追加的条目；继续 Prompt 或追加 label 后，新路径可通过标准 v3 parentId 重开，旧分支条目完整保留。导航不创建独立 Session fork。

Go 方法接收 context.Context，返回 NavigateTreeResult 和 error；取消通过 context error 表达。它与 Prompt、配置修改和 Dispose 共享 Session 锁，并在整个运行（含重试和通知）期间拒绝导航。SessionManager 锁保护候选路径与落盘位置。带标签的持久化先写临时文件，在消息所有权验证成功后 rename；rename 失败恢复旧消息，不提交 leaf/label。公开 Session 快照看不到中间状态。此规则增强固定 Pi 对标签写入失败的处理，延续 ADR-0025 的失败不半切换约定；直接并发修改底层 Agent 不属于 Session 事务，多个进程同时写同一个 Session 文件仍受已有单写者约束。

资源、Agent 和订阅均不重建。Pi 的 session_tree 是扩展事件，不在 AgentSession 订阅流中伪造新事件。扩展 hook、扩展取消、摘要生成及其 Abort/Retry、RPC/TUI 导航留给后继切片。对不同目标请求 Summarize 返回结构化 Capability Stub，当前 leaf 的 no-op 保持 Pi 的优先级，即使 Summarize=true 也不调用摘要路径。

后续 ADR-0030（Issue #92）已实现默认摘要生成、取消与重试，替代本 ADR 中相应的临时 Stub；扩展 hook 仍然延后。
