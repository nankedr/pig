# M6：在交互会话执行 Bash

Issue #122 对等基线为 Pi `936aff00918de1187f085f123c2812d8f2d67745`。

启动 `pig`，输入 `!printf 'hello\n'`。`!` 后的非空命令直接交给既有 `AgentSession.ExecuteBash`；`!!printf 'local only\n'` 仍生成 `bashExecution` 历史，但 `ConvertToLLM` 会排除它。空 `!` / `!!` 沿用普通文本路径。生成中的 Alt+Enter 沿用 #116 的字面 follow-up 语义。

stdout/stderr 由已有 BashOperations 合流。运行时显示输出和取消提示，完成后显示非零退出码或取消状态。默认显示末尾 20 个视觉行，展开工具快捷键（默认 Ctrl+O）切换完整可用输出。上下文截断沿用 2000 行 / 50 KiB 限制；提示中的完整输出文件保存未截断内容。恢复 Session 后也使用同一 Bash 组件展示记录。

交互入口同时只允许一个 Bash，重复提交会提示并回填编辑器。Bash 可与生成并行；生成期间完成的结果由 Session 暂存，在本次生成收尾后写入历史，进入之后的新 Provider 请求。Escape 优先取消活动生成；生成结束后再次 Escape 取消 Bash。重试取消与消息队列保留 #116 契约。退出、切换 Session 会取消并等待 Bash 收尾。启动失败仍保留交互展示和错误提示，但不伪造成功的 Session 记录。取消保留部分输出，执行身份防止迟到 Escape 取消下一条命令，已完成组件忽略迟到输出。

不增加 Tool 审批或 Sandbox。Bash 使用宿主权限、既有 shell 路径/命令前缀、工作目录和进程清理能力，即使 `--no-tools` 也可显式运行。

## 公开 SDK 与验证

`NewBashExecutionComponent(command, excludeFromContext...)` 是可独立使用的文本组件。`AppendOutput` 接收增量；`SetComplete` 固定结果并复制指针参数；`SetExpanded` 控制预览。组件不启动终端或定时器，交互层负责刷新。

```sh
go run ./examples/interactive-bash
CGO_ENABLED=0 go test ./codingagent ./cmd/pig -run 122 -count=1
PIG_TEST_RACE=1 go test -race ./codingagent ./cmd/pig -run 122 -count=1
make m6-bash-oracle PIG_PI_ORACLE_CHECKOUT=/path/to/locked-pi
```

`parity/terminal/bash.py` 在同一 PTY harness 中运行真实 Pi/Pig 和受控 loopback SSE，检查输入路由、输出、退出、互斥、取消、截断文件、排队及后续请求。普通验收只读取锁定 `bash-cli.json`，不依赖 Pi 或在线 Provider。Go 公开 SDK/CLI 测试补充进程清理和生产 Session reader。

精确边框/动画、扩展 `user_bash` 拦截（M7）、完整认证（M11）、图片（M12）和六平台运行验收（M13）仍 partial/deferred；包生态 #99 不在本票范围。核心构建保持 CGO_ENABLED=0。
