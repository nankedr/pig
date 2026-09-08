# M4.11：从会话直接执行 Bash

运行 `go run ./examples/session-bash`。示例通过公开 SDK 执行宿主命令，观察 chunk 和 final，排除一条本地输出，再用离线 Faux 模型继续对话，最后取消一个命令；不访问 Provider 网络。

`ExecuteBash(ctx, command, ExecuteBashOptions{OnChunk: ..., ID: ...})` 是同步调用。执行期间 `IsBashRunning` 为 true，输出通过 OnChunk 和 `bash_execution_update` 事件交付；事件 ID 仅用于关联输出，不写入消息、也不作为 Session entry ID。多个 Bash 可以同时运行，`AbortBash` 取消全部当前执行。若从回调取消，调用仍会返回包含已有输出的 BashResult。默认没有 timeout；调用者可用 context 设置期限。普通非零退出码属于执行结果，启动或外部 Operations 错误则返回 error，不记录成功消息。

执行使用 SessionManager 的 CWD、SettingsManager 的 shellPath 和 shellCommandPrefix。注入 `Operations` 可以接管宿主边界；即使关闭所有模型工具，也可以直接执行。命令前缀只参与执行，消息保留用户传入的命令。沿用宿主 Bash 的进程组清理和退出后管道读取契约。

直接 Bash 按固定 Pi 逐 chunk 解码 UTF-8、移除 ANSI 与指定控制字符、移除 CR。返回输出保留末尾 2000 行或 50 KiB；超限完整输出保存为临时文件。与模型 Bash 工具的原始字节日志不同，直接 Bash 的完整文件是清理后、未截断的文本。仅原始字节超限但清理后未超限时，也可能有 FullOutputPath 且 Truncated=false。滚动缓冲按 Pi 的 chunk 和 UTF-16 长度规则保留，最终再截断；不要通过 Truncated 推断总输出长度。Dispose 不删除这些文件，调用者可用 read 的 offset/limit 回读，随后自行清理。

`RecordBashResult` 用于记录外部执行结果。两条路径都写 `bashExecution` 消息，不伪造 ToolCall/ToolResult，也不发 message_end。闲时立即加入历史；生成和整轮重试期间先排队，完成或取消后在 agent_settled 前刷新，下一请求才会收到。`HasPendingBashMessages` 与用户 steer/followUp 队列分开；ClearQueue 不清除 Bash 结果。agent_settled 回调期间新增的 Bash 立即进入历史；独立的 Bash 排队标记允许此时可见，同时保留 Session 对 Prompt 的既有防重入约束。

ExcludeFromContext=true 仍保留历史和持久化消息，但不进入模型输入。普通 Bash 转换为一条 user 消息，其中包含命令、输出和失败/取消/截断提示；后续显式调用 Prompt 才开始模型生成。v3 Session 仍沿用首次 assistant 后落盘的既有契约；尚未产生 assistant 的新会话只在内存保存。文件写入错误会返回给 ExecuteBash、RecordBashResult 或刷新队列的 Prompt，不隐瞒失败；底层存储部分写入与跨进程原子性仍遵循现有 SessionManager 限制。

Dispose 发起取消，已经开始的 ExecuteBash 仍完成并记录取消结果；需要确定宿主进程已退出时，等待该 ExecuteBash 返回。Session.Abort 取消模型生成，独立 Bash 使用 AbortBash。RPC/TUI 控件与扩展运行时本切片不交付；本地 Bash 沿用 Darwin/Linux 支持，注入 Operations 可以跨平台。

验证入口：

```sh
node --experimental-strip-types parity/oracle/session-bash.mjs /path/to/locked-pi --check
go test -race ./codingagent -run '^TestSessionBash' -count=1
go run ./examples/session-bash
```

Oracle 固定为 Pi `936aff00918de1187f085f123c2812d8f2d67745` 的公开 createAgentSession。用例与运行测试覆盖输出清理、ID、错误、取消、排队、模型转换、重开、截断回读和进程树清理。证据登记在 `contract:codingagent/session-bash`，API 快照见 `codingagent/testdata/issue93_surface_golden.txt`。
