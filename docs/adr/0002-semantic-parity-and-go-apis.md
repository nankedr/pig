# 以语义对等约束 Go API

发生冲突时，Pig 依次优先保证可观察语义、Pi 架构与算法、Go 惯用实现，最后才考虑教学简化；私有实现可采用等价 Go 写法，公共偏离必须先明确决策。兼容以解码值和行为等价为主，不要求普通 JSON/CBOR 字节相同，但协议 framing 和线标识仍须精确互操作。作为已批准的 Go API 偏离，固定 Pi Client 没有 request cancellation API，而 Pig 的阻塞方法接收 `context.Context`：取消只释放本地 waiter，不注入隐式 timeout，并保留 request ID tombstone，直至迟到 response 被消费或连接断开；该行为到 M9 才实现，之前必须保持 Capability Stub。这样既避免机械翻译 TypeScript，也防止以“Go 风格”为由缩减功能。
