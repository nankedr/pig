# M4.12：通过 JSONL RPC 对话

从仓库根目录运行 `go run ./examples/rpc-chat`。示例编译真实 pig 子进程，启动本地 HTTP Provider fixture，通过公开 RPCClient 发起对话、收集事件、读取最终文本并停止进程，无需真实 API key。

真实服务可运行 `pig --mode rpc --provider deepseek --model deepseek-v4-flash`，沿用已有环境或文件凭证、Session 和 trust 配置。在 stdin 写入：

```json
{"id":"p1","type":"prompt","message":"你好"}
{"id":"s1","type":"get_state"}
{"id":"a1","type":"abort"}
{"id":"m1","type":"get_messages"}
```

每条记录写一个 LF，按 id 找响应，不能依靠完成顺序。prompt success 表示接纳，后续 agent_start、message_update、message_end、agent_settled 表示运行过程。Provider 错误保留在 Assistant 消息和事件中；预检失败只有一条失败响应。abort 会等待 Session idle 后响应。RPC 没有 text/json 模式的 Session header。

客户端典型顺序是 `NewRPCClient → Start(ctx) → PromptAndWait(ctx, text) → GetMessages/GetState → Stop(ctx)`。持续展示可以先 OnEvent 再 Prompt，最后 WaitForIdle。每个订阅收到独立快照；回调可再次查询，但应及时返回。取消请求 Context 不等于取消远端生成，后者使用 Abort。对实时 UI 使用带截止时间的 Context；API 不添加隐式请求超时。

已交付的命令为 prompt、abort、get_state、get_messages、get_last_assistant_text。prompt 的 streamingBehavior 可将运行期间的新文本排入 steer/followUp；独立 steer/follow_up 命令、图片、模型/配置切换、Session 切换、统计、树、bash、compaction、HTML 和扩展 UI 命令仍明确失败。这是基础传输切片，不代表全部 RPC 命令已完成。

验证见 `go test -race ./cmd/pig -run '^TestRPC94' -count=1`。固定 Pi source/dist Oracle 用例覆盖宽松分派和公开客户端事件投影；真实 Go 子进程覆盖 UTF-8 分片、EOF、并发、abort、输出失败与本地 waiter 清理。权威能力状态在 Parity Catalog 的 `contract:rpc/jsonl-transport`；Go 映射见 ADR-0028。
