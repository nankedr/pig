# 会话配置：TypeScript → Go

| 固定 Pi 入口 | Pig 入口 | 观察行为 |
| --- | --- | --- |
| core/agent-session.ts setModel / cycleModel | codingagent/session_configuration.go SetModel / CycleModel | 认证验证、scope 过滤、双向轮换、模型快照 |
| setThinkingLevel / cycleThinkingLevel / getAvailableThinkingLevels / supportsThinking | 同文件同名 Go 方法 | 能力裁剪、偏好恢复、变化事件 |
| setScopedModels / scopedModels | SetScopedModels / ScopedModels | 非持久化 scope 和所有权快照 |
| setActiveToolsByName / _refreshToolRegistry | SetActiveToolsByName / SDK 注册表 | 允许排除规则、工具说明与定义一起更新 |
| core/sdk.ts createAgentSession | codingagent/sdk.go CreateAgentSession | ModelRuntime 与动态认证、创建及恢复 |
| session-manager.ts / settings-manager.ts | codingagent/session_configuration_persistence.go | v3 配置记录与默认值的失败回滚 |
| test/suite/agent-session-model-extension.test.ts | parity/oracle/session-configuration.mjs / codingagent/issue87_configuration_test.go | 公开 SDK 的固定基线确定性证据 |
| test/suite/regressions/6949-unavailable-scoped-model.test.ts | CycleModel 候选过滤与公开 SDK 测试 | unavailable scope 条目保留、轮换跳过 |
| test/suite/regressions/6162-extension-active-tools-next-turn.test.ts | 后续扩展切片 | 当前 busy 拒绝，未声称同一运行内修改工具的对等 |

[学习材料](../../learning/m4-session-configuration.md)和 [ADR-0025](../../adr/0025-session-configuration.md)解释具体规则及边界。示例：`go run ./examples/session-configuration`。
