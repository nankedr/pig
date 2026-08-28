# 用事件流与结果对象表达运行结局

AI、Agent 和 Harness 使用并发安全的无界 FIFO 事件流与可重复读取的独立 Outcome，Provider 失败或取消仍在 Outcome 中保留已有部分内容。实现以 `context.Context` 传播取消，并对外发布不可变快照，不把有界 channel、背压或 JavaScript 共享引用竞态变成公共语义。该模型最接近 Pi 的事件与终态契约，同时允许 Go 实现保持无 data race。

Faux response factory 同样不共享 Pi 的可变 state 引用。Pig 在 FIFO 出队并递增 `CallCount` 的同一临界区捕获逐调用快照，再把快照交给 factory；factory 对快照的修改不回写公开 state。因而并发的 N 次调用在 Pi 中可能都观察到最终计数 N，在 Pig 中按调用次序观察到 1 到 N。该偏离换取确定性和无 data race，并由独立 Parity Case 固定双方行为。
