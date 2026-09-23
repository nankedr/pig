# M6 交互设置

在交互会话输入 `/settings`，键入名称过滤，Enter 或空搜索时的 Space 切换，Esc 返回。键位提示读取当前有效绑定；`/hotkeys` 展示所有有效快捷键以及布局、thinking 可见性、队列模式和双击取消键的动作。

普通项立即保存到全局 settings.json。可信项目覆盖仍优先，菜单展示实际生效值。保存失败时显示 `Setting not saved` 并保留旧配置。Esc 不撤销之前成功的切换。主题和 thinking 打开子菜单，确认才提交，取消保留原值。

| 设置 | 可选值 | 生效位置 |
| --- | --- | --- |
| Auto-compact | true / false | 下一轮使用现有自动压缩策略 |
| Skill commands | true / false | 立即重建技能命令补全 |
| Show hardware cursor | true / false | 终端光标，仍保持 IME 定位 |
| Editor padding | 0 / 1 / 2 / 3 | 当前编辑器水平边距，保留草稿 |
| Output padding | 0 / 1 | user、assistant、thinking、custom 消息 |
| Autocomplete max items | 3 / 5 / 7 / 10 / 15 / 20 | 当前编辑器补全列表 |
| Clear on shrink | true / false | regular 布局缩短时清除空行 |
| Terminal progress | true / false | 生成时 OSC 9;4，结束或退出时清除 |
| Steering / Follow-up mode | one-at-a-time / all | 真实 Agent 队列投递策略 |
| Hide thinking | true / false | 当前历史及后续回复；快捷键临时切换也反映在菜单 |
| Quiet startup | true / false | 下次启动的简洁帮助 |
| Default project trust | Ask / Always trust / Never trust | 后续没有保存决定的项目会话 |
| Double-escape action | tree / fork / none | 空闲且编辑器为空时，连续两次有效取消键 |
| Tree filter mode | default / no-tools / user-only / labeled-only / all | 下次打开 Session 树 |
| TUI mode | regular / fullscreen | 当前终端布局 |
| Fullscreen exit output | transcript / resume-hint | 退出时输出历史或已有文件的恢复命令 |
| Fullscreen scrollbar | auto / always / hidden | 当前 fullscreen 滚动条 |
| Thinking level | 当前模型支持的等级 | 现有 Session 配置事务，写入 v3 thinking 记录 |
| Theme | 已授权本地主题及 automatic | 预览、确认、取消复用 #120 |

Pi 另外提供图片显示/宽度/缩放/阻止、Transport、HTTP idle timeout、Mermaid、cache-miss notices、collapse changelog、install telemetry 和 warnings。本次不提供这些无消费端的开关；完整清单从固定 Pi 运行时提取并登记，未覆盖分支维持 partial。详见 ADR-0039。

SDK 使用 `AgentSession.GetInteractionSettings()` 读取当前配置，`UpdateInteractionSetting("steering-mode", "all")` 验证并保存普通项；thinking 用 `SetThinkingLevel`，主题用 `ThemeController.SetTheme`。`NewSettingsSelectorComponent` 提供可嵌入菜单，其回调由宿主连接；组件本身不写磁盘。`GetSettingsList` 返回焦点组件，`SetKeybindings` 绑定实例键位。GetInteractionSettings 中后续功能字段不表示已支持，能力范围以 Parity Catalog 为准。

运行离线示例：`go run ./examples/interactive-settings`。普通验收使用锁定 fixture 和 Faux，无需网络、Node 或 CGO；重新提取 Oracle 时需要已准备的固定 Pi checkout。

```sh
go test ./codingagent ./cmd/pig -run 121 -count=1
go test -race ./codingagent -run 121 -count=1
make m6-settings-oracle PIG_PI_ORACLE_CHECKOUT=/path/to/locked-pi
CGO_ENABLED=0 go build ./...
```
