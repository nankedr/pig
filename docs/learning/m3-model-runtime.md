# M3.7：统一模型选择和请求运行时

`NewModelRuntime` 把固定 Catalog Snapshot、已有 `ai.Models` 和 canonical `CredentialStore` 组装到一起。`ModelRegistry` 提供兼容查询外观，Session services 持有同一个 runtime。CLI 与 SDK 经该 runtime 执行现有 DeepSeek 文本和 Tool continuation；显式注入 Provider/StreamFunction 的 SDK 用法继续保留。

```sh
go run ./examples/model-runtime
go test ./internal/parity -run '^TestModelRuntimeParity$'
go test -race ./codingagent -run '^TestModelRuntime77'
go test ./cmd/pig -run '^TestPigModelRuntime77$'
pig --list-models deepseek
pig --offline --models 'deepseek/*:high' --no-tools -p 'hello'
```

示例使用内存凭证和注入 transport，不需要 key、不发送网络请求。真实 `pig` 命令需要配置 DeepSeek 凭证，`--offline` 仍允许显式推理。

## 模型和 thinking 的来源

CLI 显式模型最优先，支持大小写无关的 `provider/model`、模糊 ID/名称与 thinking 后缀。已知 Provider 下不存在的 ID 按固定 Pi 从默认模型复制元数据，再使用自定义 ID，并返回 warning；未知 Provider 或无法匹配的裸 ID 明确失败。多个 Provider 的裸 ID 相同时，仅唯一已认证的匹配可以自动选中。

新 CLI 会话可用 `--models` 的第一个可用模型；已有 Session 恢复优先于 scope 和 settings。SDK 未指定 Model 时先恢复已有 Session，再读 settings 默认值，最后按固定 Pi 的 Provider 默认顺序 fallback。SDK 的 `ScopedModels` 保留为会话 scope，不改变 SDK 的初始选择。thinking 按显式参数、恢复记录、有效 settings、medium 的顺序解析，最终由 `ai.ClampThinkingLevel` 限制到模型能力。显式 `--thinking` 优先于模型后缀。

`CreateAgentSessionServices` 在读取文件凭证前处理 Project Trust；调用者注入 SettingsManager 时拥有它的信任状态。项目被拒绝信任不阻止读取用户全局凭证。`CreateAgentSessionFromServices` 与 `CreateAgentSession` 共用组装路径；现有可执行工具为 read，以及显式选择的 write/bash。默认全套工具和扩展资源加载仍未开放。

## 查询与执行的边界

内嵌 `codingagent/data/models.json` 与固定 v0.84.1 Catalog Snapshot 逐字节一致，包含 39 个 Provider 的 1,220 个模型。它只提供查询数据，不启用新的 Provider/API Adapter。运行时对 Provider 的包装仅覆盖静态模型查询，认证与执行复用已有实现。

`GetAvailable` 更新可用性和来源快照，`GetAvailableSnapshot`/`ModelRegistry.GetAvailable` 读取已发布的副本。当前可执行认证范围仍是已有 DeepSeek API key；其他 Provider 的认证和 Adapter 保持明确的 Capability Stub。来源状态仅含 configured/source/label，不含 key；OAuth、ambient auth 和 Provider 注册仍按 M10/M11/M7 推进。查询到模型不等于该模型可执行。

scope 支持模型模式、thinking 后缀和基础 `*`、`?`、字符组 glob，按输入顺序去重并返回无匹配/无效 thinking 诊断。本阶段不支持 minimatch extglob 和 brace expansion。`--list-models` 显示已认证模型及上下文、输出、thinking、图片元数据，并支持搜索。

## Offline 与流结果

`ResolveOffline` 将显式参数与 `PIG_OFFLINE=1/true/yes` 合并，忽略大小写和外围空格。默认没有目录后台刷新；`Refresh` 仅更新本地认证可用性。目录生成、缓存、overlay、ModelsStore 和网络刷新显式保持 Stub，不读取默认 models.json，也不做控制面请求。Offline 不改变 Provider 显式推理契约。

共享 runtime 直接复用已有 `ai.Models` 流处理。公开 SDK 和 Headless 测试比较完整两轮 read 请求；错误保留终态并脱敏，取消保留 aborted 结果。尚未支持的 Adapter 返回 `ModelRuntime.Adapter.<api>`。对等目录 `contract:model-runtime/basic` 区分交付能力和后续范围，源码语义仍锁定 `936aff0`，不将目录版本误称为同一次源码生成结果。
