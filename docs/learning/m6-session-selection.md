# M6.10：选择、恢复与切换 Session

Issue #118 的固定对等基线是 Pi `936aff00918de1187f085f123c2812d8f2d67745`。本切片复用 legacy AgentSession、v3 SessionManager 与 AgentSessionRuntime，不新增 Session 存储格式。

## 使用

在真实终端运行 `pig --resume`（或 `pig -r`），搜索并选择历史 Session。启动取消会正常退出且不新建文件。进入会话后，`/resume` 打开相同选择器，取消保留当前 Session、编辑器草稿及仍在运行的生成；`/new` 新建 Session。`/name 新名称` 修改当前会话名称。

选择器从当前 CWD 的真实发现结果开始，Tab 切换到全部项目；显式 `--session-dir` 将全部范围限制在该目录。输入支持多词模糊搜索、`"完整短语"` 与 `re:正则`，匹配 ID、名称、全部消息和 CWD。Ctrl+S 在按分支、最近和相关性之间循环；Ctrl+N 只显示有名称的 Session；Ctrl+P 显示文件路径。上下键和翻页键在边界停止。按分支排序以子树最新活动排列。

会话内 Ctrl+R 重命名，Ctrl+D 或空查询时 Ctrl+Backspace 进入删除确认，Enter 确认、Esc 取消。禁止删除当前 Session。启动选择器不提供重命名。重命名当前 Session 复用配置事务，生成期间拒绝修改。普通会话的删除直接删除所选文件，详见范围表。

## 生命周期

选定目标后，InteractiveMode 取消并等待当前生成返回，RunHeadless 解除该轮监听，随后调用 AgentSessionRuntime.SwitchSession。runtime 先成功构建目标才销毁旧 Session；读取或构建失败后旧 Session 仍可继续工作。目标文件必须存在、非空、为普通文件，且 JSON 完整。缺失 CWD 显示错误，保留当前 Session。

切换后，模型、thinking、名称、模型历史、编辑器历史、资源补全和后续持久化指向目标。CLI 显式模型/thinking 参数仍保持现有优先级。跨 CWD 创建目标前，复用 #114 的项目信任判断并在当前 TUI 显示信任对话框；拒绝或 Esc 不读取项目设置，Context File 仍会加载。公开 SDK 自定义 runtime factory 继续负责自己的资源与信任装配。

```go
current := func(ctx context.Context) ([]codingagent.SessionInfo, error) {
    return codingagent.ListSessions(ctx, cwd)
}
all := func(ctx context.Context) ([]codingagent.SessionInfo, error) {
    return codingagent.ListAllSessions(ctx)
}
selector, err := codingagent.NewSessionSelectorComponent(ctx, current, all,
    func(path string) { /* 关闭选择器后调用 runtime.SwitchSession */ }, cancel)
```

构造器同步读取初始列表并返回加载错误；后续范围加载和管理错误在选择器内显示。完整可运行示例：`go run ./examples/session-selection`。公开 InteractiveMode 接管 runtime 和终端的最终释放。

## 验证与范围

| 行为 | 状态与证据 |
| --- | --- |
| 搜索、筛选、分支/最近/相关性排序、范围和取消 | 固定 Pi 公开组件 Oracle + Go SDK `TestSessionSelectorParity118` |
| 启动恢复、会话内取消、新建、切换、生成、保存、重开 | 同一真实进程 PTY harness 对比 Pi/Pig 请求、v3 用户历史、名称和终端恢复 |
| 选择后配置恢复、坏路径、工厂失败、管理动作 | 公开 SDK + Faux，`issue118_sessions_test.go` |
| 生成中取消选择与跨 CWD 切换、旧资源释放、拒绝信任 | 公开 Interactive SDK + PTY + 受控 SSE + 未信任 FIFO，race 检查 |
| 正则 | Go RE2 支持的模式；JS lookaround/backreference 等扩展未覆盖，保持 partial |
| 删除 | 确认后直接 unlink；Pi 优先调用外部 `trash` 的分支未实现，保持 partial |
| 缺失 CWD | 可见错误并可继续工作；Pi 的改用当前 CWD 对话框未实现，保持 partial |
| 列表加载 | 同步加载；Pi 的异步进度与并发加载时序未实现，保持 partial |
| 展示与扩展 | 精确树线/相对时间/像素布局、扩展回调 M7、认证 M11、图片 M12、六平台 M13 保持既定边界；包生态 #99 不阻塞 |

终端 harness 比较所有 termios 设置，忽略 macOS 的瞬时 PENDIN（内核等待重新显示输入）状态位，不忽略 raw/echo 等模式。普通验收只用本地受控服务；Oracle 更新需要固定 Pi checkout。

```sh
CGO_ENABLED=0 go build ./...
go test -race ./codingagent -run 118 -count=1
PIG_TEST_RACE=1 go test -race ./cmd/pig -run 118 -count=1
node parity/oracle/session-selector.mjs /path/to/locked-pi --check
node parity/oracle/session-selection-cli.mjs /path/to/locked-pi --check
```
