# M4.3 TypeScript → Go：Session 用户消息

Pi 固定源码：`936aff00918de1187f085f123c2812d8f2d67745`。

| Pi 导航 | Pig 导航 | 可观察契约 |
| --- | --- | --- |
| `core/sdk.ts#createAgentSession` | `codingagent/sdk.go#CreateAgentSession` | 最高层 SDK 装配，测试通过此处创建 Session |
| `core/agent-session.ts#sendUserMessage` | `codingagent/session_messages.go#SendUserMessage` | string/文本块归一化；空闲同步执行，运行中由 DeliverAs 路由 |
| `core/agent-session.ts#prompt` | `codingagent/session.go#Prompt` | context 取消与 StreamingBehavior |
| `core/agent-session.ts#steer/followUp` | `codingagent/session_messages.go#Steer/FollowUp` | 委托 Legacy Agent admission 与 FIFO 调度；文本路径 |
| `core/agent-session.ts#_handleAgentEvent` | `codingagent/session.go#handleAgentEvent` | 消费时更新展示，message_end 持久化 |
| `core/agent-session.ts#_emitQueueUpdate` | `codingagent/session_messages.go#dispatchQueueEvents` | 按接收顺序通知，防御性快照，回调不持锁 |
| `core/agent-session.ts#setSteeringMode/setFollowUpMode` | `codingagent/session_messages.go#setQueueMode` | 保存设置并更新 Agent 模式 |
| `core/agent-session.ts#clearQueue` | `codingagent/session_messages.go#ClearQueue` | 清除两条底层队列；Go 既有签名仅返回 error |
| `agent-session-concurrent.test.ts` | `codingagent/issue85_messages_test.go` | 公开 SDK 对等、取消和收尾并发测试 |

`parity/oracle/session-messages.mjs` 通过固定 Pi 的 createAgentSession 注入确定性 Provider，生成共同与偏离 fixture；`codingagent/testdata/issue85_surface_golden.txt` 锁定公开 Go 接口。图片和模板展开等未实现分支见 [学习材料](../../learning/m4-session-messages.md)。
