# M4.2：定位文件，再 read

Issue #84 把 find/ls 从 Capability Stub 接入已有 Agent Tool 调度。调用 `CreateAgentSession` 时设置 `Tools: []string{"ls", "find", "read"}`，即可走列目录 → glob 查找 → read → 后续生成。CLI 使用 `pig --tools ls,find,read -p "找到目标文件并查看内容"`。未选择工具时仍保留 read/bash/edit/write 四工具。

- `ls` 默认当前目录，包含隐藏项，按名称小写后的 English collation 排序；目录后缀 `/`，符号链接跟随目标，无法 stat 的条目跳过。默认最多 500 项、50KB；limit=0 返回 `(empty directory)`。
- `find` 默认调用已安装 fd，默认 1000 个结果、50KB。带 `/` 的 pattern 增加 `--full-path`，相对路径模式按 Pi 补 `**/`；`--` 隔离看起来像命令参数的模式。隐藏文件可匹配，`.gitignore` 交给 fd 分层处理；仓库外增加 `--no-require-git`，仓库内保留 Git 边界。
- find 保持 fd 或自定义 Operations 的顺序，不额外排序；自定义 Glob 负责限制数量。恰好达到 limit 时 find 提示触顶；ls 只有循环遇到更多条目才提示。字节截断和数量截断可以同时出现，details 保留原始计数。
- 路径复用 read/write 的 host path 解析，可用相对路径、绝对路径、`~`、file URL、`@` 和空白规范化。没有新增工作区路径沙箱。

实现入口是 `codingagent/find_tool.go` 和 `codingagent/ls_tool.go`。两个 definition 共享 Tool 执行体，通过 Agent dispatcher 校验参数并将错误送回模型；自定义 FindOperations/LsOperations 可连接远程文件系统。context 取消会阻止后续操作，fd 探测与搜索的进程组被终止，直接子进程回收后返回。同步自定义操作需主动响应 context。

运行示例前，显式安装 fd，并放入 PATH（Linux 也支持 fdfind）或 `GetAgentDir()/bin`。工具调用不会自动安装或下载它：

```sh
go run ./examples/find-ls-read
go test -race ./codingagent -run '^TestFindLs' -count=1
go test ./cmd/pig -run '^TestPigFindLsReadContinuation$' -count=1
node --experimental-strip-types parity/oracle/find-ls.mjs <locked-pi-checkout> --check
```

正式实目录 Oracle 和 CLI 用例要求 PATH 中有 fd；不存在时会明确 Skip，不把缺工具当作对等通过。其余平台脚本、自定义 Operations、ls 实目录测试保持纯 Go/offline。Oracle 也要求提前准备 Pi 依赖和 fd，脚本禁止自动下载。当前证据来自 darwin-arm64，Pi 固定提交为 `936aff00918de1187f085f123c2812d8f2d67745`。先读 Pi tools 测试及 #3302、#3303、#6104 回归，再建立 fixture；用相同 SDK Case 在原分支基线运行得到 `CreateLsTool: not implemented`，实现后通过。

兼容分支的限制见 [ADR-0022](../adr/0022-find-ls-platform-contract.md)：自定义 FindGlobOptions 的 limit 仍为 int，默认本机 fd 执行目前支持 darwin/linux，其余平台明确 Stub；其他 locale 排序和六平台运行验证待 M13，交互渲染与扩展宿主待 M7。Pi 的 fd 行式输出会截断换行文件名、trim 名称两端空白；不会声称这些名称可可靠回读。公开能力状态与复现证据登记于 `contract:codingagent/find-ls`。
