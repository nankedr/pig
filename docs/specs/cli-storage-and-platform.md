# CLI、存储与平台契约

## 1. 身份与兼容原则

用户可见命令为 `pig`、`pig-ai`，状态目录为 `~/.pig`，项目目录为 `.pig`，Pig 自有环境变量使用 `PIG_*`。普通运行和默认路径不得读写 `~/.pi`、项目 `.pi`、Pi 的临时文件前缀或 `PI_*`；只有用户显式 import/export 时才可把指定 Pi 路径当迁移源。Provider 行业标准变量（例如 `DEEPSEEK_API_KEY`、`OPENAI_API_KEY`）不改名。

Pig 不提供隐式 Pi 兼容模式。只有某项既有操作显式接收路径时，Pig 才读取用户指定的 Pi 文件，例如 Session 打开/导出或 `--auth-path`；当前不额外承诺一套通用 migration CLI。空的 `~/.pig` 绝不能因为同机存在 Pi 状态而自动继承 trust、凭证、Session 或 package。

CLI 命令、flag、退出状态、signal 行为和可观察输出形态复刻固定快照，但文案和本地身份改为 Pig。普通 JSON 只要求语义兼容；用于 Pi 对端互通的已发布 wire 字段和 discriminator 保持原值。

## 2. 命令面与阶段性可用性

M1 必须有可完整运行的 `pig --mode text|json`，支持 Faux 和 DeepSeek（OpenAI Chat Completions API adapter）、一次完整对话、流式输出、`complete` 和 `read`。M1 Session 只在内存中；TUI、RPC、Remote、Harness、其余 Provider、extension 执行及尚未完成的 `pig-ai` 操作仍暴露稳定接口，但调用时返回 `ErrNotImplemented`。

未知 long flag 属于 extension surface，不能在通用参数解析层提前拒绝。extension runtime 尚未实现时，应把它稳定地报告为对应 extension 操作 `ErrNotImplemented`，而不是误报拼写错误或吞掉 flag。已知 flag 的缺参、非法枚举等仍是参数错误。

CLI 收到 `ErrNotImplemented` 必须输出稳定错误并非零退出；stub 不得产生成功事件或副作用。SIGINT/取消应沿 run context 传播，保留已经产生的 Assistant 内容并形成 aborted 终态，而不是直接伪造普通失败。

## 3. 三种机器接口不能混用

| 接口 | 方向与编码 | 用途 | 关键边界 |
| --- | --- | --- | --- |
| `--mode json` | stdout 逐行 JSON event | headless 事件消费 | 单向输出，不接收 command，不是 RPC |
| `--mode rpc` | 子进程 stdin/stdout LF 分隔 JSONL | 本机进程控制 coding-agent | 可双向、command/response/event 混流；不是安全远程协议 |
| Remote Protocol | JSON-like value 经严格 schema 校验后编码 CBOR，外加 4 字节大端长度前缀 | M9 跨 transport 客户端互通 | 严格 framing，默认 16 MiB；认证和访问控制由 transport/host 提供 |

`--mode json` 启动时先输出语义兼容的 Session header（M1 也为内存 Session 建立 header），随后逐行输出 `AgentSessionEvent`；它不读取 stdin command。

### 3.1 JSONL RPC

RPC 每行恰好一个 JSON value，writer 始终以 `\n` 结束；reader 兼容行尾可选的 `\r` 和 EOF 前最后一条未换行记录，不把 Unicode line separator 当换行。command 可带 `id`，response 原样关联该 `id`；Agent event 可以在 response 之间异步输出。未知 `type` 返回错误 response，畸形 JSON 返回 parse error response，进程不应把该行强制转换成成功 command。

固定 Pi 快照在 JSON parse 后直接按 `RpcCommand` 分派，没有额外运行时 schema validation；Pig 保留这个宽松边界。line reader 不等待上一条 `handleInputLine` 完成，因此多条 command 可并发处理，不能假设输入顺序等于完成顺序。共享状态仍须遵守 AgentSession 自身串行化和不变量。

RPC 可以传输完整消息、Session、工具参数/输出及 extension UI 子协议，属于可信本地 subprocess 控制面；把 stdin/stdout 直接暴露到网络不会自动获得认证、保密、限流或租户隔离。

### 3.2 Remote Protocol

Remote Protocol 与 RPC 是两个 package、两套 wire format。它先对 plain JSON-like 值做严格 schema 校验，再用 CBOR 编码，并以 4 字节大端无符号长度 framing。收发帧默认上限均为 16 MiB；非法长度、非法 CBOR 或 schema 错误使 decoder 永久失败，不能在同一 stream 上猜测恢复边界。

协议承载 attach/detach、request/response、Session snapshot/revision 等远程生命周期。Server snapshot/response snapshot 是权威状态；客户端忽略旧 revision。Client request 默认无 timeout；作为已批准的 Go 偏离，调用方 context 可取消本地 waiter，request ID 保留 tombstone 直至迟到 response 到达或断线。shared/exclusive lease 只表达同一 Client 内的资源生命周期与占用，不是跨客户端身份、授权或所有权证明。

