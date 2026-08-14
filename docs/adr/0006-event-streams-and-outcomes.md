# 用事件流与结果对象表达运行结局

AI、Agent 和 Harness 使用并发安全的无界 FIFO 事件流与可重复读取的独立 Outcome，Provider 失败或取消仍在 Outcome 中保留已有部分内容。实现以 `context.Context` 传播取消，并对外发布不可变快照，不把有界 channel、背压或 JavaScript 共享引用竞态变成公共语义。该模型最接近 Pi 的事件与终态契约，同时允许 Go 实现保持无 data race。
