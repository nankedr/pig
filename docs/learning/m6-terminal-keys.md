# M6.3：终端按键与快捷键

本切片对应 #111，Pi 对等基线为 `936aff00918de1187f085f123c2812d8f2d67745`。普通验收重放锁定 fixture，不访问真实 Provider，也不需要 Pi 或 Node。

## 从字节到动作

`ProcessTerminal` 在 raw mode 下读取字节，交给 `StdinBuffer` 组装 UTF-8、CSI、SS3、OSC、DCS、APC 和 bracketed paste。独立 Escape 默认等待 10ms；终端内的其他未完成转义序列等待 150ms，以覆盖 Go 轮询调度与分片粘贴。公开 StdinBuffer 的 `Timeout` 单位为毫秒，默认 10ms。UTF-8 半个字符不会因超时而插入替换字符。

`SetHandlers` 分别接收完整按键和粘贴内容，回调串行运行，回调内不要再次调用同一个 buffer。`Flush` 返回待处理序列，不调用回调；`Clear` 和 `Destroy` 取消计时器并等待当前回调退出。SDK 调用者应在退出时 Destroy。

`ParseKey` 解释键名，`MatchesKey` 按基线匹配别名及修饰键，`DecodePrintableKey` 解码 Kitty/modifyOtherKeys 文本。二者不是简单的等号比较，例如旧式 Ctrl+H 可以同时符合 Backspace 与 Ctrl+H。编辑器沿用 Pi 的动作优先级；release 不编辑、不提交、不触发会话动作，repeat 正常执行。

终端启动发送 `CSI >7u`、`CSI ?u` 和 DA 查询。非零 Kitty flags 启用 Kitty；DA 或零 flags 在尚未启用 Kitty 时选择 modifyOtherKeys。协商响应被消费，不进入草稿；迟到 Kitty 会关闭 fallback。停止和 drain 只弹出本次推入的协议一次。暂停恢复保留草稿并重新协商，读取循环和 buffer 计时器随生命周期收回。

## 用户配置

配置位置为 `$PIG_CODING_AGENT_DIR/keybindings.json`，默认 `~/.pig/agent/keybindings.json`。支持单个键名或数组；缺省使用默认值，空数组解绑。旧式名称（例如 `submit`）迁移到完整名称；若两者同时出现，完整名称优先。无效文件或无效字段按 Pi 行为回退，不写回配置文件。

```json
{
  "tui.input.submit": "ctrl+s",
  "tui.editor.historyPrevious": "ctrl+p",
  "tui.editor.historyNext": "ctrl+n",
  "tui.editor.cursorRight": [],
  "app.clear": "ctrl+r",
  "app.exit": "ctrl+q",
  "app.interrupt": "escape",
  "app.session.new": "ctrl+alt+n"
}
```

多个动作可以显式绑定同一个键。`GetConflicts` 只报告用户显式配置间的冲突，不报告默认值的上下文复用，也不自动挪走其他动作的默认绑定。CLI 启动显示冲突警告；SDK 使用 `GetKeys`、`GetResolvedBindings`、`GetEffectiveConfig` 和 `Reload` 查询或重载。

SDK 的 `EditorOptions.Keybindings` 和 `InteractiveModeOptions.Keybindings` 注入独立 manager。未注入的 Editor 使用 `GetKeybindings` 的全局默认值；`SetKittyProtocolActive` 控制公开纯解析函数的模式，真实终端协商会更新它。多个真实终端的 TextUI 各自读取本实例的协议状态。

```sh
go run ./examples/terminal-keys
go test ./tui -run '111' -count=1
go test ./codingagent ./cmd/pig -run '111' -count=1
```

## 已验证与保留边界

已验证默认编辑、覆盖、解绑、历史、提交、清空、退出、中断活动请求、新建会话及 POSIX 暂停恢复。SDK fixture 覆盖 597 种编码、两种 Kitty 模式、锁定修饰位、键盘布局备用码、数字小键盘和 WezTerm Escape/重复文本。CLI fixture 比较真实 Provider 请求文本，既有多行编辑 fixture 继续通过。真实 PTY 验证协商、停止/重启、raw mode 恢复、SIGTSTP/SIGCONT 与无迟到回调。

macOS 使用 purego v0.8.4 调用 CoreGraphics 的 `CGEventSourceFlagsState`，保持 `CGO_ENABLED=0`。仅 Apple Terminal 的原始 CR 需要该兜底；失败时返回未按 Shift。测试验证归一化和本机系统查询，不声称自动化测试模拟了物理 Shift 按键。

保留 partial：非 UTF-8 单字节高位 Meta 输入（使用 ESC 前缀替代）；Windows VT 输入及原生修饰键、六平台真实终端验收归 M13；无效/罕见转义组合未穷举。模型选择器、思考级别切换、剪贴板、队列、会话树/fork/resume 等 UI 动作只有配置定义，本切片未交付其 UI。自动补全、扩展、图片仍属各自里程碑。

Oracle 重建需要锁定源码和独立 Catalog Snapshot：把 `parity/baseline/catalog/chat/source/providers/` 放入 Pi 的 `packages/ai/src/providers/data/`，把 `source/manifest.json` 放为该目录的 `.manifest.json`，再依次构建包；不改变 tracked 源码。`node parity/oracle/keys.mjs <Pi> --check` 与 `node parity/oracle/keys-cli.mjs <Pi> --check` 复核 fixture。
