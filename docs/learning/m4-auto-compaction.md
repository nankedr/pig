# M4.8：自动压缩并从上下文溢出恢复

运行 `go run ./examples/auto-compaction`。示例先生成两轮历史，用确定性 Provider 模拟 prompt is too long，自动摘要并继续生成，再重开 v3 会话继续任务，全程离线。

```go
err := session.SetAutoCompactionEnabled(true)
if err != nil { return err }
err = session.Prompt(ctx, "继续任务")
```

开关默认开启。每次 Prompt 前检查上次 Assistant，捕捉之前中止但已接近阈值的回答；本轮 Agent 结束后先处理 Provider retry，再判断是否压缩。窗口来自当前 Model.ContextWindow，阈值是 contextWindow - reserveTokens，严格超过才触发。usage 包含 input、output、cacheRead、cacheWrite；错误或全零 usage 以最近有效 usage 加尾随消息估算。已被摘要覆盖的旧 usage 不会再次触发。

threshold 压缩只准备下一轮上下文；有排队消息时才续跑。overflow 错误和低于模型原始 maxTokens 的 length 截断可压缩后继续一次。成功答案的 input 已超过窗口时也按 overflow 压缩，但不再次生成该答案。同一输入恢复后仍 overflow 时发布明确失败终态，避免无限循环。

事件顺序通常为 agent_end → compaction_start → compaction_end → 恢复的 agent_start … → agent_settled。agent_end.willRetry 仅指 Provider retry；compaction_end.willRetry 才是溢出恢复标志。摘要自己的 summarization_retry_* 不占整轮 RetryAttempt，AbortRetry 也不取消摘要，使用 AbortCompaction 即可。

自动压缩复用手动压缩的 cut point、摘要预算和 v3 原子提交。旧消息完整留在文件中，模型上下文重建为摘要加保留尾部。失败 Assistant 只从运行上下文移除，Headless 仍能返回它的 partial 与取消状态。WaitForIdle 等待整个恢复过程。

压缩时可以 Steer、FollowUp，或通过 Prompt 的 streamingBehavior 指定投递队列；Session 暂存后按 one-at-a-time/all 模式续跑。运行后取消压缩会保留这些消息，可显式 Prompt 继续或 ClearQueue 清空；输入前检查中的 AbortCompaction 只取消摘要，已提交的 Prompt 仍执行。普通 idle 和 Provider retry 等待期仍沿用原有入队限制。

基线有一个已验证限制：503 后紧接 overflow，重建可能保留前一个失败 Assistant，导致继续时报 cannot continue from message role: assistant。Pig 明确返回此终态并保留历史。空摘要后仍 overflow 也只尝试一次，不表示恢复成功。

验证命令：

```sh
node --experimental-strip-types parity/oracle/auto-compaction.mjs /path/to/locked-pi --check
go test -race ./codingagent -run '^TestAutoCompaction' -count=1
go run ./examples/auto-compaction
```

Parity Catalog：contract:codingagent/compaction；API 快照：codingagent/testdata/issue90_surface_golden.txt。RPC/TUI 控件、扩展 hook 和 branch summary 仍待后续切片。设计决策见 [ADR-0028](../adr/0028-auto-compaction.md)，源码导航见 [TypeScript → Go](../mappings/typescript-to-go/m4-auto-compaction.md)。
