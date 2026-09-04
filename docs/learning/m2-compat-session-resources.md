# M2.6 compat 与 Session Resource

旧调用方使用 `ai.Stream` / `ai.Complete` 或其 Simple 版本。入口按 `Model.API` 获取注册表快照，原样传递事件流、Context 和具体 options；Complete 等待同一流的 Result，即使请求已取消也保留 aborted Outcome。Provider 失败仍是带 partial 内容的 Outcome，结构错误仍通过 Go error 返回。

16 个 deprecated stream 别名复用注册表，并验证各自的固定 API ID。注册自定义实现或 Faux 后，旧调用方可直接消费同一行为；直接 ProviderStreams 和新建独立 Provider 保持自己的适配器。与固定 Pi 的差异见 [ADR-0017](../adr/0017-compat-registry-and-resource-cleanup.md)。

注册覆盖保留顺序，source 注销只移除当前由它拥有的注册项。`RegisterBuiltinAPIProviders` 只填补缺项，`ResetAPIProviders` 原子恢复初始十项。注册回调在锁外执行，已开始的 Stream 不受后续 reset 影响。

Faux core、新建 Provider 和 compat 注册均使用同一队列实现。Set 替换队列，Append 追加，读取按 FIFO 出队；不同实例不共享队列和 cache，同实例不同 session 的 cache 隔离。Unregister 可重复调用，不影响后来覆盖同一 API 的实例；持有旧 handle 的调用者仍可操作旧实例。公开 State 指针应在流结束后读取；并发响应工厂接收 ADR-0006 规定的逐调用快照。

资源所有者通过 `RegisterSessionResourceCleanup` 注册回调，并保存注销函数。`CleanupSessionResources("session")` 传递 session ID，无参数表示清理全部资源。每次调用都执行当时的注册快照，重复清理仍调用回调；资源所有者负责幂等释放。一个回调 panic 不会阻止后续回调，返回的聚合错误可通过 `errors.Is` 查出原 error。回调内增删注册从下一次清理生效，并发 cleanup 需要回调自行同步资源状态。

离线运行：

```sh
go run ./examples/compat-session-resources
go test -race ./ai -run 'TestCompat|TestSessionResourceCleanup|TestIssue65'
go test ./internal/parity -run '^TestCompatSessionResources'
node --experimental-strip-types parity/oracle/compat-session-resources.mjs .upstream/pi --check
```

最后一项需要预装依赖的固定 Pi checkout，普通门禁只重放已提交 fixture。真实 Responses、Azure、Codex 协议仍属于 M10；其他协议和 ambient auth 属于 M11；图片路径属于 M12。底层 Stub 不调用 fetch 或 hooks，注册用户实现不代表这些内置能力已实现。
