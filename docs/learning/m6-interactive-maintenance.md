# M6：在交互会话中维护当前 Session

先用两轮对话积累上下文，再输入 `/compact 保留关键约束`。界面显示压缩进度，成功后显示摘要；下一次生成使用摘要和保留的近期消息。Escape 可取消；失败或取消不退出会话。压缩会先取消当前生成，Bash 尚在运行时则提示等待。

修改本地模板、Skill、Context File、主题或设置后，输入 `/reload`。进度、诊断和失败会显示在终端；成功后新模板、Skill 命令和自动补全使用新资源。生成期间提示等待。维护期间提交的文字会回填编辑器，可在完成后继续提交；取消和失败保持底层重载的阶段语义，不承诺回滚。

`/session` 显示当前上下文容量和完整历史消息、input/output/cache usage 及 cost。压缩不删除历史账单，重试后的统计仍读取权威 SessionStats。压缩后没有新回复时当前上下文为 unknown，不能将它显示为零。

`/export` 将 HTML 写入默认位置；`/export "report space.html"` 可指定含空格的路径。成功显示目标位置，路径或写入错误会显示并保留会话。导出沿用安全离线模板，不创建父目录，不隐式上传或打开网站；`.jsonl` 与 `/share` 保持 Stub。

## 公开 SDK 与验收

```sh
go run ./examples/interactive-maintenance
CGO_ENABLED=0 go test ./codingagent ./cmd/pig -run 'Maintenance.*124' -count=1
PIG_TEST_RACE=1 go test -race ./codingagent ./cmd/pig -run 'Maintenance.*124' -count=1
make m6-maintenance-oracle PIG_PI_ORACLE_CHECKOUT=/path/to/locked-pi
```

示例使用 Faux、临时 Session 和资源，预置两轮历史与模板，可直接输入四条命令。SDK 用户继续使用 NewInteractiveMode.Run 与已有 AgentSession 方法，不需要新包装接口。API snapshot 位于 `codingagent/testdata/issue124_surface_golden.txt`。

`parity/terminal/maintenance.py` 用同一个真实子进程/PTY harness 运行 Pi 与 Pig，控制 loopback SSE、资源文件与导出文件。普通验收只依赖已锁定 fixture，不要求 Pi 或在线凭证。公开 SDK 补充慢重载的取消、忙输入恢复、失败及 Stop 清理。差异与未覆盖分支见 ADR-0042 和 `contract:codingagent/interactive-maintenance`；不宣称 M6 整体冻结。
