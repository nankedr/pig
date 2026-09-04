# M2.4 Deferred tools 与动态 Tool 定义

`ai.SplitDeferredTools(input, enabled, normalizeName)` 返回 immediate、deferred 两个有序 `[]ai.Tool`。它只处理当前 `Context.Tools`：先按规范化名称去重，同名定义保留最后一个版本，位置保留第一次出现的位置；然后根据 transcript 的 ToolCall 和 ToolResultMessage.AddedToolNames 拆分。定义中的名称、描述、参数 schema 和 constrained sampling 原样保留，归一化只用于匹配。

关闭 deferred 时，去重后的全部 Tool 都进入 immediate。开启时，仅被 AddedToolNames 标记且整个 transcript 从未调用的当前 Tool 进入 deferred。标记重复不增加定义，标记缺失于当前 Tool 集时也不会恢复已移除的 Tool。ToolResult 的 ToolName 本身不是发现标记；只有 Assistant ToolCall 表示调用，和结果是否成功无关。

normalizeName 为 nil 时按原名称匹配；需要名称转换的调用者必须让 Tool 定义、ToolCall 和 added names 使用同一函数。Go 的值和指针消息变体具有相同行为，缺失、null、空 added names 都不增加标记。函数不改写输入；返回的 Tool 元数据保持普通 Go 值语义，Parameters 等引用字段与输入共享，调用者修改前应自行复制。

固定 Pi 仅识别“先调用、后标记”为 immediate；Pig 根据 #63 还支持“先标记、后调用”恢复 immediate，详见 [ADR-0016](../adr/0016-deferred-tools-used-promotion.md)。删除 transcript 中的调用记录会影响下一次拆分，函数没有额外的隐藏状态。

离线运行：

```sh
go run ./examples/deferred-tools
go test ./ai -run 'TestSplitDeferredTools|TestIssue63LockedGoAPISnapshot' -count=1
go test -race ./agent ./internal/parity -run '^TestDeferredTools' -count=1
node --experimental-strip-types parity/oracle/deferred-tools.mjs .upstream/pi --check
```

示例通过 Models/Faux 发出三次请求：先调用 discover 并新增 read、unused 定义，再观察两者 deferred，最后调用 read 后观察其恢复 immediate。Faux factory 使用公共 helper 观察每次完整 Context，不模拟具体 Provider 的 wire 方言。

Agent 验证覆盖实际 Tool 执行：发现 Tool 返回 AddedToolNames 和 Terminate，当前 run 在 ToolResult 处结束；宿主调用 `SetTools` 注册新执行器，再用 `Continue` 发起后续请求。已有 Agent 在 run 开始时捕获 Tool 集，此切片沿用该行为；AddedToolNames 是发现标记，宿主仍负责提供定义和执行器。OpenAI Chat Completions 的 deferred ToolResult、Kimi/Anthropic/OpenAI Responses wire 支持继续保留到 M10，原有 Capability Stub 不变。
