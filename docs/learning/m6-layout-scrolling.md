# M6.5 长对话与终端布局

对应 [#113](https://github.com/nankedr/pig/issues/113)，对等基线为 Pi `936aff00918de1187f085f123c2812d8f2d67745`。CLI 继续复用 legacy AgentSession、v3 Session 和 Transcript；核心无 CGO。

```sh
go run ./examples/layout-scrolling
go run ./cmd/pig --tui-mode fullscreen
go run ./cmd/pig --tui-mode regular
go test ./tui ./cmd/pig -run '113' -count=1
```

全屏模式进入 alternate screen，退出、失败与挂起时归还终端。`settings.json` 的 `tuiMode` 和 `fullscreenScrollbar` 分别选择布局及 `hidden`／`auto`／`always` 滚动条；默认退出后输出对话文本。`resume-hint` 暂只归还屏幕，恢复提示仍为 partial。

全屏键盘沿用 keybindings.json 的 `tui.altScreen.*`：PageUp/PageDown 每次滚动一页减四行，半页键滚动半页，Home/End 到首尾。滚轮按命中区域从内向外传递剩余位移，再按 Pi 基线回退到主 ScrollView。注意基线的 `contain` 不阻止最后这次主视口回退。支持 SGR 和旧式 X10 滚轮，以及 SGR 滚动条拖动。普通模式保留终端鼠标行为，另提供 PageUp/PageDown 浏览应用内历史；Home/End 仍用于编辑器。

跟随末尾时新增输出保持可见。主动上滚后保留 `scrollTop` 行偏移，窗口高度变化和编辑区增长不会自动跳到末尾；内容缩短后限制到新的合法范围，回到末尾即恢复跟随。这与固定 Pi 的行偏移锚点一致，不承诺宽度重排或在锚点之前插入内容后仍定位同一文本字符。

## SDK

- `ScrollView` 管理单个子组件、滚动范围、末尾跟随和滚动条；构造不启动终端。
- `VStack`／`HStack` 通过 basis、grow、shrink、minSize、maxSize 和 visible 分配尺寸。`RenderLayoutFrame` 返回裁剪后的行、布局树和主滚动区域。
- `GetScrollViewsAt` 返回最深层优先的命中，`GetScrollbarGeometry` 返回轨道和滑块几何数据。
- `TUIAltScreen` 支持布局根、键盘／鼠标滚动、聚焦输入和终端生命周期；`TUIMainScreen` 支持普通屏幕渲染与状态捕获。
- `TextUI` 为对话和编辑区提供现成组合，通过 `TextUIOptions.Mode` 选择模式；`SetMode` 保留草稿和滚动位置，`ScrollToTop`／`ScrollToBottom` 可供应用按钮或命令调用。示例的 `/mode`、`/top`、`/end` 是示例命令，不是 Pig CLI 命令。

组件自身的可变内容由调用者同步；更新内容后调用 `RequestRender`／`Refresh`。`Transcript` 和 `TextUI` 已在其公开接口内同步。直接更改布局树应在停止渲染时完成；`SetLayoutRoot` 可用于运行时替换根。

## 验收证据与边界

SDK fixture 从 Pi 公开布局函数、组件和注入终端输入回调提取，涵盖尺寸分配、裁剪、嵌套命中、滚轮传播、拖动、持续追加、缩放和缩短。CLI fixture 来自真实 Pi 子进程、PTY 和分阶段放行的本地 SSE。Pig 普通验收仅使用锁定 fixture 与本地受控服务，不要求 Node、Pi 或在线 Provider。

```sh
node parity/oracle/layout-scrolling.mjs /path/to/locked-pi --check
node parity/oracle/layout-scrolling-cli.mjs /path/to/locked-pi --check
go test -race ./tui -run '113' -count=1
CGO_ENABLED=0 go test ./...
```

`contract:tui/layout-scrolling` 保持 partial：终端帧使用同步输出中的完整重绘，尚未复刻 Pi 普通屏幕的增量重绘与原生 scrollback 组织；应用内历史可回看。选择、复制、URL 打开、提示词跳转、overlay、flash、颜色查询和恢复提示不在已验收分支内，原有相关 Stub 保留。图片属于 M12、扩展运行时属于 M7、六平台验收属于 M13，#99 不参与本票。

本机证据：darwin-arm64 上已用独立交互 PTY 运行无 CGO 示例，输入 Home、`/mode` 两次、End、`/quit`，观察到历史首尾切换、alternate screen 进入／退出序列与正常退出。Terminal.app 的 GUI 访问被电脑控制工具安全限制拒绝，因此真实 GUI 终端的目视演示未验收；以上 PTY 结果不替代这一项。
