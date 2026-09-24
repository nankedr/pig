# M6 真实终端人工验收

当前状态：**按用户 2026-09-24 的决定暂时跳过，未执行，非通过**。PTY 自动回放不代表 GUI 终端、物理按键或 IME 已验收。以下步骤保留供后续执行；#125 和父 #8 已同步自动验收进度，用户授权其余冻结与发布验证完成后关闭两张 issue；发布豁免记录不替代以下人工观察。

在 darwin/arm64 上选择候选 commit，保持 checkout 干净，用 `CGO_ENABLED=0 go build -trimpath -o /tmp/pig-m6 ./cmd/pig` 构建。记录 `git rev-parse HEAD`、`go version`、`sw_vers`、`uname -m`、终端应用名称和版本、字体、窗口尺寸、TERM/COLORTERM 及键盘布局。不记录真实凭证。

在真实终端启动离线受控服务；新目录会保留 Session、设置、导出等结果：

```sh
script -q /tmp/m6-terminal.txt python3 scripts/m6-manual.py /tmp/pig-m6 /tmp/m6-manual-regular
script -q /tmp/m6-fullscreen.txt python3 scripts/m6-manual.py /tmp/pig-m6 /tmp/m6-manual-fullscreen --fullscreen
```

服务只监听 127.0.0.1，使用 synthetic key，逐行返回中文和 emoji，方便在生成期间插话、取消和浏览。它不是 Provider 实现证据。录屏/截图应包含操作和结果；记录会话目录中的 Session 和 `exit.json`，也保存退出前后的 `stty -g`。命令会执行宿主 Bash/外部编辑器，使用专门临时目录。

| 证据 ID | 操作与通过条件 |
| --- | --- |
| editing-paste | 输入中文、emoji、多行；移动/删除/撤销；括号粘贴不意外提交；发送内容与 Session 一致。 |
| keys-legacy | 在 legacy 模式下验证方向键、Home/End、Ctrl 组合、Esc、Enter；记录终端配置与实际输入。 |
| keys-kitty | 在支持 Kitty keyboard protocol 的终端启用协议，验证实际协商、物理 Shift+Enter 换行、Alt+Enter 排队和释放事件；退出恢复。 |
| keys-modifyOtherKeys | 在支持该协议的终端启用 modifyOtherKeys，验证 Shift+Enter、Alt 组合及退出恢复。不能将发送合成 escape 当作物理按键证据。 |
| render-scroll-resize | 普通和全屏分别生成长回复、滚动至历史、等待新增内容、回到底部、缩放窗口；编辑器草稿和历史不丢失。 |
| theme-settings | `/settings` 更换主题、padding、布局、硬件光标；取消子菜单不错误提交，重启显示保存值；修改本地主题文件验证热更新。 |
| dialogs-selectors | 模型、thinking、Session、fork、tree 选择器的搜索/确认/取消；信任弹窗拒绝与允许分别验证，检查文件副作用。 |
| queue-cancel | 回复流动时 Enter 插话、Alt+Enter 排队、取回草稿、Esc 取消；继续生成，检查消息顺序及迟到输出。 |
| bash-abort | `!printf hello`、`!!printf excluded`，检查上下文差异；`!sleep 30` 后 Esc，确认子进程结束、会话可继续。 |
| external-editor | 设置 VISUAL/EDITOR 或 externalEditor 为真实编辑器；Ctrl+G 编辑多行草稿；确认光标、终端协议和输入恢复，临时文件已清理。 |
| maintenance | 两轮对话后 `/session`、`/compact`、`/reload`、`/export report.html`；核对压缩后继续、资源刷新和 HTML 内容；错误路径可见且可继续。 |
| normal-exit | 分别 Ctrl+D、连续 Ctrl+C、`/quit`；退出码、stty、光标、粘贴模式、全屏状态恢复，重复启动可用。 |
| signal-recovery | 从另一个终端对记录的 Pig PID 发送 SIGTERM/SIGHUP/SIGINT，记录退出码及状态恢复；SIGINT 可按信号退出，不要求与 Pi 的泄漏行为相同。 |
| editor-failure-recovery | 编辑器非零退出、命令不存在、编辑器运行中终止 Pig；草稿保留、错误可见、子进程和临时文件清理、shell 可用。 |

为每项记录步骤、观察、通过/失败和截图/终端录制文件。不同协议可使用不同终端；如果环境不支持某协议，该项保持 pending，不能标成 pass。

生成记录模板并手工填写：

```sh
python3 scripts/m6-evidence.py --template > /tmp/m6-evidence.json
python3 scripts/m6-evidence.py /tmp/m6-evidence.json
```

`commit` 填确切候选 commit；每项填写 `terminal`、`terminal_version`、`steps`、`observed`、`result` 和 `artifacts`，每个工件包含相对于记录目录的 `path` 与原始文件 `sha256`（用 `shasum -a 256`）。只有真实完成后填 pass。模板、缺项、失败、旧 commit 或工件哈希变化均拒绝冻结；脚本只能校验记录完整性，不能替代人的观察。
