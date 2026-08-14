# 安全与网络契约

## 1. 威胁模型与非目标

Pig 是以当前宿主用户权限运行的本地 coding agent。Project trust 只决定是否加载项目提供的配置、资源、package 和 extension；它不是工具逐次授权、文件系统 sandbox、网络 sandbox 或远程认证机制。用户信任项目后，项目 package/extension 应被视为可执行宿主代码。

V1 忠实保留 Pi 的默认无工具确认、无内建 sandbox 语义。后续可以在稳定接口上增加可选 policy/approval/sandbox，但不能悄悄改变默认行为或把 trust 按钮描述成完整隔离。

## 2. Project trust

### 2.1 触发资源

只有检测到需要 trust 的项目资源时才进入判定。资源包括当前 cwd 的 `.pig/settings.json`、`extensions`、`skills`、`prompts`、`themes`、`SYSTEM.md`、`APPEND_SYSTEM.md`，以及 cwd 或祖先目录的 `.agents/skills`。只有一个空 `.pig` 目录不触发；用户全局 `~/.agents/skills` 是用户资源，即使 cwd 为 home 也不算项目资源。

未信任时不得读取、解析、安装或执行这些项目资源；仅为判断“是否存在”所需的受限 stat/目录探测例外。用户/全局/CLI 显式指定的资源不属于项目 trust 边界。

### 2.2 判定顺序和默认值

判定按以下优先级完成：

1. 调用方显式 trust override；
2. 没有 trust-requiring 项目资源时直接 trusted；
3. 已加载的用户/全局/CLI pre-trust extension 首个明确 `yes`/`no` 决定；
4. `~/.pig/agent/trust.json` 中 cwd 或最近祖先的保存决定；
5. 全局 `defaultProjectTrust`，默认 `ask`，另有 `always`、`never`；
6. `ask` 且有 UI 时提示用户；无 UI、取消提示或无法选择时 fail closed 为 untrusted。

路径先转绝对路径，并在目标存在时解析 symlink 得到 canonical real path；祖先决定向下继承，最近项优先。用户可只信任当前目录、信任父目录、只在本 Session 信任，或保存拒绝。trust store 位于独立 Pig 目录，使用锁保护直接覆写并保持约定权限；不得从 Pi 自动迁移。

`--approve` 若存在，只表示项目 trust/修改项目资源的批准，不是每个工具调用的 approval。

### 2.3 Pre-trust 安全修正

固定 Pi 快照在启动 trust 判定前会为 `sessionDir` 狭窄读取项目 settings。Pig 已裁决修正：trust 决定前不得读取 `.pig/settings.json` 的任何字段，包括 `sessionDir`；启动所需路径只能来自 CLI、环境或用户全局可信设置。信任后再完整加载项目 settings。这是有意的安全偏离，需用“恶意 pre-trust settings 不产生读取、写入或外联”的测试锁定。

### 2.4 Context 文件例外

`AGENTS.override.md`、`AGENTS.md`、`CLAUDE.md` 等 context 指令沿既定搜索规则加载，不受 project trust gate 保护。拒绝 trust 只禁用上节资源，不能防止这些文件影响 prompt；UI、文档和审计日志必须明确该例外，不能宣传“untrusted 项目不会注入指令”。若未来要改变这一点，需要单独兼容决策。

## 3. 工具、approval 与 sandbox

默认没有逐次工具确认弹窗或 approval 状态机。Extension runtime 到 M7 才执行；届时 extension 可通过 `tool_call` hook 阻止调用，handler 抛错必须 fail closed，不能当作允许。但这是可编程 hook，不等同于内建权限系统。

默认没有 cwd/workspace 文件边界：`read`、`write`、`edit`、`bash` 可使用绝对路径和 `..`，由宿主用户文件权限决定。它们按操作系统语义跟随 symlink；Pig 不提供 symlink escape 防护、realpath containment 或只读仓库保证。取消/超时只负责结束工作和进程树，不收回此前产生的宿主副作用。

Bash 使用宿主 shell 并继承完整进程环境，因而可以读取环境凭证、访问 `~/.pig/agent/auth.json`、运行任意宿主命令和发起网络请求。工具文档不得暗示 sandbox。截断的完整输出写到 OS 临时文件且不隐式删除，敏感输出可能因此留存。

## 4. Package、resource 与 extension 边界

项目 scope 的 package/resource 配置、缺失依赖安装和 extension 执行都必须先通过 project trust；user/global/CLI 显式来源按用户代码处理。M7 前 extension 入口只做发现和 inventory，执行相关操作返回 `ErrNotImplemented`。

npm/git 安装复刻普通依赖与 lifecycle script 语义，不默认加 `--ignore-scripts`。受信任 package 的 manifest resource 路径和 symlink 可以指向 package root 外部；Pig 不额外施加 package-root sandbox 或 realpath containment。仅受管安装目录的目标路径做必要的词法 containment，防止安装器自身覆盖无关位置；这不把 package 变成安全数据。

因此 package 来源、版本和更新是代码供应链决策。安装、更新及缺失二进制获取必须在 UI/日志中可见并服从 offline，不能由资源扫描静默触发。

## 5. 凭证

### 5.1 Canonical store

唯一 canonical 默认 store 是 `~/.pig/agent/auth.json`。父目录权限为 `0700`，文件为 `0600`，并发更新必须加锁，并在锁内直接覆写以保持固定快照的崩溃语义。默认不接入 OS keychain。`pig` 和 `pig-ai` 共用该 store；只有显式 `--auth-path <file>` 才操作其他文件，且永不把 cwd `auth.json` 当作隐式 fallback，即使固定快照的 `pi-ai` 如此实现。

