# M4.6：查询会话统计与当前上下文

运行 `go run ./examples/session-stats`。示例通过公开 SDK 生成一轮回复、查询统计，然后重开 v3 Session 再次查询。Faux stream 在本地运行，不访问 Provider 网络；临时 Session 在退出时删除。

`GetSessionStats()` 遍历 SessionManager 的全部条目，包括其他分支和压缩前历史。User、Assistant、ToolResult 分别计数；Tool calls 按 Assistant 中的 toolCall 块计数，失败的调用也计算。TotalMessages 只计 type=message 条目，包括开放消息角色；custom_message、压缩和分支摘要条目不增加消息计数。Assistant、带 usage 的 ToolResult、compaction 和 branch_summary 的四类 token 及 cost.total 都加入历史消耗，错误和取消后的已保存 partial 消息也不排除。

`stats.Tokens.TotalTokens` 是 input、output、cacheRead、cacheWrite 的和，不是每条 usage.totalTokens 的累加。Go 沿用已有 ai.Usage 载体：仅填这四项和 TotalTokens；账单费用在 `stats.Cost`，Tokens.Cost 不承载账单，Reasoning/CacheWrite1H 等可选明细保持缺失，不额外累加或补零。原始消息和条目的可选 usage/明细保持原值，包括缺失、null 与显式零。

`GetContextUsage()` 使用当前模型的 contextWindow 和当前分支中的消息。通常以最后一个成功且非零的 Assistant usage 为锚点，后面的消息按已有 UTF-16 字符估算规则计入。该锚点优先使用非零 usage.totalTokens，零时回退四项之和；错误、取消和零 usage 不作为锚点。通过公开 Agent 接口替换消息时，Assistant 值和指针使用同一锚点规则。无锚点时估算全部当前消息，空会话返回零。不读取 system prompt，也不把整个历史账单当成当前上下文大小。

压缩改变了上下文。只要当前分支最新 compaction 后还没有成功、非零的 Assistant usage，Tokens 和 Percent 都为 nil，表示未知；不能用压缩前保留消息的 usage 或压缩摘要自身的费用冒充当前用量。其他分支的压缩不会让当前分支变成未知。

| 返回值 | 含义 |
| --- | --- |
| `usage == nil` | 当前模型没有正数 contextWindow |
| `usage != nil && usage.Tokens == nil` | 窗口已知，压缩后用量未知 |
| `usage.Tokens != nil && *usage.Tokens == 0` | 用量明确为零 |
| `stats.SessionFile == nil` | 内存 Session，没有文件身份 |
| `lastText == nil` | 当前消息中没有可返回的 Assistant 文本 |

`GetLastAssistantText()` 从当前消息向前找最后一条 Assistant，只跳过 content 为空的 aborted 消息。拼接该条的所有 text 块，以 Pi 的 JavaScript trim 规则去除首尾空白；结果为空则返回 nil。它不会越过纯工具调用、thinking 或空 error 回复去找更早的答案，非空错误/取消文本仍可返回。

查询返回独立快照，不写文件、不发事件、不调用 Provider。流式生成中的 StreamingMessage 尚未进入消息历史，因此不会提前计入。同步 message_end 回调发生在该消息持久化之前；历史统计可能仍不含该条，而 Messages 已更新。需要本轮完整统计时在 agent_settled 回调或 Prompt 返回后读取。并发查询保证数据竞争安全，但不同查询调用不是同一个事务，不能假设运行中多次调用必然看到相同时刻。未配置 Agent 的空构造器继续返回结构化未实现错误。

验证：

```sh
node --experimental-strip-types parity/oracle/session-stats.mjs /path/to/locked-pi --check
go test -race ./codingagent -run '^TestSessionStats' -count=1
go run ./examples/session-stats
```

Oracle 读取固定 Pi 实现及 agent-session-stats/compaction 测试，以公开 createAgentSession 的 source/dist 重放 17 组场景，复用 session-interop fixture 的 v3 分支和 compaction 格式。Go 通过公开 SDK 比较，再验证真实 read Tool、流式同步回调、并发读取、错误/取消和独立进程恢复。证据在 `contract:codingagent/session-stats`，API 快照在 `codingagent/testdata/issue88_surface_golden.txt`。

本切片不实现压缩编排、RPCClient/RPC 查询命令、TUI 统计展示、扩展查询 hook 或费用分组 UI；ContextUsage 类型可用不代表扩展运行时可用。源码见 [TypeScript → Go 导航](../mappings/typescript-to-go/m4-session-stats.md)。
