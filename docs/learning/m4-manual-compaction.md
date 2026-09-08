# M4.7：手动压缩后继续任务

运行 `go run ./examples/manual-compaction`。示例通过公开 SDK 生成两轮对话、压缩、重开 v3 文件，再由 Headless 继续。Faux stream 检查后续输入中出现摘要，全程不访问 Provider 网络。

```go
result, err := session.Compact(ctx, "保留目标、约束和关键文件")
if err != nil {
    return err
}
fmt.Println(result.Summary)
err = session.Prompt(ctx, "继续")
```

Compact 会先停止当前生成并等待工具和消息持久化完成。它从 SessionManager 当前分支读取历史，以 keepRecentTokens 向后寻找保留点，ToolResult 始终跟随原 ToolCall。如果切在一轮中间，分别总结更早历史和本轮前缀，再拼成一份摘要。已有 compaction 的 summary 作为更新输入，之前仍保留、现在移出窗口的消息也进入新摘要。自定义指令用于历史摘要，Pi 的前缀摘要提示不附加这些指令。

历史摘要最多使用 reserveTokens 的 80%，前缀摘要最多使用 50%，均受模型 maxTokens 限制。TokensBefore 以最近有效 Assistant usage 加尾随估算计算；EstimatedTokensAfter 是新上下文的逐条估算。Usage 只合并成功摘要请求的用量，不含失败重试。

摘要保留目标、约束、进度、决策、下一步及关键上下文。工具调用中的 read/edit/write 路径和上次非扩展摘要的文件列表合并写入 details；被修改过的文件从只读列表中去除。相同信息也以 read-files/modified-files 标签附在摘要末尾。

摘要请求沿用当前模型、thinking、认证与 HTTP 设置，但使用独立 sessionId 和 cacheRetention=none。settings.retry 控制摘要重试，settings.retry.provider 继续控制单次 HTTP 请求。事件顺序为 compaction_start、可选 summarization_retry_scheduled / summarization_retry_attempt_start / summarization_retry_finished、compaction_end。摘要重试不会改变整轮 RetryAttempt，AbortRetry 不取消摘要；请用 AbortCompaction 或 Context。

失败和取消时没有新 compaction 条目，不裁剪上下文。成功追加 v3 条目后，从 summary 和保留区间重建 Agent 消息；旧消息仍在文件里。重开时同样重建，因此摘要真正进入下一次模型请求。连续立即压缩返回 Already compacted；没有可移出的历史返回 Nothing to compact。compaction_end listener 可开始下一次生成；请不要在活跃生成的同步 listener 内阻塞调用 Compact。

验证：

```sh
node --experimental-strip-types parity/oracle/manual-compaction.mjs /path/to/locked-pi --check
go test -race ./codingagent -run '^TestManualCompaction' -count=1
go run ./examples/manual-compaction
```

Parity Catalog：contract:codingagent/compaction。固定 Pi 源码和发布构建的公开 SDK Oracle 验证请求、预算、裁剪、更新、文件信息及重试；Go 的真实 HTTP 测试另验证认证刷新、SSE 截断恢复和 Headless 重开。API 快照见 codingagent/testdata/issue89_surface_golden.txt。

自动压缩、扩展摘要与 hook、branch summary、RPC/TUI 压缩入口、压缩期间自动排空队列仍未实现。详见 [ADR-0026](../adr/0026-manual-compaction.md)和 [TypeScript → Go](../mappings/typescript-to-go/m4-manual-compaction.md)。production v3 与 Harness v4 保持独立。
