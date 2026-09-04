# M2.5 内存 Telemetry 与 adapter conformance

Issue #64 交付默认零值可用的 `telemetry.InMemoryTelemetryContext`。通过 `StartSpan` 显式传递父 span，`GetSpans` 返回按开始顺序排列的独立快照；SDK 默认仍使用 `NOOPTelemetryContext`，没有 exporter、端点或全局当前 span。

运行离线示例：

```sh
go run ./examples/telemetry
```

输出父子 span 的名称、父 ID、状态和结束顺序，然后实际执行全部 9 个 conformance case。

## 生命周期与快照

`StartSpan` 在调用 goroutine 中执行 callback，原样返回 `(any, error)`；异步工作由 callback 等待 goroutine/channel 完成。callback panic 会先结束 span，再原样 panic。成功默认 `ok`，失败默认 `error`；最后一次有效 `SetStatus` 优先，不被自动状态覆盖。普通 Go error 的详情映射为 `{Name: "Error", Message: err.Error()}`，非 error panic 或无法读取的 error 只记录 error 状态。

span ID 和结束序号从 1 开始，分别按实际开始和结束顺序递增。并发子 span 可以先于或晚于父 span 结束。settled 后的记录调用直接忽略；晚建 child 使用 NOOP，但仍执行 callback 并传播结果。

记录和快照会复制属性 map、标量数组/slice、事件列表、状态详情和 ID/结束序号指针。修改输入或返回快照不会回写记录。方法支持并发，callback 和错误详情读取在锁外执行；调用者仍须保证传入 map、slice 和状态详情在方法读取期间不被其他 goroutine 修改。收集器无界保存 span 和事件，应按测试或独立工作范围创建实例。

## Go 被动性映射

Pi 的 `SpanOptions`、`SpanStatus` 和属性对象可以通过 getter/Proxy 抛错。Go 的现有 options/status 结构体不能承载这种行为；不为模拟 Proxy 扩展公共 API。

动态属性支持 string、bool、数字及其一维数组/slice（包括标量别名和 `[]any`）；nil 属性表示 unset，合并时保留原值。函数、map、指针、嵌套数组等超出契约的值导致本次 options/属性/event 操作整次忽略。非法初始属性直接使用 NOOP，不保留半个 span；未知 status 忽略且不标记为显式状态。有效 `ok` 状态忽略 Error 字段。所有转换避免调用用户的格式化或序列化方法。

这是不可读 payload 在 Go 静态 API 上的测试映射，不宣称兼容 Pi 对契约外任意 JavaScript 值的行为。`Error()` panic 在独立边界抑制，不改变业务返回的 error；callback 自身的 panic 不被吞掉。

## 验证与适配器接入

`telemetry/testing.CreateTelemetryAdapterConformance(factory)` 保留固定 Pi 的 9 个 case 名称及顺序。每个 `Run(ctx)` 创建独立 fixture，运行公开接口断言并关闭 fixture。断言、工厂、快照读取和关闭失败通过 error 报告；清理使用保留 context 值的无取消 context。

接入任意 adapter 时，实现 fixture 的 `Context()`、`GetSpans(ctx)` 和 `Close(ctx)`，按 case 注册到测试框架。`GetSpans` 应返回已经可观察的 span 快照。示例中的 factory 展示了内存适配器的完整接法。套件会检查 callback、状态、属性、事件、父子关系、结束顺序、晚建 child 和被动性；专用 Go 测试另覆盖快照隔离、错误检查重入和并发记录。

```sh
go test -race ./telemetry/... -count=1
go test ./internal/parity -run '^TestTelemetryMemoryParity$' -count=1
node --experimental-strip-types parity/oracle/telemetry.mjs .upstream/pi --check
```

Oracle 校验 checkout 为固定 commit `936aff00918de1187f085f123c2812d8f2d67745` 且 tracked 文件无修改，运行内存场景及 Pi 原始 9 个 case。提交的 fixture 记录初始快照、独立快照、结果身份、父子关系和完成顺序，Go 重放无需网络或 Pi checkout。

公开 API snapshot 继续由 `telemetry/telemetry_test.go` 与 `telemetry/testing/conformance_test.go` 的编译期声明检查及 `internal/surface` 门禁约束。动态 schema 校验和由 schema 生成专用 typed helper 仍不在本切片范围内，Catalog 的模块总状态保留 partial。
