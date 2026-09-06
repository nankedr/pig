# M3.10：执行宿主 shell 并保留输出

模型 ToolCall 经 Agent 参数校验进入 bash，stdout/stderr 产生累计尾部 update，执行结束后进入 ToolResult，再交给模型 continuation。`CreateBashTool` 与 `CreateBashToolDefinition` 共用执行体；`CreateLocalBashOperations` 提供可替换的本地执行后端。definition 的 Execute 沿用 ADR-0020 的内建 Go 回调，不引入扩展宿主 ABI。

离线示例会实际运行宿主 shell，以 Faux Provider 驱动 bash → read → continuation；无需网络或凭证：

```sh
go run ./examples/bash-read
```

Headless 显式选择 bash，默认仍只有 read：

```sh
go run ./cmd/pig --provider deepseek --model deepseek-v4-flash --tools bash,read -p '执行 pwd，然后检查当前目录'
go run ./cmd/pig --provider deepseek --model deepseek-v4-flash --tools bash,read --mode json -p '生成长输出，再用 read 查看开头'
```

SDK 通过 `AgentTools` 注入 `CreateBashTool(cwd, BashToolOptions{...})`。ShellPath 优先；Unix 默认依次尝试 `/bin/bash`、PATH 中 bash、sh，使用 `-c`。CommandPrefix 以换行拼接到命令前；SpawnHook 最后修改 command/cwd/env。Headless 使用完成项目信任处理后的 shellPath 与 shellCommandPrefix 设置。命令、路径及权限均遵循宿主语义，Project Trust 不是命令审批或 sandbox。

环境继承宿主变量，在 PATH 前加入 Pig agent 目录下的 bin（不重复）。每次 Session 执行注入当前 `PIG_SESSION_ID`、可用时的 `PIG_SESSION_FILE`、`PIG_PROVIDER`、`PIG_MODEL`、`PIG_REASONING_LEVEL`，覆盖或清除从父进程继承的旧 Session 元数据。直接 definition/Tool 调用无 Session 时不注入这些值；ExposeSessionEnvironment=false 可关闭。Pig 不生成 `PI_*` 身份变量，用户已有的其他环境变量仍保留。

默认没有 timeout。显式 timeout 是正的有限秒数，最大 2147483.647。非零退出码成为含输出与 `Command exited with code N` 的错误；timeout/取消保留已有输出，并附状态。Abort/Dispose 等待在途执行收尾，已接纳 update 的 listener barrier 完成后才发布 final。Unix CLI 的 SIGINT、SIGTERM、SIGHUP 通过取消上下文终止进程组；被信号杀死的命令遵循 Pi 的 null exit code 表示。单独的 AgentSession.ExecuteBash 用户命令入口仍是后续能力。

尾部预览限制为 2000 行或 50KB，以先到的限制为准；显示解码与 Pi TextDecoder 一致：移除开头 BOM，残缺 UTF-8 子序列使用单个替代字符；完整文件仍保留原始字节。末尾换行不额外计行，超长末行保留 UTF-8 完整字符的尾部。流式更新约每 100ms 合并一次，第一条 update 为空，结束前刷出最后累计快照。shell 退出后仍有输出的继承管道继续读取，每次数据重置 100ms 空闲等待；安静的继承管道不会让执行永远挂住。执行返回后的迟到 operations 回调被忽略。

完整原始 stdout/stderr 在超限时写入 OS 临时目录的 `pig-bash-*.log`，预览内存保持有界。成功结果的 details 和文本提供路径；非零退出、超时和取消的错误文本同样保留截断提示与路径。可用 read 的 offset/limit 分页查看。文件在命令退出、CLI 退出及 Session 清理后仍存在，不注册自动删除；示例也会留下文件并打印路径。用户自行管理这些文件，OS 临时目录仍可能被系统清理。

本切片硬门为本机 darwin-arm64。进程配置、终止与默认 shell 选择位于平台文件；Linux 提供同一 Unix 实现但未宣称跨平台行为验收，其他平台返回显式未实现错误。Windows/Termux 与六平台门留到 M13。

```sh
node --experimental-strip-types parity/oracle/bash-tool.mjs <locked-pi-checkout> --check
go test -race ./codingagent -run '^TestBashTool' -count=1
go test ./cmd/pig -run '^TestPigBash' -count=1
```

Pi Oracle 固定 commit 为 936aff00918de1187f085f123c2812d8f2d67745，首个失败 Parity Case 经真实 AgentSession 重放命令、空输出、非零退出和 timeout。补充测试依据 Pi tools.test.ts、#5208/#5303 回归行为，通过 SDK/definition/CLI 验证 UTF-8、行/字节截断、流式 barrier、环境、信号和文件保留。能力与证据归属 `contract:codingagent/bash-tool`。
