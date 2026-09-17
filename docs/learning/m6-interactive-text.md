# M6.1 最小 Interactive 文本对话

对应 [#109](https://github.com/nankedr/pig/issues/109)，固定 Pi commit `936aff00918de1187f085f123c2812d8f2d67745`。此切片开启真实终端入口，M6 整体仍为 partial。

## 从终端运行

```sh
go run ./examples/interactive-text
```

在真实 TTY 中输入任意文本并回车，再输入第二轮文本。示例仅使用 Faux，不需要凭证或网络；两次回复预先锁定。空编辑区按 Ctrl+D、500ms 内两次 Ctrl+C 或提交 `/quit` 退出。第一次 Ctrl+C 清空编辑区。支持基本 UTF-8 输入、退格和 bracketed paste；粘贴换行不自动提交。

实际 Provider 使用已有模型与凭证配置：

```sh
go build -o ./pig ./cmd/pig
./pig --provider deepseek --model deepseek-v4-flash --no-tools
./pig --continue --provider deepseek --model deepseek-v4-flash --no-tools
```

交互式 CLI 复用现有设置、项目信任、本地资源、AgentSession 与 v3 Session。回复增量进入消息区，下一轮沿用历史；Provider 错误可见，随后可以继续输入。只有 stdin/stdout 都是终端且没有 `--print`、`--mode json` 或 `--mode rpc` 时进入 Interactive。非 TTY、Headless text/json 和 JSONL RPC 保持原路由。

## SDK 生命周期

`NewInteractiveMode(runtime, options)` 不触碰终端。`options.Terminal` 可注入 `tui.Terminal`，缺省使用 `tui.NewProcessTerminal(os.Stdin, os.Stdout)`。`Init(ctx)` 启动终端；`Run(ctx)` 会自动 Init，展示历史并读取输入，运行至退出或取消；`Stop()` 幂等停止。

Interactive 接管传入 runtime 的结束清理；初始化失败、Run 返回或 Stop 都会 Dispose。单独调用 Init 的应用应 `defer mode.Stop()`。一个 mode 只运行一次，禁止并发 Run。SDK context 取消返回 `context.Canceled`；CLI 的 SIGTERM/SIGHUP 按 Pi fixture 正常清理并退出 0。外部进程发送的 SIGINT 与键盘 Ctrl+C 不同：Pig 清理后恢复默认处理并重发 SIGINT，保留真实信号退出；Headless 继续使用既有 130。Pi 此路径会留下 raw mode，fixture 保留这一观察；Pig 按 #109 的恢复要求清理，清理语义差异明确保留 partial。

`codingagent` 只负责提示词和 Session 事件编排。输入解析、终端读循环、resize、屏幕输出和 raw mode 恢复属于 `tui`。ProcessTerminal 保留调用方文件所有权，停止时等待输入循环退出，再恢复模式、光标和粘贴状态。注入终端可通过可选 `Done() <-chan struct{}` / `Err() error` 报告 EOF 或读取失败。

## 复核证据

```sh
go test ./cmd/pig -run '^TestPigInteractive' -count=1
go test -race ./codingagent -run '^TestInteractiveSDK' -count=1
go test ./codingagent -run '^TestIssue109InteractiveAPISnapshot$' -count=1
```

普通验收为 Go 测试，使用真实 PTY 子进程、回环 SSE 服务和 Faux；不读取 Pi、不安装 Node、不连接在线 Provider。SSE 在首段输出后等待终端观察再继续，从而证明流式显示而非只检查最终文本。CLI 测试同时比较退出码、终端模式、光标、粘贴模式、两次请求和通过公开 Session API 重开的消息角色。

`parity/oracle/fixtures/interactive.json` 来自 Pi 的真实 CLI 子进程与 PTY。显式重新提取需要已准备且 tracked clean 的锁定 Pi checkout、Node 与 Python：

```sh
node parity/oracle/interactive.mjs /path/to/locked-pi --check
```

Parity Catalog 行 `contract:codingagent/interactive-text` 登记证据和未覆盖范围，API 快照在 `codingagent/testdata/issue109_surface_golden.txt`。

## 本票边界

当前硬门为 darwin 本机，darwin/linux 具有无 CGO 终端实现，其他平台启动返回明确 Stub，六平台验收留待 M13。完整 Markdown、编辑器光标移动与 grapheme、scrollback、复杂布局、fullscreen、Escape/挂起和 Kitty 协议没有声明对等；图片、扩展运行时和更新通知维持既有 Stub。终端消失或输出长期阻塞的 emergency exit 尚未验收。包生态 #99、M7、M11、M12 不在本次范围，父 issue #8 不作修改。
