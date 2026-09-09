# 导航时带走探索结论

M4.10 在同一个 Session 树内切换路径时，把离开的探索摘要带到目标上下文。运行 `go run ./examples/branch-summary`，可看到生成摘要、目标分支继续生成，以及持久化重开后原分支仍可访问。

```go
result, err := session.NavigateTree(ctx, targetID, codingagent.NavigateTreeOptions{
    Summarize: true,
    CustomInstructions: "保留结论和关键文件",
    Label: "探索结论",
})
if err != nil { return err }
if result.Cancelled { return nil }
return session.Prompt(ctx, "结合探索结论继续")
```

共同祖先到旧 leaf 之间的离开路径进入摘要；选择的目标若是用户/custom 消息，EditorText 返回其文本，新摘要挂在父节点。选择 assistant 则挂在该节点。根用户消息会产生根摘要。标签标记新摘要；旧分支的消息不会删除。`SummaryEntry.FromID` 是摘要挂载位置，这是固定 Pi 的字段语义。

摘要请求从新消息向旧消息取预算，跳过工具结果，但保留 assistant 工具调用和已有摘要。文件记录去重后存进 Details，并附在摘要文本末尾。`branchSummary.reserveTokens` 留给提示和响应；显式 reserveTokens=0 使用完整 contextWindow，缺省时预留 16384；输出请求固定 2048 token。自定义指令默认追加，ReplaceInstructions=true 且自定义指令非空时替换默认格式。当前模型继续用于摘要，但摘要不继承 thinking 等级。

摘要不立即执行后续生成；成功返回后调用 Prompt。模型通过标准 branchSummary 消息投影收到摘要，原分支仍可通过 NavigateTree 返回。同一 leaf 的重复导航不生成摘要；往返另一分支时仅处理新的离开路径及其中已有摘要。

摘要期间可调用 `AbortBranchSummary()`；检查 `Cancelled` 和 `Aborted`。外部 context 取消也返回 context error。生成失败、配额错误、重试耗尽和文件写入失败保留原路径。`IsCompacting()` 包含分支摘要，`WaitForIdle(ctx)` 等待导航提交完成。同步重试回调可取消，但不能在其中阻塞等待当前操作。

```sh
node --experimental-strip-types parity/oracle/branch-summary.mjs /path/to/locked-pi --check
go test -race ./codingagent -run '^TestBranchSummary' -count=1
go run ./examples/branch-summary
```

21 个 Oracle 场景从 Pi source/dist 和 Go 公开 SDK 比较请求、事件、摘要文件、重开与继续生成。真实 HTTP 测试补充动态认证和截断重试，公开 SDK 测试补充并发占用、取消、原子写入和数据所有权。Catalog 为 `contract:codingagent/branch-summary`，API snapshot 为 `codingagent/testdata/issue92_surface_golden.txt`。

扩展 hook、扩展自定义摘要与取消、摘要的 RPC/TUI 控制留给后继切片；自动压缩已由 M4.8 交付。详见 [设计决策](../adr/0030-branch-summary-navigation.md)与 [TypeScript → Go 导航](../mappings/typescript-to-go/m4-branch-summary.md)。
