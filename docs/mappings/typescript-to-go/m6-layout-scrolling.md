# M6.5 TypeScript → Go

| 固定 Pi 边界 | Pig 公开边界 | 对等观察 |
| --- | --- | --- |
| ScrollView | tui.ScrollView | 固定单子组件、范围限制、跟随末尾、滚动条 |
| VStack / HStack / allocateStackSizes | tui.VStack / HStack / AllocateStackSizes | basis、grow、shrink、约束、间距、可见性 |
| renderLayoutFrame | tui.RenderLayoutFrame | 嵌套布局、裁剪、光标可见行、主 ScrollView |
| getScrollViewsAt / getScrollbarGeometry | tui.GetScrollViewsAt / GetScrollbarGeometry | 命中次序、滑块几何 |
| TuiAltScreen | tui.TUIAltScreen | 普通组件或自定义布局根，逐行差分，键盘、鼠标、首尾 |
| TuiMainScreen | tui.TUIMainScreen | 原生 scrollback、增量重绘、光标、捕获／恢复状态、强制重绘 |
| InteractiveMode | codingagent.InteractiveMode + tui.TextUI | 同一 Transcript 与 Editor、CLI 参数和 settings 选择模式 |

`TextUI.SetMode` 是 Go 对话组合的公开模式切换入口。布局组件通过 `LayoutComponent.LayoutNode()` 表达 Pi 的 symbol capability。Go 的 `Component.Render` 和布局操作显式返回 error。

详细边界、命令及 fixture 重放见 [学习说明](../../learning/m6-layout-scrolling.md)。
