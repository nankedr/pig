# 会话配置的提交与生效边界

Issue #87 沿用固定 Pi 的模型身份（provider + id）、可用目录快照轮换、scope 顺序、thinking 能力裁剪和持久化偏好。scope 中不在当前可用快照的模型被跳过；不足两个候选返回空结果，当前模型不在候选中则从索引 0 向指定方向移动。显式 scope thinking 优先，从非 reasoning 模型切回时使用 settings 中的偏好。关闭 reasoning 不覆盖保存的偏好。

AgentSession 的配置方法与 Prompt 入口共享锁。整个 Prompt（含工具循环、整轮重试等待）期间拒绝配置修改；同步 thinking_level_changed 通知期间也拒绝新配置和 Prompt，以免后一个修改或生成越过当前通知。事件在配置和持久化完成后发布，监听者可读取 Session 快照。重复设置相同的有效 thinking 不写历史、不发事件。调用者应在配置方法返回后开始下一次生成；直接修改底层 Agent 不属于 Session 配置事务。

这是 ADR-0006 不可变快照规则的 Go 映射。Pi 的扩展工具可在同一运行中修改下一请求的工具集；本切片尚未实现扩展运行时，不承诺该分支。独立 Agent 的配置方法不受此决策改变。

允许/排除规则限制整个工具注册表。tools 显式提供时限定可启用工具；noTools=all 且未提供 tools 时注册表为空；noTools=builtin 只改变初始启用集。当前默认注册四工具 read/bash/edit/write，显式配置的已交付 grep/find/ls 也沿用已有执行能力。未知、被排除、未注册和重复名称明确失败，整次工具切换不生效。Pi 忽略未知名称且允许重复；这里按 Issue 的明确失败要求与 Agent 现有唯一工具名称约束偏离。工具切换用创建时的 Context File 和自定义 system prompt 快照重新构建说明，不重新读取项目文件。工具集和 scope 不写入 v3 Session。

模型切换验证已知模型身份和结构、实时认证，以及运行时路径已有的 DeepSeek/OpenAI Chat Completions 适配器边界。注入 stream 的 SDK 可用于能力模型实验和确定性 Oracle，不新增任何真实适配器。合法模型描述可携带调用者的 endpoint、headers 和 thinking 能力覆盖，保持既有 SDK 模型参数语义；未知身份或非法 thinking 不接受。

Session 配置先准备完整 v3 文件，再写 settings，最后替换 Session 文件并发布内存状态。普通写入错误会返回错误；Session 替换失败会回滚本次触及的 settings 字段。此处比普通 SettingsManager 的记录错误模式更严格，以实现 Issue 要求的失败不半切换。两个文件不是跨进程或断电事务；进程在两次文件写入之间崩溃，以及底层存储在报错前部分写入，仍受文件系统/SettingsStorage 的既有可靠性限制。回滚失败会与原始错误一起返回，不报告成功。
