# M3.2 历史 Session 恢复与互操作

显式 `--session <path>` 或 `OpenSessionManager` 可以打开固定 Pi 基线的 v1、v2、v3 文件，然后通过同一个 AgentSession 继续对话。只访问显式文件，不发现或迁移其他 Pi 状态。

## 打开与迁移

reader 逐行读取 JSONL，跳过畸形 JSON 和空行。没有 discovery 扫描上限、Scanner token 上限或固定 entry 数量上限；内存仍随有效记录和最长行增长。测试覆盖超过 512 MiB 的稀疏文件、超过 1 MiB 的 header/坏行、3 MiB UTF-8 消息和 10,001 条 entry。

第一条有效记录必须是 `type: session` 且 `id` 为字符串。非空非法文件报错并保留原内容，空文件初始化新 header。与固定 Pi 相同，缺少 version 视为 v1；version 大于等于 3 不迁移。

v1 顺序分配 entry ID 和 `parentId`，把 compaction 的 `firstKeptEntryIndex` 转为 `firstKeptEntryId`。v2 把 message 的 `hookMessage` role 改为 `custom`。迁移在打开时写回同一路径，保留原始 JSON 的未知字段与开放消息，只更新迁移涉及的字段；之后新消息按 v3 追加。重开已迁移文件不重复生成 ID。

无尾换行的历史 v1/v2 会因迁移重写而补上换行。v3 reader 也能读最后一行，但固定 Pi writer 直接追加：缺少尾换行时，旧末行与第一条追加记录会粘连，下一次读取将其作为坏行跳过。Pig 对这一基线行为保持一致；要可靠继续手工生成的 v3 文件，先确保文件以换行结尾。

## 恢复的上下文

reader 保留 model、thinking、message、compaction、branch summary、custom、custom message、label 和 session info。恢复上下文沿当前 leaf 的父链，使用最新 compaction 的 summary 和保留区间，再加后续记录。已有 branch summary 与 custom message 会投影为 Provider 输入；普通 custom 状态和 label 不进入模型上下文。

读取既有压缩和分支记录不需要执行压缩、导航或 fork。M4 的修改操作继续保留 Capability Stub。

## 运行与复核

```sh
go run ./examples/session-interop
go test ./codingagent -run 'Historical|SessionLarger|Issue72'
go test ./cmd/pig -run '^TestPigProcessesContinueHistoricalSessions$'
go test ./internal/parity -run '^TestSessionInteropParity$'
node --experimental-strip-types parity/oracle/session-interop.mjs /path/to/locked-pi --check
```

示例真实打开 v2、恢复 runtime、续聊、保存类型化 custom/summary 与开放消息，再重开 v3。可选位置参数指定输出文件（会覆盖该文件），供独立 Pi reader 消费。

Oracle 同时使用 Pi 的正式 writer/reader 和 Pig 示例的正式 reader/writer。`session-interop.json` 保存 Pi 写出的全部 entry 及历史输入的打开、上下文和追加结果；`session-interop-pig-writer.json` 保存 Pi 正式 reader 和模型输入转换对 Pig 文件的观察，覆盖公开 Go 消息构造器及可选字段的缺失/null 区别。只归一化明确声明的随机 ID、引用和生成时间；稳定历史字段与消息内容逐值比较。Go 公共边界重放 fixture，真实 CLI 与 SDK 测试验证上下文确实到达 Provider，并能继续保存。
