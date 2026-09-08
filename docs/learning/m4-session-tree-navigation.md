# M4.9：在 Session 树中导航并继续分支

运行 `go run ./examples/session-tree-navigation`。示例用离线 Faux stream 创建两轮对话，回到第一轮继续新分支，再重开并访问原分支；所有文件都位于临时目录。

```go
result, err := session.NavigateTree(ctx, targetID, codingagent.NavigateTreeOptions{Label: "重新探索"})
if err != nil {
    return err
}
if result.EditorText != nil {
    fmt.Println("可编辑的原消息：", *result.EditorText)
}
return session.Prompt(ctx, "换一个方向继续")
```

先用 `session.SessionManager().GetTree()` 展示节点和 label，传入目标的 entry ID。label 是节点注释，不是可直接传给 NavigateTree 的别名。选择用户消息或 custom_message 会退到其父节点，返回拼接后的文本，图片不进入 EditorText；选择 assistant、工具结果、配置、压缩记录等其他节点会包含该节点重建上下文。当前 leaf 的导航是无副作用 no-op。根用户节点的父节点为空，所以导航后消息为空；若添加 label，新 leaf 是挂在根位置的标签记录。

导航保留同一 Agent、SessionManager、文件、Session ID、资源及订阅。下一次 Prompt 的用户消息写在新 leaf 下，旧分支完整保留。无标签导航本身不写文件，也不保存独立游标；尚未继续生成就重开，会回到文件最后追加的记录。标签或后续消息把新分支位置持久化。

特别注意配置：固定 Pi 在 navigateTree 中保留**当前 model/thinking、工具和 system prompt**，只替换消息上下文；`BuildSessionContext` 则描述目标路径的历史 model/thinking。新 assistant 写下实际使用的模型；thinking 若没有新的 thinking_level_change，重开仍恢复路径历史等级。Oracle 同时核对这两个视角，不能将导航实现为一次隐式 SetModel/SetThinkingLevel。

无效目标、已取消 context、已释放会话及忙状态均返回 error。整个 Prompt、重试等待、同步通知回调期间不能导航；在方法返回后再继续 Prompt。带标签导航的写入失败会恢复旧状态和消息。只读快照不会看到半切换，资源和事件监听器也不会重绑。

```sh
node --experimental-strip-types parity/oracle/session-tree-navigation.mjs /path/to/locked-pi --check
go test -race ./codingagent -run '^TestSessionTreeNavigation' -count=1
go run ./examples/session-tree-navigation
```

Parity Catalog 登记 `contract:codingagent/session-tree-navigation`，API snapshot 为 `codingagent/testdata/issue91_surface_golden.txt`。测试从公开 CreateAgentSession/NavigateTree/Prompt 进入，比较固定 Pi 源码及构建产物，覆盖根、祖先、其他分支、custom、工具结果、已有摘要/压缩/标签/配置节点以及继续生成和重开。失败及监听器生命周期由公开 SDK 测试补充。

本切片实现无摘要路径。不同目标的 Summarize=true 返回 ErrNotImplemented；CustomInstructions 和 ReplaceInstructions 在无摘要时按基线忽略。扩展 session_before_tree/session_tree hook、扩展取消、摘要生成与取消重试、RPC 和 TUI 控制继续保留 Stub。独立 Session fork 沿用 M3。[设计决策](../adr/0026-session-tree-navigation.md)与 [TypeScript → Go 导航](../mappings/typescript-to-go/m4-session-tree-navigation.md)给出对应位置。
