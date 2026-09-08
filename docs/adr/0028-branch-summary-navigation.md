# 导航时保留离开分支的摘要

Issue #92 延续 ADR-0027 的 Session 导航提交边界，实现固定 Pi 936aff0 的默认 branch summarizer。共同祖先选择以用户选择的目标节点为准，摘要范围为旧 leaf 到共同祖先之间的路径；用户/custom 目标实际挂载在其父节点。branch_summary 的 fromId 按 Pi 记录挂载位置（根位置为 root），不是离开路径的 leaf。标签标记摘要节点，标签记录自身成为 leaf。

预算为当前模型 contextWindow（缺省 128000）减去 settings.branchSummary.reserveTokens（缺省 16384）；显式设置 0 保留整个窗口，非正剩余预算按基线不限制。独立 Go GenerateBranchSummaryOptions 的零值表示缺省 16384；Session 使用已解析设置保留显式零值。从新到旧收集消息，跳过工具结果，保留 compaction/branch_summary 的文本。摘要超出预算时，若已收集 token 严格小于预算的 90%，仍保留该摘要；90% 比较不向下取整。已有非 hook 分支摘要贡献累计文件记录；工具调用只遍历到预算截止处，包括超出预算的当前条目。修改文件从读取列表去重。请求输出预算固定为 2048，不继承当前 thinking。CustomInstructions 默认追加；ReplaceInstructions 仅在自定义文本非空时替换默认提示。

摘要复用当前 Session stream 与已交付 ModelRuntime/认证契约。每次摘要使用独立 sessionId，禁用缓存；重试复用同一请求的 ID，重试预算来自 settings.retry。订阅流仅发布 summarization_retry_scheduled、source=branchSummary 的 summarization_retry_attempt_start 和 summarization_retry_finished，不伪造扩展 session_tree 或 compaction_start/end。

摘要期间 IsCompacting 为真，WaitForIdle 覆盖生成和导航提交；Prompt、配置变更和其他导航/压缩拒绝重入。AbortBranchSummary 只取消分支摘要，Abort/Dispose 也会取消，AbortCompaction 不影响它。取消返回 Cancelled/Aborted，外部 Go context 取消额外返回可由 errors.Is 识别的 error；普通模型错误及重试耗尽返回模型错误并保留原路径。自定义 stream 的 Go error 不推断为可重试。

生成期间释放 Session/SessionManager 锁，提交前重新检查取消、disposed、原 leaf 和条目数。摘要和标签一起写临时 v3 文件，Agent 消息替换成功后原子 rename，再发布条目/leaf；写入失败恢复原消息，结果不暴露未提交摘要。提交检查后到达的 Abort 不撤回成功结果。此原子性增强基线，沿用 ADR-0025/0027；不保证跨进程写入，也不把直接修改底层 Agent 视为 Session 事务。

跨分支往返按基线生成新的离开路径摘要；已有摘要作为输入并累计文件信息，不把全树重新摘要。当前 leaf no-op、没有离开条目的前向导航不调用模型。没有可摘要消息的非空路径持久化 “No content to summarize”。原路径始终保留，重开与下一次 Prompt 使用标准 v3 上下文投影。

扩展 hook、自定义扩展摘要/取消、RPC/TUI 操作、自动压缩和摘要期间队列自动排空保持后继切片边界；本切片不冻结 M4。