V1 只实现 Pig Client，并与固定快照的 server package 测试 host/受控 service 及 fake server 做互操作，不假设上游存在 standalone Coding Agent Server，也不实现 Pig Server。ByteTransport 的认证、加密、访问控制、流控和 remote ownership 由接入方显式提供，协议库不得暗示已解决。

## 4. 存储布局

规范路径至少包括：

- `~/.pig/agent/`：Agent 全局状态、`auth.json`、`trust.json`；
- `~/.pig/` 下的 Pig package/resource/cache/Session 路径：按对应模块的固定布局集中管理；
- `<project>/.pig/`：受 project trust 保护的项目 settings、package 和 resource 配置；
- `<project>/.agents/skills` 及祖先同名目录：按安全规范参与 trust 判定，但不因此改名或迁移。

路径由一个 canonical layout API 产生，业务代码不得散落拼接 `~/.pig`。创建含凭证的父目录使用 `0700`，凭证文件使用 `0600`，锁和直接覆写语义见安全规范。

## 5. Session 格式与限制

生产路径是 `Agent + AgentSession + v3 JSONL`，Harness v4 是独立实现：

- M1：内存 Session，不创建会话文件；
- M3.1：实现固定 Pi 快照 v3 的创建、追加写入和显式路径重开；
- M3.2：显式打开 v1/v2 历史与可恢复文件，迁移写回 v3、恢复上下文并验证 Pi/Pig 双向正式 reader/writer；
- M3 后续：继续实现最近会话查找、树/分支和 fork 语义；
- M8：另行实现固定快照的 v4 Harness Session；v4 不读取 v3，也不把未来 Harness 文档中的 API 补进来。

语义兼容要求在双方都存在 reader/writer 的格式上做双向 fixture：Pi 写入 Pig 读取，Pig 写入 Pi 读取。v3 reader 保留快照的容错行为，包括跳过畸形 JSONL 行；不能因 Go decoder 默认严格而把整份可恢复 Session 判死。

固定快照对其接受的 v3/v4 文件没有默认总字节数、单行长度或 entry 数量上限，Pig 不得添加隐藏上限。可用流式扫描、索引和懒加载优化；面向不可信输入的限制只能作为显式新策略加入，并与兼容模式分开。

## 6. `pig-ai` 的 canonical auth 偏离

固定快照的 `pi-ai` CLI 直接读取当前工作目录的 `auth.json`，只覆盖 OAuth provider，并用普通 JSON 读写。Pig 明确不复刻这项安全和一致性缺陷：`pig-ai` 与 `pig` 共用 canonical CredentialStore `~/.pig/agent/auth.json`，遵循同一 provider-owned credential、锁、直接覆写和 `0600` 权限语义；它永远不隐式读写 cwd 下的 `auth.json`。只有显式 `--auth-path <file>` 才改为操作指定文件，该参数不建立 fallback，也不改变后续未带参数进程的 canonical store。

这是已裁决、需测试锁定的 Pig 偏离，不是遗漏的兼容功能。早期尚未实现的 OAuth `login`、`list` 与 help 行为保留接口并返回 `ErrNotImplemented`，不得临时退回 cwd 文件方案；`logout` 属于 Models/Coding Agent 的其他表面，不虚构成 `pig-ai` 子命令。

## 7. Shell、外部命令与临时文件

Bash 工具先使用用户显式配置的 `shellPath`；未配置时，Unix 依次使用 `/bin/bash`、PATH 中的 bash，最后回退 sh。Windows 最终目标保留配置 shell、Git Bash 常见路径和 PATH 探测顺序；legacy WSL 的 stdin transport 也按固定快照验证。M0/M1 的发布 gate 只要求本机 `darwin-arm64`，但接口不能把本机路径写死。

命令通过 shell 的 `-c` 等价方式执行，默认没有 timeout；只有工具参数或调用方显式设置时才超时。子进程继承完整宿主进程环境，并注入 Pig 身份的 Session、Provider、Model、reasoning 等变量；不得注入旧 `PI_*`。取消或超时要终止进程树，而非只取消读取协程。

输出展示保留 Pi 的行数/字节截断和尾部优先语义。发生截断时，完整 stdout/stderr 保存到带 Pig 前缀的 OS 临时文件，结果中告知路径；Pig 不隐式清理该文件，退出、Session 删除或下一次执行都不删除它。

外部工具的探测顺序、命令行、stdout/stderr 处理、exit code、错误和 fallback 属于平台兼容面。需要下载缺失二进制时必须显式可见、服从 offline；不得在普通工具调用中静默拉取。核心必须 CGO-free；必须使用原生能力时隔离为可替换的平台 helper，任何核心例外都需要新 ADR。

## 8. 平台目标

项目使用 Go 1.24、单 Go module、顶层按 `ai`、`agent`、`codingagent` 等 package 分层。只实现 native runtime，不提供 browser、JavaScript 或 WASM target。

阶段 gate 为：M0/M1 只关注开发机 `darwin-arm64` 编译和测试；最终 M13 覆盖 macOS、Linux、Windows 的 amd64/arm64，并覆盖 Termux 安装路径。早期可以把未验证平台标记为未支持，但公共接口、路径抽象和 shell abstraction 必须为最终矩阵保留稳定位置。
