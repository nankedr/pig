# Provider refresh 使用同步 generation commit

`ModelsPublication.Update` 是同一 Provider refresh generation 的同步提交临界区。Pig 在调用 Update 前原子确认 generation 仍有效，并在 callback 返回前拒绝同一 Provider 的同步 `Models.Refresh`；不同 Provider 的 refresh 仍可并发。Provider、store 与 auth 的阻塞 callback 必须观察传入的 `context.Context` 并在取消后尽快返回。

固定 Pi 依靠 JavaScript 单线程 turn，使 generation check 与同步 Update 之间不会被另一个 refresh 插入。Go 的阻塞 `Refresh` 与任意 callback `func()` 无法同时支持同步同 Provider 重入和原子 stale-write 排除：若持锁等待会自锁，若释放锁则旧 callback 可在嵌套新 generation 返回后覆盖新状态。Pig 因此显式拒绝该重入组合，保留 generation-checked publication、正常 supersession、跨 Provider 并发和无隐藏 timeout。该偏离比允许 stale publish 或持锁调用扩展代码更可审计，并以结构化错误暴露而非静默挂起。
