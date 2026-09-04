# Deferred response 使用快照、单次生成和有序取消

#62 要求重复读取不重复消费响应、取消阻止后续成功读取，并拒绝未知或不匹配的 Deferred Handle；Pig 因此将这些要求落实为公开语义：按实例与 Provider/Model/API/ID 校验 handle，保留提交快照，并让并发 fetch 共享一次 final 生成。固定 Pi 仅按 ID 取消且未知 ID 也成功，并发 fetch 可能重复进入异步 factory，还保留提交对象的共享引用；Pig 延续 ADR-0006 的不可变快照与无竞态原则，明确采用不同的行为。

取消本次请求与取消 Deferred Handle 分开处理：提交结束后取消其 context 不影响后续 fetch；取消一次 fetch 只停止该读取；显式 handle 取消唤醒所有等待者，并与成功终态发布串行化，已发布的 Stream Outcome 保持不变。有效 cancel 先提交取消记录，再调用 response hook，因此 hook 失败不撤销取消；重复 cancel 返回已取消错误。没有 context 参数的同步 factory 无法被强制终止，但不会阻塞 fetch 的取消返回。

factory 的 state 沿用 ADR-0006，在提交出队时捕获快照；其 `DeferredFetchCount` 不代表随后轮询时的实时状态。固定 Pi 的顺序生命周期由 `deferred-lifecycle.json` 对等验证，上述差异与终态顺序由公开 Go API 的确定性和 race 测试单独验证。
