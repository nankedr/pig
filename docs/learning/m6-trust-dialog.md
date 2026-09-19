# M6.6：先决定项目信任，再进入会话

项目信任控制是否读取 `.pig` 项目设置和自动发现的项目资源。它不是 Tool 审批或 sandbox；内建 Tool 和受信内容使用宿主用户权限。`AGENTS.md` 等 Context File 仍按既有规则进入上下文。包生态和扩展执行保持原来的 Stub 边界。

## 运行

```sh
go run ./examples/trust-dialog
go run ./examples/trust-dialog --interactive
go build -o /tmp/pig ./cmd/pig
cd <含有 .pig/settings.json 或 .pig/SYSTEM.md 的项目>
/tmp/pig --offline --provider deepseek --model deepseek-v4-flash
```

示例在临时目录操作，不改变当前项目或用户信任记录。第一条命令离线展示 SDK 的显式信任；第二条使用真实终端交互。真实 CLI 仍需已有 Provider 认证；自动验收使用回环 SSE 服务，无外部网络。

未知项目的 `ask` 策略显示五种选择：信任当前目录、信任父目录、仅本次信任、保存拒绝、仅本次拒绝。上下键或 j/k 移动，Enter 确认，Escape / Ctrl+C 取消。取消等同于本次拒绝，不保存决定，继续进入可用会话；下次启动仍会询问。保存父目录信任会删除当前目录覆盖，后代继承最近的已保存决定。

优先级沿用现有语义：显式 `--approve` / `--no-approve`，无敏感资源时自动信任，最近祖先的已保存决定，全局 `defaultProjectTrust`（always / never / ask），最后才询问。无 UI 的 ask 按不信任处理。显式覆盖和默认策略不自动持久化。

## SDK 边界

`codingagent.PrepareProjectSettings` 接收已有、默认未信任的 `SettingsManager`。传入 `Terminal` 才允许交互；省略时沿用 Headless 语义。只有持久化成功才调用 `SetProjectTrusted(true)` 加载项目设置。把准备好的 SettingsManager 注入现有 `CreateHeadlessSession` 即可复用 legacy AgentSession、v3 Session 与本地资源装配。

`tui.SelectList` 对 value 做不区分大小写的前缀筛选，筛选重置选择，上下键循环；`tui.SelectDialog` 使用与 Pi 启动选择器相同的夹紧导航，`List.SetFilter` 可由宿主提供筛选。`ShowSelectDialog` 管理一次终端生命周期，选择返回 item，取消返回 nil，终端/上下文错误向上传递。

`TUI.ShowOverlay` 支持宽度、最大高度、锚点、百分比位置、偏移、边距、可见性与非捕获浮层。Handle 的 Hide 移除浮层，SetHidden 临时隐藏；Focus/Unfocus 控制输入。关闭恢复此前焦点，窗口宽度变化触发可见性重算。输入回调可关闭浮层；组件内更新后用 RequestRender 排队渲染。

## 可复核证据

```sh
go test -race ./tui ./codingagent ./cmd/pig -run '114' -count=1
node --experimental-strip-types parity/oracle/dialogs.mjs <locked-pi-checkout> --check
node parity/oracle/overlays.mjs <locked-pi-checkout> --check
node parity/oracle/trust-dialog.mjs <locked-pi-checkout> --check
```

Pi 固定在 `936aff00918de1187f085f123c2812d8f2d67745`。`trust-dialog.json` 来自真实 Pi CLI + PTY + 受控服务：六种选择分别跑两次启动，比较对话框是否出现、SYSTEM.md 是否进入请求、Context File 例外、信任存储、退出码和终端恢复。普通 Go 验收只读取锁定 fixture，不需要 Node、Python 或 Pi。

`dialogs.json` 记录公开 SelectList 的筛选、选择、渲染和回调；`overlays.json` 记录公开 TuiMainScreen 的定位、尺寸、边距及宽字符遮挡。额外 Go 测试使用 FIFO 证明决定前以及拒绝/取消后不读取敏感项目文件，保存失败不会进入可用会话，信号退出恢复终端。

遵守 ADR-0010，Pig 修复 Pi 的决定前项目设置读取缺口。未覆盖的复杂多浮层 resize focus restore、完整 ExtensionSelector/TrustSelector 扩展组件、动态主题加载、图片以及六平台运行验收继续标记 partial / Stub；本票不宣称整个 TUI 已对等。
