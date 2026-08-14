# 扩展系统规范

## 当前承诺

Pig V1 最终要求 Extension Surface 的能力对等，但当前不承诺 Pi JavaScript/TypeScript 扩展无需修改即可运行，也不承诺 Go plugin、WASM、sidecar 或某种脚本语言。

早期阶段只冻结：

- 扩展子系统边界；
- 完整能力清单；
- 公共发现入口和 Capability Stub；
- 稳定的 `ErrNotImplemented` 识别与 CLI 非零退出行为。

以下内容在 M7 专项设计完成前不得冻结：

- 扩展语言与类型绑定；
- 模块加载、解析和分发；
- 进程内或跨进程通信；
- 生命周期、热加载和状态保留；
- callback、并发、取消和错误隔离 ABI；
- UI component 与终端回调表示。

## Extension Surface 范围

Parity Catalog 必须完整盘点 Pi 扩展可以观察、拦截或增加的能力，包括但不限于：

- 注册和执行 Tool；
- 注册 slash command、CLI flag 和快捷键；
- 观察 Agent、session、Tool 与 UI event；
- transform context、拦截 Provider payload/header/response；
- 注册 Provider、model、认证和动态模型刷新；
- 增加 message renderer、编辑器、对话框、状态和其他 TUI 集成；
- 提供 skill、prompt、theme、Context File 以外的资源；
- 读取受支持的 host API、设置、会话和资源状态；
- package manifest、entry discovery、配置、错误和 cleanup；
- Pi 示例和第三方可达 deep import 暴露出的实际能力。

盘点能力不等于复制 TypeScript declaration，也不等于提前设计 Go callback。每个上游 symbol 需要映射到能力、阶段和证据，语言相关细节可以标记为待 M7 决策。

## Capability Stub 规则

扩展尚不可运行时：

- entry 可以被发现、展示和写入 Parity Catalog；
- 未知长 flag 继续流入 Extension Surface，而不是被静态 CLI parser 提前拒绝；
- 调用返回 `NotImplementedError{Module, Operation}`，并匹配 `ErrNotImplemented`；
- CLI 输出稳定诊断并返回非零状态；
- 不加载或执行扩展代码；
- 不联网、不写扩展状态、不发成功事件；
- 不用“已发现”或“已安装”暗示扩展已经生效。

## 分阶段实现

### M0：完整盘点与边界

提取全部 Extension Surface、公开 entry、类型、事件、hook、示例与测试。建立 package/loader/runner/UI/provider 等能力分组和 Stub，但不定义运行时 ABI。

### M1 至 M4：保持可见、明确失败

Headless、RPC 和资源路径遇到扩展能力时保持相同边界。已有核心 Provider callback 和 Agent hook 必须为未来扩展保留语义，但当前不能据此宣称扩展可加载。

### M5：包与资源，不执行扩展

完整实现 npm/git package manifest、安装、更新、依赖、lifecycle script、资源 symlink、Project Trust 和 global/project precedence。包内 skill、prompt、theme 等非扩展资源按 Pi 规则可用；扩展 entry 只发现和登记，仍返回 Stub。

Node、npm、pnpm、Bun 或用户配置 wrapper 只在用户明确触发需要它们的包工作流时成为外部依赖。普通 Pig 运行不依赖这些工具。受信任包及 lifecycle script 是 host code，不受 package-root 沙箱限制。

### M7：先决策，再实现

M7 的第一个门禁是专项调查、grilling 和 ADR，不是编码。只有用户接受运行时方案后，才冻结并实现 ABI，使 M5 已发现的扩展 entry 与完整 Extension Surface 真正运行。

## M7 方案评估维度

候选方案至少比较：

- macOS、Linux、Windows、Termux 的可移植性；
- 安装体积、工具链和分发方式；
- 进程内性能与跨进程序列化成本；
- 崩溃、panic、死循环和内存泄漏的隔离；
- host 权限、Project Trust、凭证和文件访问边界；
- 热加载、卸载、版本升级和状态迁移；
- Go 类型、动态值、stream、async callback 与取消的表达；
- Provider payload/header/response hook 的时序与替换语义；
- Tool update、listener barrier 和并行执行语义；
- TUI component、键盘输入和同步渲染 callback；
- 调试、stack trace、日志和错误归属；
- ABI 版本协商、向后兼容和 capability negotiation；
- Pi 现有扩展的迁移成本与最终能力对等证据。

不能仅因为项目使用 Go 就默认选择 Go plugin；也不能仅为兼容现有扩展默认嵌入 Node。Go plugin、WASM、sidecar、脚本语言或混合方案都必须接受同一组约束评估。

## Provider callback 与扩展边界

`onPayload`、`onResponse`、header transform 等 Provider callback 是 `ai`/`agent` 的公共语义，不属于某一种扩展 ABI。M0 必须声明其完整类型和替换规则，M1 在 Chat Completions 核心路径验证，M10 补齐剩余 API/compat 分支。M7 只决定扩展如何订阅这些既有 seam，不能重新定义其时序。

Callback 可以接触完整 Provider payload、响应状态和 header，可能包含敏感数据。M7 必须明确权限、文档和审计方式；Project Trust 本身不能被描述成数据脱敏或沙箱。

## 验收

扩展阶段只有在以下条件满足后才能标记 verified：

- Extension Surface 的 Catalog 项均有 ABI 映射；
- package discovery 到运行、回调、UI 和 cleanup 端到端可用；
- Pi 的扩展测试与示例均有 Go 侧等价证据或明确迁移说明；
- 异常、取消、并发、reload、版本不匹配和 host crash case 已覆盖；
- 安全和权限模型与实际行为一致；
- 不再存在无解释的 Capability Stub。

若最终方案不提供 TypeScript/JavaScript 源码兼容，文档必须明确迁移路径；能力对等不能被“语言不同”免除。
