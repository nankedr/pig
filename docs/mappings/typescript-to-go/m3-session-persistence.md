# M3.1 v3 Session TypeScript 到 Go 导航

本页只导航固定 Pi 源码与 Pig 实现；能力状态和证据以 `parity/catalog.jsonl` 为准。

| Pi 来源 | Pig 实现 | 责任 | 主要证据 |
| --- | --- | --- | --- |
| `core/session-manager.ts#SessionManager.create` | `codingagent.NewSessionManager` | 新 Session、ID、Pig 默认/显式目录和延迟建文件 | `internal/parity/session_persistence_test.go` |
| `core/session-manager.ts#SessionManager.open` | `codingagent.OpenSessionManager` | 显式路径打开、header/CWD/历史恢复、空文件初始化 | `codingagent/issue71_session_persistence_test.go` |
| `core/session-manager.ts#append*`、`_persist` | `SessionManager.AppendMessage`、`AppendModelChange`、`AppendThinkingLevelChange` | v3 JSONL parent chain、首次排他创建和后续追加 | `parity/oracle/fixtures/session-persistence.json` |
| `core/sdk.ts#createAgentSession` | `codingagent.CreateAgentSession` | 恢复消息与 thinking，把新 model/thinking 元数据写入持久化 Session | `codingagent/issue71_session_persistence_test.go` |
| `main.ts` 的 Session 选择 | `codingagent.Main`、`CreateHeadlessSession` | 默认持久化、`--session`、`--session-dir`、`--session-id`、`--no-session` | `cmd/pig/issue71_process_test.go` |
| Pi Agent 目录布局 | `codingagent.GetAgentDir` | 采用 Pig `~/.pig/agent` 与 `PIG_*`，不迁移 Pi 状态 | `cmd/pig/issue71_process_test.go` |

Pi 的随机 ID 和时间戳在 Oracle 中只投影结构；文件出现时机、记录类型、顺序、parent 关系、重开后的追加行数和副作用不做归一化。生产 v3 Session 与 `agent` 包的 Harness v4 Session 保持两个边界。
