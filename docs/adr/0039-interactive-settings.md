# 交互设置的生效与保存边界

#121 固定 Pi commit `936aff00918de1187f085f123c2812d8f2d67745`。菜单读取 legacy AgentSession 的队列和 thinking 实时值、SettingsManager 的有效合并值，以及 TextUI 的实际布局和 thinking 可见状态；复用 v3 Session，不引入另一套会话格式。

普通设置循环切换即保存全局配置，Esc 关闭不回滚已成功的修改。主题和 thinking 沿用既有子菜单，取消不提交。保留 Pig 已交付的 thinking、theme 顶部入口，其余条目的值序列遵循 Pi。精确像素布局不在本契约内。

UpdateInteractionSetting 使用现有 SettingsStorage 锁，只更新触及字段或嵌套子字段，保留未知字段、迁移规则和其他进程写入的内容。写入成功后才发布内存状态；读取失败或保存失败返回错误，菜单保留旧值并显示 Setting not saved。此严格保存策略沿用模型配置和模型范围的失败语义，不改变普通 SettingsManager setter 的错误记录契约。主题也采用严格保存，失败恢复有效主题。文件系统断电/部分写入不属于跨文件事务保证。

可信项目设置继续覆盖全局设置；未信任的项目配置不读取。全局变更不会撤销当前项目信任，默认信任只影响后续新项目会话。存在项目覆盖时菜单和真实 Session 反映项目有效值，全局偏好保存供其他项目使用。thinking 仍由已有 Session 配置事务保存。忙碌期间保持 ADR-0025 的配置拒绝语义。

18 个普通设置接通即时效果或明确的下一次启动效果；主题与 thinking 复用 #117/#120。quiet startup 控制 Pig 简洁启动帮助；double escape 使用有效 app.interrupt 绑定且仅在空编辑器、空闲会话生效。切换布局保留编辑器实例；退出 fullscreen 可输出 transcript 或已有会话文件的恢复提示。没有持久化文件的会话不提供无效恢复命令。

图片选项保留 M12 边界；Transport、HTTP idle timeout 与 warnings 的 Provider/认证分支保留既有矩阵边界；Mermaid、cache-miss notices、changelog 和 install telemetry 尚无对应可观察消费端，保持 partial，不展示无效果开关。#124 及后续所属能力票交付消费端时再接入菜单；这些不成为本票隐含前置。扩展运行时 M7、完整认证 M11、六平台验收 M13、未排期的 #99 均不扩张。

逐项 Pi 清单在 parity/interactive-settings-inventory.json，唯一能力状态和证据映射在 Parity Catalog 的 contract:codingagent/interactive-settings。SDK 菜单回调、CLI 子进程 PTY 的每项切换与重启，以及公开 SDK 的实际会话效果共同证明本次范围。
