# M2.9 Legacy Agent steering 与 follow-up

Issue #68 接通公开 `agent.Agent` 的两条独立 FIFO 队列。运行离线示例：

```sh
go run ./examples/legacy-agent-queues
```

示例在 Assistant 流更新时投递 steering 和 follow-up。初次回答结束后，模型先看到 steering，再看到 follow-up，最后输出 `idle=true queued=false`。

## 投递和 turn 边界

`Steer` 与 `FollowUp` 复制输入消息；调用成功后，后续修改输入不影响已投递内容。两条队列默认 `QueueOneAtATime`，每次取一项；`QueueAll` 每次取出当时所有项。模式可在运行中独立修改，各自的 clear 操作互不影响，`ClearAllQueues` 同时清空两条队列。

steering 在首个模型请求前及正常 `turn_end`、`ShouldStopAfterTurn` 完成后被消费。已开始的 Assistant 与整个工具 batch 都先完成，再注入 steering，不打断正在执行的工具。自动工具继续或 steering 尚未结束时，follow-up 保留在队列；只有 Agent 原本会正常停止时，它才开始下一 turn。与固定 Pi 相同，这些 turn 共用一次公开 Prompt 调用和一个 run context，最终仅发布一次 `agent_end`。这里的正常收束不等于先报告 idle 再自动启动新的 run。

`ShouldStopAfterTurn` 返回 true 时保留两条队列，直接结束；Provider error/aborted 或工具取消也不自动消费 follow-up。`Continue` 可以从 user/ToolResult 尾部恢复；assistant 尾部必须有待处理消息，优先取 steering，再取 follow-up。从 steering 恢复时跳过首轮重复轮询，保证 one 模式不会一次取两项。

## 状态与并发

idle 时投递会返回明确错误。busy 时 Prompt/Continue/Reset、空 transcript Continue、无队列的 assistant 尾部 Continue 同样报错，并保留队列和 transcript。收尾与已取消的 run 不再接收投递。最后一次空队列检查与关闭接收在同一锁内，避免已成功投递的消息错过正常收尾边界；同时到达的消息以入队锁顺序定义 FIFO。

`Abort` 取消当前 context，Provider 的部分内容仍通过 aborted outcome 写入 transcript。未消费队列保留，调用方可 Continue 或 Reset。`Reset` 必须在 idle 时调用，清理 transcript、错误、streaming 状态、pending tools 和两条队列，同时保留模型、tools、配置、模式与订阅。

`WaitForIdle` 只等待调用时的 run。Listener 按注册顺序串行执行；`agent_end` listener 全部完成前，Busy 保持 true。结束 listener 的错误汇总给 Prompt/Continue，仍执行后续结束 listener。取消等待方 context 仅取消该 waiter，不取消 Agent；Provider、工具及其他用户回调须遵守传入 context 并结束自己的工作。

## 基线差异和证据

固定 Pi 接受 idle 与 `agent_end` listener 中的入队；其结束 listener 抛错会中断 dispatch，并进入失败处理再次发布 `agent_end`。Pig 按 issue #68 收紧投递状态，并在原终态上等待全部结束 listener，避免重建失败消息覆盖已产生结果。这些差异见 [ADR-0018](../adr/0018-legacy-agent-queue-admission.md) 和独立 deviation fixture，不作为相同语义重放。

```sh
go test -race ./agent -run 'TestLegacyAgent|TestAgentPromptTextSettles' -count=10
go test ./internal/parity -run '^TestLegacyAgent' -count=1
node --experimental-strip-types parity/oracle/legacy-agent-queues.mjs .upstream/pi --check
```

共同 Parity Case 覆盖四种 one/all 组合、正常运行和停止后 Continue 两种路径的请求分组、生命周期顺序、listener 顺序及最终状态。Go 测试另验证工具边界、清队列、非法状态、确定性 abort、并发投递、waiter 与 producer goroutine 的结束。SDK 公开签名沿用 issue #29 的 surface snapshot；本切片不新增公开类型。Harness、PrepareNextTurn 和默认 stream 安装仍按原 Capability 边界处理。
