# 交互维护命令复用会话生命周期

Issue #124 以 Pi `936aff00918de1187f085f123c2812d8f2d67745` 为基线，交互层只路由 `/compact`、`/reload`、`/session`、`/export`。压缩、资源读取、统计与 HTML 文件生成仍分别由既有 AgentSession.Compact、Reload、GetSessionStats、ExportToHTML 负责，不引入第二份 Headless 或 SDK 实现。

压缩与重载在受控工作任务中执行，Run 保持读取输入并显示进度。Escape 取消任务的 Context；退出先取消并等待任务返回，再释放终端和 Session。输入复用递增 turn 身份，避免迟到的 Escape 取消下一轮。活动生成时 `/compact` 先等待生成取消收尾，`/reload` 按固定 Pi 交互契约提示等待；底层 Reload 可与生成并行的 ADR-0035 不变。活动 Bash 时拒绝压缩，避免并发历史写入。

维护期间提交的新输入保留回编辑器，显示忙提示，完成后由用户提交。这沿用 ADR-0026 的 Go 接纳边界；Pi 压缩队列自动排空尚未覆盖，明确 partial。取消保留 errors.Is 语义：Oracle 中 split-turn 取消可能显示 `Compaction failed: ... operation was aborted`，Pig 显示 `Compaction cancelled`；比较不伪造错误字符串相同，而验证无新压缩记录且可继续生成。只在成功结果上显示摘要，提交后的取消不会撤回结果。

重载成功后重新应用键绑定、交互设置、主题及自动补全，并展示本次资源快照的诊断。失败与取消不回滚已完成阶段，保持 ADR-0035。模型目录刷新、扩展运行时和 trust hooks 不属于本次交付。统计直接呈现权威 SessionStats：当前上下文与整棵历史累计 usage/cost 分开，压缩后尚无新回复时显示上下文未知。

HTML 导出沿用 ADR-0033，支持默认位置及单/双引号路径；不上传、不打开托管服务。生成期间允许只读统计与快照导出。`.jsonl` 导出仍返回显式 NotImplemented，`/share` 仍 Stub。精确 Pi 动画、按模型的成本细分/cache waste、扩展 M7、认证 M11、图片 M12 和六平台 M13 保持 partial/deferred；包生态 #99 不阻塞本票。
