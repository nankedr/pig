# M4.14：从 RPC 管理会话与压缩

运行 `go run ./examples/rpc-lifecycle`。示例编译真实 pig 子进程，用本地 Provider 完成两轮对话，从第二个用户节点 fork，继续另一条路径，再切回原会话、压缩并继续。全部文件写入临时目录，无需线上凭证。

| 操作 | 公开 Go 入口 | 可观察结果 |
| --- | --- | --- |
| 新建与恢复 | NewSession、SwitchSession | 返回 cancelled；GetState 可见新 SessionID 和 SessionFile |
| 分叉与复制 | GetForkMessages、Fork、Clone | Fork 返回待编辑的用户文本；Clone 保留当前 leaf 历史 |
| 增量与树查询 | GetEntries(ctx, since)、GetTree | since 不含自身；返回当前 leaf；条目遵守正式 v3 格式 |
| 命名 | SetSessionName | 去除首尾空白，写入 session_info |
| 手动压缩 | Compact(ctx, instructions) | 返回摘要和保留点；原树条目仍在，后续请求使用压缩上下文 |
| 自动压缩 | SetAutoCompaction | 持久化设置；overflow 可发摘要并恢复生成 |

每个响应通过 id 关联请求；Prompt 的成功响应表示接纳，生成通过异步事件继续。Compact 的响应在提交之后返回。请求 Context 取消只停止客户端等待；Abort 才取消远端生成或压缩。压缩期间可以查询，但新的普通 Prompt 被拒绝。替换会取消并等待旧工作结束，再重绑 Session 和监听器；新的写入不会追加到旧文件。

重开验证可使用 `Stop → Start → SwitchSession(savedPath)`，然后用 GetMessages 与 `OpenSessionManager(savedPath).BuildSessionContext()` 比较。显式 CLI 模型与 thinking 仍优先于目标会话历史；省略显式选择才能验证保存的配置恢复。

固定 Pi 只从扩展 command context 提供 NavigateTree/分支摘要，没有相应 wire 命令。当前可以查询树并 fork，摘要导航继续使用公开 SDK `AgentSession.NavigateTree`；RPC 的扩展入口仍待交付。`navigate_tree` 和 `abort_compaction` 返回 Unknown command。

测试：`go test -race ./cmd/pig -run '^TestRPC96' -count=1`。对等目录条目为 `contract:rpc/session-lifecycle`。重跑固定源码：`node --experimental-strip-types parity/oracle/rpc-lifecycle.mjs <locked-pi-checkout> --check`。详细提交和兼容边界见 [ADR-0032](../adr/0032-rpc-session-lifecycle.md)。
