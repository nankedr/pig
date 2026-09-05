# M3.1 v3 Session 持久化

Pig 的 Headless CLI 默认创建生产 v3 Session。默认目录是 `~/.pig/agent/sessions/--<cwd>--/`；`PIG_CODING_AGENT_DIR`、`PIG_CODING_AGENT_SESSION_DIR` 和 `--session-dir` 可以显式覆盖 Pig 路径。普通运行只使用 Pig 状态，不扫描或迁移 `~/.pi`、项目 `.pi`、凭证或 trust。

## 创建与首次写入

`NewSessionManager` 先确定 Session ID 和目标路径，但不会立刻创建 JSONL 文件。model、thinking 和首条 user 消息先保留在内存；第一条 Assistant 消息（包括 error 或 aborted 的部分结果）到达时，writer 用排他创建一次写入 header 和已有 entry。之后每个 entry 只追加一行。显式打开空文件会立即写入 header；非空非法 Session 会报错并保持原内容。M3.2 已支持 v1/v2 原地迁移，详见[历史 Session 恢复](m3-session-interop.md)。

每条 entry 都包含 ID、时间戳和 `parentId`。正式 writer 覆盖 `model_change`、`thinking_level_change` 与 Agent message；ToolResult 作为 message 持久化。Provider 失败、取消和清理不会丢掉已经形成的 Assistant outcome。磁盘写入失败会返回可观察错误，内存 outcome 仍可检查，但不会伪报已持久化。

## 重开与内存模式

使用显式路径跨进程继续：

```sh
pig --provider deepseek --model deepseek-v4-flash --session ./session.jsonl -p "Continue"
```

`OpenSessionManager` 从 header 恢复工作目录，从 model/thinking entry 恢复设置，并把 message 历史交给新 Agent runtime。打开已有 v3 本身不重写历史；v1/v2 会在打开时迁移写回。下一轮只追加新 entry。显式传入一个 Pi v3 Session 文件只授权读写该文件，不触发 Pi 状态、trust 或凭证迁移。

无需文件时使用：

```sh
pig --provider deepseek --model deepseek-v4-flash --no-session -p "Ephemeral request"
```

Go SDK 可把 `NewSessionManager` 或 `OpenSessionManager` 返回值传给 `CreateAgentSessionOptions.SessionManager`。可运行示例：

```sh
go run ./examples/session-persistence
```

最近会话查找、`--continue`、`--resume`、fork 和树修改属于后续 M3 slice，当前仍返回结构化 Capability Stub。

## 对等证据

`parity/oracle/session-persistence.mjs` 在固定 Pi commit `936aff00918de1187f085f123c2812d8f2d67745` 上记录首次写入时机、v3 链、重开追加、内存模式以及空/非法路径行为。`internal/parity/session_persistence_test.go` 在 Pig 公共 `SessionManager` 边界重放同一 Parity Case；SDK 和两个真实 CLI 进程再验证最高公共边界。
