# 外部编辑器的终端所有权

Issue #123 复用 legacy AgentSession、v3 Session 和既有 SettingsManager；外部编辑器只接收本次展开后的草稿，不接收历史、系统提示或 Provider 凭证。对等基线为 Pi `936aff00918de1187f085f123c2812d8f2d67745`。

`InteractiveMode.OpenExternalEditor` 读取 `externalEditor` → `VISUAL` → `EDITOR` → 平台默认值。保留 Pi 的按单个空格拆分规则（包括空参数和原样引号），不经 shell 二次解释。临时目录使用随机名、0700 权限，`prompt.md` 使用 0600；成功读取只移除末尾一个 LF，失败保留原草稿。所有路径尽力删除临时目录。

`tui.TextUI.EditExternally` 暂停输入和渲染，退出全屏、释放键盘协议与 raw mode，并等待旧输入派发完成，再调用外部进程。AgentSession 可以继续生成，事件更新保留在内存，恢复后统一渲染。返回后重新启用输入协议、重建布局并恢复编辑器焦点。终端 generation 隔离旧读取器退出事件。Stop 取消并等待编辑器后才处置终端。

`ProcessTerminal.RunCommand` 继承注入终端的输入输出。darwin/linux 使用独立进程组；有控制终端时将前台交给编辑器，退出后交回原进程组。Context 取消杀死并等待编辑器，清理同组子进程。核心保持无 CGO；其他平台显式返回 NotImplemented，六平台运行验收留在 M13。

Pi 对非零退出和命令不存在静默保留草稿，读写异常会向上抛出。依照 issue 的错误可见与会话保留要求，Pig 将这些失败显示为错误并恢复可用会话；这是明确的 Go 偏离，不声称错误展示完全对等。后台守护进程主动脱离进程组、恶意替换临时路径、存储耗尽的写入中途失败、终端设备故障以及扩展编辑器保留 partial；不新增 Sandbox、扩展宿主或包生态。
