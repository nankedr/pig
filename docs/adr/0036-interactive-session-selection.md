# 交互 Session 选择与替换边界

Issue #118 复用 ADR-0032 的先构建目标、再销毁旧 Session 契约。InteractiveMode 先等待旧 Prompt worker 完成并解除该轮监听，再提交 runtime 替换，避免迟到事件进入新 transcript。只打开或取消选择器不会停止当前生成；成功替换后清空旧编辑器历史及旧生成轮次的排队任务，重建目标历史与资源补全，并保留切换期间新输入的草稿和待处理按键。

恢复入口拒绝不存在、空、特殊或 JSON 损坏的文件；OpenSessionManager 的既有可创建语义不变。同 CWD 恢复复用已注入的 SettingsManager 及其 ApplyOverrides；跨 CWD 使用既有项目信任流程，CLI 显式信任覆盖继续生效。新的 Headless runtime factory 在构建目标资源前消费交互信任准备步骤，自定义 SDK factory 仍负责自身装配。失败返回可见错误并保留旧 Session；生成可能已经取消，但后续仍可继续。

相对固定 Pi：选择器同步装载而不实现异步进度；正则使用无 CGO 的 Go RE2 子集；删除仅提供 Pi unlink 分支，不调用外部 trash；缺失 CWD 不提供替换目录对话框。上述分支在 Parity Catalog 明确为 partial。此决策不扩展 M7、M11、M12、M13 或包生态范围。
