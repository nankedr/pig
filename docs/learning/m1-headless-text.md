# M1 Headless text 与 JSON 路径

Headless Coding Agent 是不启动 TUI、通过最终文本或 JSON 事件工作的产品路径。M1 的 text 和 JSON slice 都可以从真实 `pig` 子进程完成一次或多次 Prompt；两者共用 `RunHeadless` 生命周期，但当前不包含设置、持久化、资源加载或扩展运行时。

## 执行链

```text
pig process
  -> codingagent.Main
  -> CreateHeadlessSession
  -> AgentSessionRuntime
  -> RunPrintMode
     -> text: final Assistant text
     -> json: Session header + projected AgentSessionEvent JSONL
  -> RunHeadless
  -> AgentSession.Prompt
  -> legacy agent.Agent
  -> Provider stream / read Tool continuation
  -> final HeadlessOutcome
```

`CreateHeadlessSession` 只接受显式工作目录、Provider、模型、凭证输入和 Tool 选择。它使用固定内置模型目录和 `NewInMemorySessionManager`，不会读取 M3 的 settings、credential store、trust 或 session 文件。`RunHeadless` 保留最终 `AssistantMessage`、其中的 text blocks 和取消标记；`RunPrintMode` 才负责选择文本或 JSON presenter。

## 运行 CLI

当前产品装配只开放 DeepSeek Chat Completions。Provider 和精确模型 ID 都必须显式给出；key 可以通过参数或标准环境变量提供，参数优先：

```sh
export DEEPSEEK_API_KEY=...
go run ./cmd/pig --provider deepseek --model deepseek-v4-flash -p "Explain this package"
```

也可以从非终端 stdin 读取第一段 Prompt：

```sh
printf 'Summarize go.mod' | go run ./cmd/pig --provider deepseek --model deepseek-v4-flash
```

成功时 stdout 只包含最后一条 Assistant 消息的文本块。参数错误、Provider 错误和 Capability Stub 写入 stderr 并退出 1。SIGINT 会取消同一个 run context、让 Session 形成 aborted 终态并保留已经生成的 partial outcome；真实进程不把 partial 当成功文本写到 stdout，而是退出 130。

## Session-first JSONL

`--mode json` 使用同一个 Headless runner，但 stdout 是单向 JSONL：

```sh
go run ./cmd/pig \
  --provider deepseek \
  --model deepseek-v4-flash \
  --mode json \
  "Explain this package"
```

第一行固定为当前内存 Session 的 v3 header，包含 `type`、`version`、`id`、`timestamp` 和 `cwd`。后续每行恰好一个正式 `AgentSessionEvent`，顺序与进程内 Session 的通知顺序相同。`message_update` 只携带增量 `assistantMessageEvent`；进程内累计 `message` 以及嵌套的 `partial` snapshot 都不会写入 wire。

Provider 终态错误由可解析的 `message_end`/`agent_settled` 事件表达，因此 JSON 进程退出 0；消费者应读取事件终态，而不是只检查退出码。SIGINT 仍会先输出 `aborted` 终态和 `agent_settled`，然后退出 130。参数错误、输出失败和 Capability Stub 退出 1。

JSON 模式不是 Coding Agent JSONL RPC。通过管道传入的 stdin 会整体作为 Prompt：

```sh
printf '{"type":"prompt","message":"hello"}' | \
  go run ./cmd/pig --provider deepseek --model deepseek-v4-flash --mode json
```

这里的 JSON 文本不会被解释为 command；`--mode rpc` 仍返回 `codingagent.mode.rpc: not implemented`。

## Tool 与能力边界

M1 Headless 装配目前只有 `read` Tool：

```sh
go run ./cmd/pig \
  --provider deepseek \
  --model deepseek-v4-flash \
  --tools read \
  -p "Read go.mod and report the module path"
```

指定其他 Tool 会返回对应的结构化 `ErrNotImplemented`。RPC、session persistence、`@file`、skills、prompt templates、themes 和 extensions 仍是精确 Capability Stub；禁用尚未装配的资源发现开关不会激活这些子系统。`PIG_DEEPSEEK_BASE_URL` 只用于把 DeepSeek 请求指向确定性的本地验证端点，不替代正式 Provider 配置。

## 完全离线示例

仓库的文本示例通过 Faux Provider 发起一次真实 `read` Tool continuation，不需要密钥或网络：

```sh
go run ./examples/headless-text
```

示例输出：

```text
Headless read completed.
```

JSON 示例通过同一个 `RunPrintMode` 输出真实的 header 与事件流：

```sh
go run ./examples/headless-json
```

两个示例分别展示共享生命周期和 presenter 边界。Session ID、时间戳与工作目录会随运行环境变化。
