# M6.2 多行编辑与历史输入

对应 #110，Pi 对等基线为 `936aff00918de1187f085f123c2812d8f2d67745`。
`TextUI` 与 SDK 共用 `tui.Editor`；AgentSession、v3 Session、本地资源和 Provider 调用仍由既有 InteractiveMode 管理。

## 运行

```sh
go run ./examples/multiline-editor  # 无终端、网络、凭证的公开 SDK 示例
go run ./examples/interactive-text # 真实终端，两轮 Faux 回复
```

终端中 Enter 提交，Ctrl+J、Alt+Enter 或 Shift+Enter 换行；普通 Enter 前的反斜杠也可换行。
Ctrl+C 清空草稿，空编辑区 Ctrl+D 退出。历史上键先移动到第一视觉行/行首，再回取历史；下键恢复原草稿。恢复 Session 的用户消息也进入输入历史。

## 已验证的编辑动作

| 动作 | 默认键或 SDK | 范围 |
| --- | --- | --- |
| 插入、读写、清空 | `SetText`、`GetText`、`InsertTextAtCursor`、`SetText("")` | CRLF/CR 归一为 LF；Tab 展开为四空格；程序化写入可撤销 |
| 左右导航 | ←/→、Ctrl+B/F | 按字素移动，粘贴占位符为原子单位 |
| 单词导航 | Alt+B/F、Alt/Control+←/→ | 拉丁字母、数字、空白、标点及粘贴占位符；词典语言分词见 partial |
| 行首/尾、上下、分页 | Home/End、Ctrl+A/E、↑/↓、PageUp/Down | 逻辑行首尾、折行后的视觉行、粘性列 |
| 字符跳转 | Ctrl+] / Alt+Ctrl+] 后输入字符 | 向前/后跨行查找 |
| 删除 | Backspace、Delete、Ctrl+D | 中文、组合字符、emoji/ZWJ/国旗字素；行边界合并 |
| kill/yank | Ctrl+W/U/K、Alt+Backspace/D/Delete、Ctrl+Y、Alt+Y | 向前/后删除词或行；连续 kill 合并；yank-pop 循环 |
| 撤销 | Ctrl+-（传统终端 `0x1f`） | 连续输入按词合并；粘贴与程序化插入为原子操作 |
| 历史 | `AddToHistory`、↑/↓ | 去首尾空白、连续去重、最多 100 条、边界不越界、恢复草稿 |
| 焦点与回调 | `SetFocusState`、`SetOnChange`、`SetOnSubmit`、`DisableSubmit` | 聚焦渲染光标标记；提交清空编辑器并回调展开后的文本 |
| 粘贴 | bracketed paste | 分包缓存，粘贴中的换行与控制字符不会调用提交/退出 |

超过 10 行或 1000 个 UTF-16 单元的粘贴显示为 `[paste #N …]`。`GetText()` 返回显示文本，`GetExpandedText()` 返回展开文本，Enter 提交展开文本并去除首尾空白。用展开文本调用 `SetText` 可进入全文编辑。删除占位符可通过一次撤销恢复内容与注册表。

`EditorCursor.Col` 与 `TextChunk` 索引是 **UTF-8 字节偏移**，不是终端列数。Oracle 明确把 Pi UTF-16 索引投影为 Go 字节索引；终端光标使用字素显示宽度。换行、空行、宽字符和长词经同一布局函数折行，窗口变化只改变显示，不重写文本。

编辑器是同步组件，调用方应串行调用；`TextUI` 在终端事件、渲染和清空之间持锁。回调同步执行，组件构造和设置回调不触发终端操作。

## 证据与 partial

```sh
go test ./tui -run '^TestEditorParity110$' -count=1
go test ./cmd/pig -run '^TestPigMultilineEditor110$' -count=1
go test -race ./tui ./codingagent -run 'Test(Editor|InteractiveSDK)' -count=1
node parity/oracle/editor.mjs /path/to/locked-pi --check
node parity/oracle/editor-cli.mjs /path/to/locked-pi --check
```

普通测试离线重放锁定 fixture，CLI 使用真实 pig 子进程、PTY 与回环 Provider，直接比较每轮 Provider 收到的文本；同时覆盖空输入、取消、分段粘贴与缩放。SDK fixture 比较每一步文本、光标、展开结果、完整渲染帧与回调序列。快照在 `tui/testdata/issue31_surface_golden.txt`，目录条目为 `contract:tui/multiline-editor`。

此切片保持 partial：ICU 的中文/泰语等词典分词、不同 Unicode 版本与终端宽度差异、完整 Kitty/modifyOtherKeys 协议、可配置键绑定、自动补全、复杂粘贴占位符跨视觉行的所有跳转组合仍未声明全面对等。自动补全 setter 保留明确 Stub。完整 TUI/Markdown、scrollback、fullscreen、图片、扩展运行时、认证、包生态及六平台运行验收沿用后续里程碑边界。
