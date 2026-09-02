# M1 Headless text 路径

Headless Coding Agent 是不启动 TUI、通过文本或 JSON 事件工作的产品路径。M1 的 text slice 已经可以从真实 `pig` 子进程完成一次或多次 Prompt；它和后续 JSON 模式共用 `RunHeadless` 生命周期，但当前不包含设置、持久化、资源加载或扩展运行时。

## 执行链

```text
pig process
  -> codingagent.Main
  -> CreateHeadlessSession
  -> AgentSessionRuntime
  -> RunPrintMode
  -> RunHeadless
  -> AgentSession.Prompt
  -> legacy agent.Agent
  -> Provider stream / read Tool continuation
  -> final HeadlessOutcome
```

`CreateHeadlessSession` 只接受显式工作目录、Provider、模型、凭证输入和 Tool 选择。它使用固定内置模型目录和 `NewInMemorySessionManager`，不会读取 M3 的 settings、credential store、trust 或 session 文件。`RunHeadless` 保留最终 `AssistantMessage`、其中的 text blocks 和取消标记；`RunPrintMode` 才负责把成功结果呈现为 stdout。

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

## Tool 与能力边界

M1 Headless 装配目前只有 `read` Tool：

```sh
go run ./cmd/pig \
  --provider deepseek \
  --model deepseek-v4-flash \
  --tools read \
  -p "Read go.mod and report the module path"
```

指定其他 Tool 会返回对应的结构化 `ErrNotImplemented`。JSON、RPC、session persistence、`@file`、skills、prompt templates、themes 和 extensions 也仍是精确 Capability Stub；禁用尚未装配的资源发现开关不会激活这些子系统。`PIG_DEEPSEEK_BASE_URL` 只用于把 DeepSeek 请求指向确定性的本地验证端点，不替代正式 Provider 配置。

## 完全离线示例

仓库示例通过 Faux Provider 发起一次真实 `read` Tool continuation，不需要密钥或网络：

```sh
go run ./examples/headless-text
```

示例输出：

```text
Headless read completed.
```

该示例直接调用共享 `RunHeadless`，因此也展示了后续 JSON presenter 将复用的生命周期边界。
