# grep 的外部工具与取消生命周期

Issue #83 开放 `CreateGrepTool` 与 `CreateGrepToolDefinition`，沿用 ADR-0020 的执行槽和 Agent 权威参数校验。`GrepToolInput.Context/Limit` 及 `GrepToolDetails.MatchLimitReached` 在 M4 冻结前从整数指针改为 `*float64`，保留固定 Pi 的 number、小数上下文和小数匹配限额行为。

grep 优先使用 `GetAgentDir()/bin/rg`，其次探测 PATH 中的 `rg --version`；遵循 Pi 的探测语义，能启动且非零退出的 version probe 仍代表命令存在。搜索参数不经过 shell，保留 `--json --line-number --color=never --hidden` 和 pattern 前的 `--`。本切片使用既有 macOS/Linux 进程组能力；Windows/Termux 运行验收继续遵循 ADR-0011 的 M13 门禁。

有意偏离固定 Pi `ensureTool("rg", true)` 的静默自动下载：本切片不执行自动安装、下载或包管理。缺失时返回安装到 PATH 或 Pig bin 的提示；`PIG_OFFLINE` 启用时明确跳过下载。显式 `--offline` 同样不会引起可选外联，因为该工具没有下载路径。后续自动安装能力必须提供可见进度并服从统一 Offline Mode。本切片在 Parity Catalog 中保留 partial 状态，不把自动安装或六平台验收标为完成。

取消和达到匹配限额会终止 Unix 进程组，等待搜索进程和输出读取收尾。取消错误保留 `context.Cause`，使已有 Agent 生命周期保存错误 ToolResult 后再 settled。相比 Pi 只终止直接子进程并移除 abort listener，Pig 也终止后代进程，并在异步 context 文件读取结束后检查取消。这继承宿主权限，不新增 workspace 限制或逐 Tool 审批。

行截断采用 Pi 的 500 个 UTF-16 单元计数；恰好切断代理对时，Go 的 UTF-8 输出使用 U+FFFD，Oracle 以 UTF-8 可传输文本投影比较。错误 ToolResult 的空 details 对象与 Pi 的 undefined 均按“无 details payload”投影，其余字段精确比较。共享通用截断函数的既有字符计数契约保持原状。