凭证解析顺序为：

1. CLI `--api-key`；
2. canonical auth entry；
3. custom model/provider 配置中的 key；
4. Provider 标准环境变量或 AWS/Google 等 ambient auth。

已存在的 auth entry 拥有该 Provider 的解析权；例如 OAuth refresh 失败应显式失败，不能静默回退环境变量而切换身份。任何 fallback 都必须发生在“该层没有 entry”时，而不是 entry 解析失败时。

### 5.2 引用和命令

Store 保留 Pi 的三类值：直接值、`$VAR`/`${VAR}` 环境引用、`!command` 命令引用；`$$` 和 `$!` 分别用于转义字面量。`!command` 通过宿主 shell 以当前用户权限执行，10 秒超时，丢弃 stderr，并在进程生命周期内缓存结果。这意味着 auth 文件本身是可信可执行配置；不得从未信任项目复制 credential entry，也不得在 pre-trust 阶段运行命令引用。

### 5.3 红线

credential、Authorization header、cookie、token 和其他 Provider secret 不得进入日志、Telemetry、Session、RPC、fixture、golden、错误或 debug dump。prompt/completion、工具参数/输出和文件内容不得进入默认日志、Telemetry 或 debug dump，但功能所需的 Session/RPC 可以承载，测试 fixture/golden 只能使用合成或明确脱敏的内容。错误展示只允许经过结构化脱敏的 Provider/状态信息。

`DEEPSEEK_API_KEY` 只能作为本机或 CI protected secret；不得提交到仓库、写入示例、录制 cassette 或打印到测试输出。由于默认无 sandbox，Bash 和 host-code extension 技术上仍可读取 store 与环境；这是明确风险，不得把文件权限描述成对同用户进程的隔离。

## 6. Offline 与外联

### 6.1 统一开关

CLI `--offline` 与 `PIG_OFFLINE` 使用一个 parser：只有 `1`、`true`、`yes`（忽略大小写和首尾空白）开启，变量缺失及其他值均关闭，不能因“非空”就开启。CLI 显式值优先于环境和配置。

Offline 禁止可选启动/控制面外联：

- model catalog 后台刷新和 version/update check；
- install telemetry、share/version/Radius 等服务调用；
- package 的缺失安装、更新和 binary download；
- 其他非用户显式请求的探测或后台请求。

已有本地 package、内嵌 model catalog 和本地工具仍可用。Offline 不是 OS 网络 sandbox：它不拦截 Bash、host extension 或用户显式发起的 Provider inference；若调用者需要完全断网，必须在进程/容器/系统层实施。命令帮助必须使用“禁用启动和控制面网络操作”，不能承诺全进程断网。

### 6.2 Endpoint 所有权

Pig 默认不得访问 Pi 运营的 `pi.dev`、`radius.pi.dev`、Earendil 或其他 Pi 基础设施。Model catalog 随版本内嵌，后台刷新默认关闭；refresh、share、Radius、release/update 等若实现，只能使用 Pig 自有端点或用户显式配置的端点。V1 不建设 Pig Server。

默认 CI 使用 Faux Provider 和本地 fake server，必须可完全离线。Live DeepSeek 只用于 M1 gate/发布前的受保护小 token smoke test，缺少 secret 时明确 skip，不影响普通单元测试；请求和响应不得进入日志或 fixture。

## 7. 本地 RPC 与 Remote Protocol

`--mode rpc` 是本地 subprocess 的 LF JSONL 控制面，会承载完整 Session、消息和工具数据，没有内建认证、加密、frame size 或租户隔离；不得直接当网络服务暴露。

Remote Protocol 是独立的严格 CBOR framing 协议，默认收发帧上限 16 MiB，但 schema 和帧上限只防止部分畸形输入，不提供身份。ByteTransport/host 必须负责 peer authentication、加密、授权、流控、连接配额和 remote ownership；所有 peer 输入按不可信数据处理。decoder 一旦遇到非法 frame 永久失败并关闭该连接。

Client 的 shared/exclusive lease 只约束同一 Client 内的生命周期，不证明跨 Client 所有权，也不能替代服务端授权。request 默认无隐藏 timeout；作为已批准的 Go 偏离，context 可取消本地 waiter，ID 保留 tombstone 直到迟到 response 或断线，避免将恶意/迟到响应绑定到新请求。

## 8. 必测安全契约

- 空 `~/.pig` 不读取或迁移任何 Pi trust、auth、Session、package 状态。
- 无 UI 且需要 trust 时默认拒绝；祖先决定、canonical symlink 路径和最近项优先可复现。
- 拒绝 trust 后项目 settings/package/extension 无读取、写入、执行或外联，但 context 文件例外仍按约定加载并有明确提示。
- `--approve` 不产生工具 approval；默认工具可越过 cwd、跟随 symlink，测试名称明确这是兼容契约而非安全保证。
- Offline 阻断所有隐式控制面外联和下载，但不伪装成 Bash/Provider 网络 sandbox。
- auth 文件权限、锁、直接覆写的崩溃语义、provider-owned 失败和 `pig-ai` canonical store 偏离有测试；任何输出都不泄漏测试 secret。
- RPC 不被误路由到 Remote decoder；Remote 对超限或畸形输入 fail closed。peer 认证属于 host/transport 集成门禁，只有接入方提供认证策略时才验证未认证连接会被拒绝，protocol/client 本身不得伪称拥有身份机制。
