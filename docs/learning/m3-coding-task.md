# M3.11：默认四工具与可恢复编码任务

默认本地 Coding Agent 现在启用 `read/bash/edit/write`，CLI、Headless SDK、Services SDK 和注入 Provider 的 Session 使用同一装配函数。`CreateCodingTools(cwd, options...)` 也可以单独生成四工具；显式 `AgentTools` 继续覆盖 SDK 的工具来源。

```sh
go run ./examples/coding-task
go run ./cmd/pig --provider deepseek --model deepseek-v4-flash -p '创建 hello.go，读取并修改，然后运行它'
go run ./cmd/pig --continue --mode json -p '继续上一项工作'
go run ./cmd/pig --fork <session-id-or-file> -p '从已有历史尝试另一种实现'
```

示例使用离线 Faux Provider，在临时目录实际执行四工具、关闭并重开 Session、fork 后继续，结束时清理演示文件。真实 Provider 命令需要已配置凭证。

`--tools write,read` 显式选择工具，`--exclude-tools bash` 最后排除，`--no-tools` 或 `--no-builtin-tools` 在没有显式选择时禁用默认工具。SDK 的 `Tools: []string{}` 表示空选择；nil 表示默认集合。工具说明和使用建议仅包含最终启用的工具。注入 Provider 的 M1/M2 SDK 路径默认使用内存 settings 与内存 Session；除模型明确调用的文件或 shell 工具外，不产生配置或 Session 持久化副作用。

可信 settings 的 `shellPath`、`shellCommandPrefix` 传入 Bash。拒绝 Project Trust 会忽略项目 settings，仍可使用全局设置和内建工具，因为信任门不是 Tool 审批或 Sandbox。启动时装配 Provider 的 `retry.provider` 重试、超时和最大重试等待参数；未设置 timeout 时使用 `httpIdleTimeoutMs`，该值为 0 时映射为 2147483647ms。Agent 的 transport、steering/follow-up queue mode 和 thinking budgets 同样来自 settings。尚未实现的图片处理、WebSocket 等行为不因此开放。

新建、重开和 fork 均以 SessionManager 身份创建 Agent。每次模型调用保留 SessionID，继续显式关闭 Provider 缓存；Chat Completions 的 `CacheRetentionNone` 分支接受身份而不发送缓存键或 affinity header，与固定 Pi 的禁缓存行为一致。Bash 从当前 Session 读取 `PIG_SESSION_ID`、可用时的 `PIG_SESSION_FILE`、Provider/model/thinking；内存 Session 没有文件变量。

JSON 模式先输出本次 Session header，再输出本轮事件，不重播历史。成功与失败 ToolResult 都由现有 Session 事件桥落盘。Bash 非零退出交给模型继续处理；SIGINT、SIGTERM、SIGHUP 取消进程树后退出 130，错误 ToolResult 可重开继续。恢复只追加新消息；fork 复制选定历史，后续追加不会修改源 Session。

固定 Pi commit `936aff00918de1187f085f123c2812d8f2d67745` 的 `sdk.ts`、`tools/index.ts`、`sdk-session-manager.test.ts`、`sdk-stream-options.test.ts`、`agent-session-dynamic-tools.test.ts` 和排除工具回归定义了本次公开边界。先运行 Oracle 建立选择与提示词 fixture，Go Parity Case 在旧实现上失败，再接通默认装配。真实 CLI 进一步验证创建→读取→编辑→执行→失败结果→退出→恢复→fork，以及取消后重开。

```sh
node --experimental-strip-types parity/oracle/coding-tools.mjs .upstream/pi --check
go test -race ./codingagent -run '^TestDefaultCodingTools' -count=1
go test ./cmd/pig -run '^(TestPigDefaultCodingTaskResumeAndFork|TestPigBashShutdownSignalsKillProcessTree)$' -count=1
go test ./ai -run '^TestOpenAICompletionsSessionIdentityWithoutCache$' -count=1
```

证据归属 `contract:codingagent/default-coding-tools`；API 快照见 `codingagent/testdata/issue81_surface_golden.txt`。grep/find/ls、RPC、压缩、AgentSession 编排式树导航、Provider 缓存与六平台行为验收继续由后续里程碑负责。本切片不单独宣称 M3 冻结完成。
