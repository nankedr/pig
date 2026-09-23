# M6：用外部编辑器修改草稿

在 Pig 输入草稿，按 `Ctrl+G` 进入外部编辑器，保存退出后继续编辑或按 Enter 提交。多行中文、emoji 和大段粘贴会作为完整草稿传入；历史消息不会写入临时文件。取消或失败保留原稿，并显示错误。

在用户 settings.json 中配置 `"externalEditor": "vim -Nu NONE -n"`。未配置时依次读取 `VISUAL`、`EDITOR`，POSIX 默认 `nano`。与固定 Pi 基线一致，命令按空格拆分，引号不会形成 shell 分组，路径含空格时应使用无空格的包装脚本；GUI 编辑器需要其等待退出参数。Windows 默认值仍为 `notepad`，当前执行适配明确未实现，留待 M13。

保存时仅去掉末尾一个换行；正常提交继续遵循已有编辑器的空白处理。外部编辑器拥有终端期间，生成可以继续，Pig 暂存界面更新；回到 Pig 后可看到最新会话内容。终端尺寸变化会触发布局重建。

## 公开 SDK

`InteractiveMode.OpenExternalEditor(ctx)` 适用于已 Init 的会话，Run 和 GetUserInput 也处理同一个快捷键。默认终端或注入的 `tui.ProcessTerminal` 都可使用。自定义 Terminal 需另外实现 `RunCommand(context.Context, string, ...string) error`；不支持时返回错误。

`tui.TextUI.EditExternally(ctx, edit)` 是终端交接与草稿替换边界；回调收到完整草稿，只在返回成功时替换。`ProcessTerminal.RunCommand` 要求先停止输入读取，继承对应终端流。Stop 取消并等待运行中的编辑器，编辑器不会成为孤立的后台任务。

```sh
# 离线 Faux Provider，需交互终端；输入草稿后 Ctrl+G，保存后 Enter，Ctrl+D 退出
EDITOR='vim -Nu NONE -n' go run ./examples/external-editor

go test ./codingagent ./cmd/pig -run 'ExternalEditor.*123' -count=1
PIG_TEST_RACE=1 go test -race ./codingagent ./cmd/pig -run 'ExternalEditor.*123' -count=1
make m6-external-editor-oracle PIG_PI_ORACLE_CHECKOUT=/path/to/locked-pi
```

`parity/terminal/external-editor.py` 使用真实 CLI、控制终端和离线 loopback Provider，比对初始草稿、空参数/引号、成功提交、退出 7/130、命令不存在、后台生成隔离、文件清理与终端状态。SDK 补充环境选择、大段粘贴、文件创建/读取失败、Context/Stop 取消和同组子进程清理。

本机 darwin-arm64 的真实编辑器往返可复现：

```sh
go build -o /tmp/pig-external-editor ./cmd/pig
python3 parity/terminal/external-editor.py /tmp/pig-external-editor --pig --vim --fullscreen --failures
```

本次使用 `/usr/bin/vim` 完成编辑、保存、Provider 提交与恢复，结果见 `docs/verification/issue123-external-editor.md`。对等目录保留 partial：扩展编辑器 M7、完整认证 M11、图片 M12、六平台 M13、写入中途存储故障与终端设备故障尚未覆盖；包生态 #99 不阻塞本次交付。
