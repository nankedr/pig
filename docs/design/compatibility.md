# 兼容性设计

## 兼容目标

Pig 的“1:1 复刻”指可证明的功能与语义对等，不是逐行翻译 TypeScript，也不是复制 JavaScript 的表面语法。发生冲突时按以下顺序裁决：

1. 外部可观察行为与语义；
2. Pi 的架构、依赖方向、状态机和算法；
3. Go 的惯用实现；
4. 教学上的简化。

公共 API、架构、持久化、协议、并发或可观察行为的偏离必须先讨论；难以逆转的偏离还需 ADR。私有且行为等价的机械性 Go 实现可以直接采用，但必须在 Parity Catalog 中记录映射。

## Compatibility Surface

| 表面 | 兼容要求 |
| --- | --- |
| CLI | 除主命令名和 Pig 品牌外，复刻子命令、flags、mode、输出结构、退出码和信号行为 |
| Go SDK | 覆盖全部公开语义能力，使用稳定的 Go 命名、Options、interface 和具体类型映射 |
| 配置 | 复刻字段、默认值、合并优先级、校验、动态值解析和错误行为 |
| 持久化 | v3/v4、配置、认证、模型存储等在双方都有读写器时双向语义互操作 |
| 协议 | JSON/JSONL/CBOR/RPC 的值、顺序、校验和状态转换兼容；framing 的规定字节精确兼容 |
| 运行时 | 事件顺序、取消、重试、并发、Tool 结果、资源冲突和失败路径兼容 |
| 平台 | 复刻探测顺序、外部命令、降级、错误和最终六目标行为 |
| 扩展 | V1 最终要求能力对等；源码语言和 ABI 尚未承诺 |

## 语义兼容而非字节相同

JSON key 顺序、等价转义或 CBOR 的合法不同编码不要求完全相同。双方解码后必须得到等价值，并产生等价的校验、迁移和运行结果。当 Pi 和 Pig 都有 writer 与 reader 时，要求双向互操作；固定快照没有 writer 的方向不凭空增加反向能力。

以下内容仍要求字节或顺序精确：

- 长度前缀、换行边界和其他 framing 规则；
- 协议规定的 discriminator、Wire Identifier 和消息顺序；
- 参与签名、哈希或缓存 validator 的原始数据；
- 对端按字节解释而非按值解码的字段。

测试只归一化时间、UUID、临时绝对路径等明确不稳定字段，不用宽松归一化掩盖真实差异。

## 语言映射

- package 名小写，导出名使用 Go PascalCase；构造函数优先 `NewX`，保留有领域含义的 `CreateX`。
- TypeScript overload 映射为独立方法或 Options struct。
- Promise 型阻塞操作接收 `context.Context`；不另造 Go 版 `AbortSignal`。
- 只有真实 absent/null/zero 三态字段使用指针或小型 `Optional[T]`。
- 可替换 seam 使用消费方需要的窄 interface；Message、Event、Options、record 和默认有状态实现使用具体类型。
- Header override 必须区分“不提供”“提供值”“显式删除”；payload hook 必须区分“不替换”和“替换为空值”。

OpenAI Chat Completions 的完整类型和 options 在 M0 就声明，并逐字段映射。M1 只把核心路径从 Stub 推进为实现，M10 再完成高级 options、Provider compatibility flags 与剩余协议分支。任何已声明但尚未支持的字段都必须显式失败；禁止静默忽略。

## 固定快照的兼容怪癖

可观察怪癖默认也是 Compatibility Surface，除非已有 ADR 明确批准偏离。已确认的例子包括：

- v3 reader 跳过坏行；
- Harness v4 不能读取 v3；
- v4 wire 以固定快照代码为准，而不是未来设计文档；
- JSONL RPC 输入不先做完整 schema 校验，并可并发处理请求；
- 资源目录不排序，冲突使用 first-win；
- Interactive 模式的部分终止信号返回 0；
- 默认并行 Tool 批次无并发上限；
- v3/v4 Session 没有 Pig 自行增加的默认总大小限制；
- Telemetry 内存参考实现无界保留 span 与 event；
- Bash 截断输出对应的完整临时文件不会在 Tool 返回时隐式删除。

每个怪癖必须有 Parity Case 和 Catalog 证据。不能因为 Go 中存在“更合理”的默认实现就自行修正。

## 明确批准的偏离

以下偏离不算遗漏，但必须保留 ADR 和验证：

