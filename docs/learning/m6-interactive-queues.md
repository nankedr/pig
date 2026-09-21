# M6.8：生成中插话、排队与取消

InteractiveMode 在一个输入循环中处理终端动作，当前 RunHeadless 在独立 worker 中运行；两者共享既有 AgentSession。UI 不实现第二套 Agent 调度或 transport retry。

| 入口 | 行为 |
| --- | --- |
| 生成时 Enter | 投递 steering，在工具轮次边界优先消费 |
| 生成时 Alt+Enter | 投递 follow-up，等待 Agent 没有工具和 steering 后消费 |
| 空闲时 Alt+Enter | 与 Enter 相同，启动新一轮 |
| Alt+↑ | 原子取出尚未消费的两类队列，按 steering、follow-up、当前草稿回填；段落之间空一行 |
| Escape | 生成中回填队列并取消当前轮；重试等待中取消整轮重试等待；补全菜单打开时先关闭菜单 |
| Ctrl+C | 清空编辑器；连续两次退出 |
| Ctrl+D（空编辑器）、/quit | 取消活动轮，等待清理并退出 |

回填后可以直接修改再提交，也可以 Ctrl+C 丢弃。队列内容只保存在内存；只有已消费的用户消息进入 v3 Session。Assistant、Bash Tool 的部分输出在取消后仍显示，并与 Session 中的 aborted / error 结果对应。下一轮沿用同一 Session。

运行 `go run ./examples/interactive-queues` 查看离线 SDK 的入队、回取、重新投递和消费。真实终端使用 `go run ./cmd/pig`；也可运行现有 `examples/interactive-text`。取消、插话、回取快捷键均服从 keybindings.json。

## 结束与并发

AgentSession 的 agent_end 仍然属于 busy；完整 settled 收尾后才能发起新 Prompt。最后 token 可先于该屏障出现。此时 Agent 已拒绝的新输入会显示等待提示，在当前 worker 返回后作为新的 Prompt 提交一次；如果这轮被取消，则回填编辑器。已经成功入队的消息不会被重发。输入携带本地轮次标识，迟到的取消动作不能取消下一轮。

`Agent.TakeQueuedMessages` 在消费队列所用的同一把锁下取出未消费项。`AgentSession.TakeQueuedMessages` 返回两类文本并发送 queue_update，保留既有 ClearQueue 的 error-only 签名。不能使用“查询展示队列，再清空”的组合模拟回取：Agent 可能已取走消息，Session 的 message_start 还没收到，这样会复制已消费的消息。

重试状态来自 auto_retry_start / auto_retry_end：显示次数、等待倒计时和取消键；不修改 Session 的退避和错误分类，也不实现 Provider 内部的 transport retry。沿用 ADR-0018，在重试等待期间新入队会被拒绝并回填，不能宣称该分支与 Pi 完整对等。

## 证据与范围

固定 Pi `936aff00918de1187f085f123c2812d8f2d67745` 的真实 CLI 在 PTY 和本地 SSE 服务上提取五个 fixture：两类队列优先级、回取清除、取消回填编辑、重试取消、重试成功。普通验收不需要 Pi、Node、Python或在线 Provider。

```sh
go test ./cmd/pig -run 116 -count=1
PIG_TEST_RACE=1 go test -race ./cmd/pig -run 116 -count=3 -shuffle=on
go test -race ./codingagent -run '116|^TestInteractiveSDK' -count=3 -shuffle=on
node parity/oracle/queues.mjs /path/to/locked-pi --check
```

`PIG_TEST_RACE=1` 仅供本票测试，将真实 pig 子进程也编译为 race binary。公开 SDK 回归覆盖 one/all 模式下回取与消费竞争、回调重入；CLI 覆盖退出时清理、请求取消、部分输出和持久化。已有 Session/Agent 的 admission、取消和 retry 测试继续适用。

Parity Catalog 条目 `contract:codingagent/interactive-queues` 保留 partial：不声明所有终端外观、compaction UI、扩展运行时、图片输入、所有 Tool 或六平台都已验收；#99 包生态不影响本切片。
