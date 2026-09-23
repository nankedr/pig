# 交互主题采用会话实例状态、原生文件事件与固定语法规则

依据 #120 与固定 Pi commit `936aff00918de1187f085f123c2812d8f2d67745`：主题选择复用 M5 ThemeLoadResult、SourceInfo 与项目信任结论。主题预览不写设置，确认后写全局 theme，取消恢复当前设置。`light/dark` 表示自动模式；终端 scheme 报告优先于 OSC 11，再回退 COLORFGBG。无显式设置时仅高置信度背景探测保存默认选择。

Go SDK 可同时运行多个会话，因此 ThemeController 使用实例状态，渲染读取不可变快照，不使用 Pi 模块全局主题变量。替换会话资源时重绑 SettingsManager 与加载结果，不改变 legacy AgentSession、v3 Session、消息历史或编辑器实例。无接收者的主题辅助函数采用内置 dark；会话调用方通过 `Current().MarkdownTheme()` 获取自己的样式。

根据“补齐剩余缺口”的要求，本次撤销先前的 100ms 内容轮询方案。改用无 CGO 的 fsnotify 监听当前文件所在目录，仅接收当前路径的事件，在最后一次事件后 debounce 100ms。目录监听支持文件原子替换；切换路径会解除旧目录监听。无效、删除或不可读文件保留最近有效主题，报告诊断；恢复文件后继续更新。取消并等待 Watch 返回后释放 watcher、定时器，再停止终端及颜色订阅。

监听范围仍限于 M5 已授权资源，但包括可信项目和显式资源的 SourcePath，超过 Pi 的全局命名主题目录。实例隔离、扩大的授权路径范围和显式错误反馈是保留的 Go 偏离。

为保持语言及 token 分类与 Pi 一致，嵌入基线依赖的 highlight.js 10.7.3 全部语言规则，由纯 Go 的 goja 执行。运行时只执行内置规则，待高亮文本作为数据传入，不需要 Node、网络或 CGO。规则生成器从固定 Pi checkout 构建并检查字节一致性；结果缓存只存语法 HTML，不存主题颜色，避免切换主题后缓存旧颜色。无语言或未知语言按 mdCodeBlock 着色。

ThemeSettingsComponent 移植主题子菜单；SettingsList 提供搜索、导航、值切换与子菜单返回。Markdown 在默认文本、标题、引用之间分离样式上下文，内联 token 结束后恢复所在上下文。证据包括主题菜单逐步布局/回调、深浅主题语法 ANSI、宽窄终端 Markdown 逐字符颜色与装饰、实际 CLI 子进程 PTY 和文件 debounce。

本决策完成 #120 的主题兼容面，不扩大到其他尚未交付的设置项或 Markdown 方言能力；这些分别由其现有契约记录。M7/M11/M12/M13 与 #99 的排除范围不变。
