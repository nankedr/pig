# M3.3 查找、继续与 fork 本地 Session

本切片实现 Issue #73。先运行 `go run ./examples/session-navigation`，可离线观察发现、继续和独立 fork；示例使用 Faux Provider，不需要凭证。

CLI 可以通过 `--continue` / `-c` 继续当前目录最近的会话，通过 `--session <path|id>` 指定路径、完整 ID 或 ID 前缀，通过 `--fork <path|id>` 复制完整历史到当前目录。`--name` 设置名称；`--session-id` 可指定 fork 的新 ID，已存在的本地 ID 会报错。`--session-dir` 或 `PIG_CODING_AGENT_SESSION_DIR` 覆盖存储位置。

```sh
pig --provider deepseek --model deepseek-v4-flash --continue -p "继续"
pig --provider deepseek --model deepseek-v4-flash --session abc123 -p "检查上次结果"
pig --provider deepseek --model deepseek-v4-flash --fork abc123 --name "另一种方案" -p "尝试另一种实现"
```

ID 查询先查当前目录，再查所有目录；每一层先找精确 ID，再找按列表顺序出现的第一个前缀匹配。跨目录 ID 的 `--session` 会询问是否 fork，拒绝会输出 `Aborted.` 并成功退出。显式路径直接恢复；`--fork` 则明确把副本放在当前目录。TUI 的 `--resume` 选择器仍属于后续阶段；无 TUI 时使用 `--session`。

`ListSessions` 只返回当前工作目录的会话，`ListAllSessions` 默认扫描 Pig sessions 下的各项目目录（包含符号链接）；显式 SessionDir 表示一个平铺目录。损坏或不可读文件不会阻止其他文件被发现，进度计数包含扫描的 `.jsonl` 文件。列表 `Modified` 使用最后一条 user/assistant 消息时间，工具消息和重命名不会提升排序；`ContinueRecentSessionManager` 使用文件 mtime。这是固定 Pi 的两个不同契约。

`SessionManager.Branch` 和 `ResetLeaf` 只移动内存 leaf，下一次追加才把路径写入文件。重新打开时 leaf 是最后落盘的 entry；它不是独立持久化的游标。`GetTree`、`GetChildren` 与 `GetBranch` 可观察所有分支，label 的最新值及时间戳从追加记录恢复。空名称、空 label 表示清除。`BranchWithSummary` 保存调用者提供的摘要及 usage，不调用模型。

`ForkSessionManager` 复制整个文件历史，包括其他分支和未知 JSON 字段。`CreateBranchedSession` 只保留根到所选 leaf 的路径，把路径中的 label 记录移除并重新连接父项，再追加这些 entry 的最新 label 与原 label 时间戳。名称等元数据保留到所选路径为止。持久化副本的 header.parentSession 指向源文件；无 Assistant 的路径推迟首次落盘，完整文件 fork 则立即创建文件。之后写副本不会改变来源。

`AgentSessionRuntime` 通过创建时注入的 factory 实现 `NewSession`、`SwitchSession` 和 `Fork`。fork 默认在 user entry 之前截断，并返回 SelectedText；Position="at" 可保留任意指定 entry。替换先终止并等待旧请求落盘，再执行 BeforeSessionInvalidate、解绑旧 Session、清理其 Provider 资源，创建并重绑新 Session。创建或重绑失败会返回 error，候选 Session 会被释放；旧 Session 已失效，需要调用方处理失败。Go 不回滚已经写出的 fork 文件，也不把错误报告成替换成功。

Go context 取消返回 context error，尚未开始替换时旧 Session 不变。ADR-0009 将 ExtensionHandler 保持为不可执行载体，因此 Pi 的 session_before_switch/session_before_fork 扩展取消、Setup 和 WithSession 回调仍待 M7；非空扩展回调明确报 Stub。M4 的 AgentSession.NavigateTree 和自动分支摘要也未提前实现。

固定 Pi commit 为 `936aff00918de1187f085f123c2812d8f2d67745`。`session-navigation.mjs` 与 `session-tree.mjs` 生成真实 Pi Oracle fixture，Go 在 SessionManager 公开接口重放；SDK 生命周期及 CLI 真实进程测试覆盖更高边界。对等证明不包含尚未可执行的扩展钩子。
