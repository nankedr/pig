# 交互分支选择与导航的提交边界

Issue #119 为既有 ADR-0027/0030 的 Session 树导航和 ADR-0032 的 runtime fork 增加交互入口。公开选择器只持有防御性快照并返回选择，不自行导航或生成；InteractiveMode 负责摘要选项、生成收尾和 UI 恢复。

打开或取消选择器不改变生成状态。确认导航后先恢复排队消息，再取消并等待旧 Prompt worker 完成。成功导航使交互轮次失效，旧轮次延迟提交只能回到草稿，不能自动进入新分支。fork 替换整个 runtime；树导航保留同一个 AgentSession。两条路径均重新投影 transcript 和编辑历史。

fork 用所选用户消息替换草稿；树导航仅在草稿为空时填入 EditorText。编辑器草稿写入持有 TextUI 锁。摘要生成期间使用独立 context，Escape、外部取消或终端关闭都会取消请求；正文输入保留为草稿，摘要结束后恢复。摘要的事务、重试和上下文格式直接沿用 AgentSession，不再实现第二套导航逻辑。

标签修改通过 Session 锁与配置/生成互斥，先写临时 v3 文件并原子 rename，成功后才更新 Session 的 entries/leaf 与选择器快照，写入错误保留编辑状态。生成期间拒绝标签修改是相对 Pi 的原子性增强，避免 label 改写 leaf 干扰活跃生成。选择器的标签时间当前显示 RFC3339，树连接简化且不自动横向平移；这些显示分支保留 partial。系统剪贴板仍为显式 Stub，SDK 可注入复制回调。

没有新增 RPC wire 命令、CGO 或扩展运行时依赖；父 issue #8 的里程碑边界不变。
