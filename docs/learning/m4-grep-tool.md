# M4.1：搜索内容并继续编码任务

本切片实现 Issue #83。安装 ripgrep，使 `rg --version` 可运行，然后执行不需要 Provider 凭证或网络的示例：

```sh
go run ./examples/grep-edit
```

示例创建真实 Go 文件，由确定性 Provider 请求 grep，读取带文件名和行号的 ToolResult，再发出 edit 调用并继续回复。CLI 使用相同的工具装配和参数校验：

```sh
go run ./cmd/pig --provider deepseek --model deepseek-v4-flash \
  --tools grep,read,edit --mode json -p '查找 Greeting 的定义，读取文件后修改问候语'
```

默认工具仍为 read/bash/edit/write；使用 `--tools` 显式启用 grep。SDK 对应 `CreateAgentSessionOptions{Tools: []string{"grep", "edit"}}`。需要替换 stat 或 context 文件读取时，注入 `CreateGrepTool(cwd, GrepToolOptions{Operations: ...})` 返回的 `AgentTools`。`GrepOperations` 不替换搜索引擎，搜索仍通过宿主 ripgrep 执行。

`pattern` 支持正则，`literal` 改为字面匹配，`ignoreCase` 忽略大小写，`glob` 筛选文件。`path` 默认为当前目录，支持单文件、绝对路径、父目录、家目录、file URL 和显式符号链接路径。搜索包含隐藏文件，并沿用 ripgrep 的 `.gitignore`/`.ignore`/全局忽略和配置环境语义；显式指定文件时，ripgrep 可以搜索被忽略的文件。项目信任不形成文件路径边界。

结果示例：

```text
example.txt-1- before
example.txt:2: match one
example.txt-3- after

[1 matches limit reached. Use limit=2 for more, or refine pattern]
```

`context` 为匹配前后行数，重叠的上下文会重复输出。`limit` 默认 100，最小为 1；到达限额即标记截断，即使它恰好等于全部匹配数。这两个参数沿用 Pi 的 number 语义，公开 Go 字段用 `*float64` 保留小数，小数 context 可能产生空白的小数行号。无匹配是成功的 `No matches found`；非法正则、路径或进程失败形成错误 ToolResult，模型可以读取错误后继续任务。

每行最多 500 个 UTF-16 单元，总输出最多 50KB；details 包含实际触发的 `matchLimitReached`、`linesTruncated`、`truncation`。context 的内容通过 `GrepOperations.ReadFile` 在进程退出后读取，并按文件缓存；读取失败显示 `(unable to read file)`。取消保留 Go context 原因，终止进程组并等待清理后再结束会话。

外部工具优先来自 Pig 的 `bin/rg`，然后来自 PATH。缺失时提供安装提示，不静默下载；Offline 下也不会下载。自动安装、Windows/Termux 的执行与六平台验收、交互式渲染和扩展宿主仍未实现，见 [ADR-0022](../adr/0022-grep-external-tool-lifecycle.md)。

对等证据归属 `contract:codingagent/grep-tool`：

- `parity/oracle/grep-tool.mjs` 从 commit `936aff00918de1187f085f123c2812d8f2d67745` 的 Pi 实现生成 fixture；可用 `--check` 复核。
- `TestGrepToolSessionParity` 在公开 SDK 会话和 definition 上比较同一批真实文件、Unicode、边界和上下文用例。
- `TestGrepToolSessionProcessCleanup` 验证搜索、version probe 的取消及限额清理；`TestGrepToolExternalProcessContract` 检查参数、退出码和缺失路径。
- `TestPigGrepEditContinuation` 验证 text/json CLI 的错误恢复、搜索、编辑与后续生成。
- API 快照：`codingagent/testdata/issue83_surface_golden.txt`；导航见 [TypeScript → Go](../mappings/typescript-to-go/m4-grep-tool.md)。