- 跨 goroutine 发布不可变 event snapshot，不复刻 JavaScript 共享引用的后续 mutation。
- Client 请求允许调用方 context 取消本地 waiter，并用 tombstone 吸收迟到 response；固定 Pi Client 本身没有 request cancellation API。
- 统一解释 Offline Mode：仅 `1/true/yes` 开启，其余值或缺省关闭，不复刻 Pi 各路径对 `PI_OFFLINE=0` 的矛盾判断。
- Pig 在 Project Trust 决定前不读取 trust-sensitive 项目 `sessionDir`，修复 Pi 固定快照的启动安全缺口。
- Pig 使用自身品牌、`.pig` 和 `PIG_*`，不读取 Pi 的活动 `.pi` 状态。
- `pig-ai` 使用与 `pig` 共享的 canonical auth store，而不是固定 `pi-ai` 的 cwd `auth.json`。
- 不默认连接或归因到 `pi.dev`、`radius.pi.dev`、Earendil release 等 Pi 运营服务。
- V1 不实现 Pig Server，也不支持 browser/WebAssembly。
- 同一 Provider 的 `ModelsPublication.Update` 是同步 generation commit；为弥合 JavaScript 单线程 turn 与 Go 可重入阻塞调用的差异，Update 返回前对同一 Provider 再调用 `Models.Refresh` 会返回结构化错误，不启动新一代 refresh。见 ADR-0013。

这些偏离应在 Parity Catalog 中标记为已决策 deviation，而不是伪装成 verified 的相同实现。

## 身份与 Wire Identifier

命令、Go API、日志、HTML、临时文件和用户可见文案使用 Pig。为互操作而发布的 Wire Identifier 保持 Pi 值，例如 `pi-messages`、既有 session discriminator 或协议字段。Telemetry schema 默认使用 `pig.*`。

Pi 运营服务中的 client registration、originator、referrer 或 attribution 不能直接照搬，因为那会让 Pig 冒充 Pi。相应功能在没有合法 Pig 身份或用户自定义配置前保持 Capability Stub；兼容测试使用受控服务。

## 本地状态与显式交换

Pig 默认只使用：

```text
~/.pig
<project>/.pig
PIG_*
```

Provider 标准环境变量保持原名。Pig 不提供长期共享 Pi 活动目录的全局 compatibility mode，也不隐式迁移数据。用户通过显式文件路径执行 import、export 或互操作测试时，Pig 才读取 Pi 格式，并将结果复制到 Pig 自有状态或指定输出。

Pi 向子进程公开的 `PI_SESSION_ID`、`PI_SESSION_FILE`、`PI_PROVIDER`、`PI_MODEL`、`PI_REASONING_LEVEL` 等产品变量，在 Pig 中逐项映射为 `PIG_*`；不默认双写旧变量。每个映射都作为批准偏离进入 Parity Catalog 和脚本/example 测试，不能通过无差别字符串替换处理。Provider 行业标准变量和必须互操作的线标识不受该规则影响。

认证仍保留 Pi 的权限受限明文文件、`$VAR`、`${VAR}` 与 `!command` 解析、缓存和错误语义。更安全的 Keychain 可以作为后续可选 backend，不能替换默认兼容契约。

## Project Trust 与主机权限

Project Trust 控制项目设置、技能、模板、主题、包和扩展的加载执行，默认 `ask`；Headless 无法询问时 fail closed。Context File 独立加载，即使项目未受信任也会进入模型上下文。Project Trust 不是逐 Tool 审批、路径沙箱或命令沙箱，内置 Tool 与受信任包继承宿主进程权限。

项目包可执行依赖和 lifecycle scripts，并被视为 host code。文档和 UI 必须明确这一点，不能在不改变兼容承诺的情况下暗加 `--ignore-scripts` 或 package-root 沙箱。

## 错误、资源与性能

命令、RPC、协议、ToolResult 和被 Pi 测试明确断言的诊断属于公共错误契约。Go API 应提供结构化错误及 `errors.Is`/`errors.As`；Node stack、内部 debug 文案和未发布日志不是逐字兼容目标。

Pig 保持并发、顺序、取消、timeout、retry、容量、截断和算法复杂度语义，但不要求 Go 与 Node/Bun 的绝对延迟、内存或二进制大小一致。V1 不允许已知的数量级退化、goroutine 泄漏或 data race。

## 验收原则

“代码看起来相似”不构成兼容证据。每项兼容声明必须指向 Parity Catalog 中的 upstream mapping、Parity Case 与测试结果。Catalog 尚有未解释的 `partial`、`deferred` 或缺失项时，不能宣布 V1 对等完成；唯一例外是固定快照本身明确未实现的 Harness 操作。
