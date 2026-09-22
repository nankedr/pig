# 交互主题采用会话实例状态与可取消的本地轮询

依据 #120 与固定 Pi commit `936aff00918de1187f085f123c2812d8f2d67745`：主题选择复用 M5 ThemeLoadResult、SourceInfo 与项目信任结论。主题预览不写设置，确认后写全局 theme，取消恢复当前设置。`light/dark` 表示自动模式；终端 scheme 报告优先于 OSC 11，再回退 COLORFGBG。无显式设置时仅高置信度背景探测保存默认选择。

Go SDK 可同时运行多个会话，因此 ThemeController 使用实例状态，渲染读取不可变快照，不使用 Pi 模块全局主题变量。替换会话资源时重绑 SettingsManager 与加载结果，不改变 legacy AgentSession、v3 Session、消息历史或编辑器实例。

核心无 CGO：每 100ms 检查当前主题内容，以替代原生文件 watcher 的 100ms debounce。只读取已由 M5 授权加载的普通本地文件；除 Pi 全局主题目录外，也支持可信项目和显式资源的 SourcePath。原子替换后按实际路径加载；无效、删除、不可读时保留最近有效主题，并按变化给出 warning。切换路径使旧路径立即失效，取消并等待轮询返回后再停止终端，移除颜色订阅、关闭通知模式。

上述实例隔离、轮询、扩展监听范围和错误反馈是有意的 Go 偏离。精确设置子菜单布局/搜索、语法高亮与完整 Markdown 样式保留 partial；M7/M11/M12/M13 与 #99 范围不变。
