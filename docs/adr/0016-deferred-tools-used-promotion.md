# 已调用的动态 Tool 恢复即时定义

#63 明确要求只延迟尚未使用的动态 Tool，因此 Pig 在完整 transcript 中查找 ToolCall，已调用的名称始终进入 immediate，即使调用发生在 addedToolNames 标记之后。固定 Pi 的 `splitDeferredTools` 按消息顺序收集名称，标记后调用不会清除 deferred；此处按 Issue 要求接受语义偏离，用 `deferred-tools-used-deviation.json` 保存 Pi 的真实结果，并分别断言双方行为，不把差异归一化成对等。

Go SDK 暴露 `ai.SplitDeferredTools`，由调用者提供名称归一化函数，nil 表示恒等匹配。immediate 和 deferred 都返回有序 Tool 切片，以保留 Pi Map 的插入顺序；规范化名称用于去重和匹配，定义内容保留最后一次出现的原值。API Adapter 的 wire 方言和名称规则仍由 M10 实现。
