# M2.3 Deferred response 生命周期

调用者通过 `FauxCore.StreamSimple`、`Provider.StreamSimple` 或 `Models.CompleteSimple`，传入 `SimpleStreamOptions.Deferred` 提交延迟响应。`DeferredBoolean{Enabled: true}` 与 `DeferredWindowOptions{}` 都开启 deferred；缺省或 false 继续使用同步流。`NewFauxProvider` 无需额外配置即可支持 deferred，`FauxDeferredOptions` 只配置模拟轮询次数与间隔提示。

首次流的事件是 `start → done:deferred`，Stream Outcome 的 `Deferred` 包含 Provider、ModelID、API 和 ID。`PollAfterMS` 只在显式配置时出现，保留零值。Faux 不模拟过期时间，也不按 window 或 `WaitMS` 等待；真实适配器仍按各自能力决定支持范围。

`FetchDeferred` 在配置的 pending 次数内仍返回 `StopReasonDeferred`，每次携带同一个 Deferred Handle。随后首次就绪读取生成并缓存 final Assistant Message；重复读取不再出队或执行 factory，成功和脚本错误都稳定。提交时保留请求与模型快照，factory 获得提交的 options，但移除 `Deferred` 和提交的 `OnResponse`。提交、每次 fetch 和 cancel 分别调用各自的 response hook。

`CancelDeferred` 校验 handle 的四个身份字段与请求模型，记录有效取消并阻止后续成功读取。未知、其他 Faux 实例、Provider/Model/API 不匹配的 handle 都失败。fetch 失败表现为错误 Stream Outcome；cancel 返回 Go error。Models 继续完成认证、端点与 header 转换，保留 telemetry context 和 hook。未实现的 Provider/API 保留 `ErrNotImplemented`。

请求 context 只取消本次提交或读取；提交完成后取消原 context 不会取消 Deferred Handle，取消一次 fetch 也不会取消其他读取。显式 handle 取消会唤醒等待者并中止尚在传输的流，保留已发出的部分内容。已发布终态保持不变。并发 fetch 共享一次 factory 执行；同步 factory 没有 context 参数，等待者可以退出，但执行中的用户 factory 必须自行返回。公开 `State` 字段沿用原有约定，只在相关调用结束后读取。

离线运行：

```sh
go run ./examples/deferred-response
go test -race ./ai -run '^TestDeferred' -count=1
go test -race ./internal/parity -run '^TestDeferredLifecycleParity$' -count=1
node --experimental-strip-types parity/oracle/deferred-lifecycle.mjs .upstream/pi --check
```

Parity Case 覆盖同步回归、handle 字段、pending/final、缓存错误、有效取消、无效 fetch、hook 和脚本消费计数。固定 Pi 的无效取消行为与并发实现不满足 #62 的约束；这些差异由 [ADR-0015](../adr/0015-deferred-response-lifecycle.md) 和独立 Go 测试记录，不作为 Pi/Pig 等价声明。此切片不增加真实网络适配器的 deferred 能力。
