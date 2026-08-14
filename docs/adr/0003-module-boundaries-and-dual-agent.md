# 保持模块边界与双轨 Agent 架构

Pig V1 完整移植核心模块 `ai`、`agent`、`codingagent` 及支撑模块 `telemetry`、`tui`、`protocol`、`client`，不纳入 `server`、`evals` 和 `session-backends/sqlite-node`，也不实现 Pig Server。生产路径继续使用 legacy `Agent`、`AgentSession` 和 v3 Session，Harness 路径只复刻固定快照已有的 v4 底座及相同未实现结果。该边界保留 Pi 正在迁移中的真实双轨状态，而不是猜测上游未来架构。
