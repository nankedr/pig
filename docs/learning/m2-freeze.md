# M2 集成与冻结

Issue #70 集成 #60–#69 的 Legacy AI、Agent 和 Telemetry 公共链路。CLI 的 `pig --version` 与 Go SDK 的 `codingagent.Version` 统一为 `0.2.0`。M1 的 text/json Headless、Chat Completions 核心、Tool continuation 和取消语义继续受全量回归门禁约束。

## 离线门禁

```sh
make m2-gate
```

该命令依次运行全量 Go 测试、全量 race、vet、CGO-free darwin/arm64 构建、全部已交付示例，以及 20 次带随机测试顺序的 M2 并发回归。普通测试使用已提交 fixture、注入 transport 和本地受控 HTTP 服务，不需要 Pi、Node 或真实凭证。不要给普通门禁注入真实 Provider 密钥；live smoke 无密钥时明确跳过。

重复回归覆盖 deferred fetch/cancel、Agent 队列、proxy、compat registry、Session Resource cleanup 和并发 Telemetry。proxy 的取消用例要求 Fetch、阻塞 reader 和 producer 全部退出；队列、deferred 和 Tool 用例通过完成通道与 listener barrier 验证运行生命周期，超时视为失败。

## 完整冻结

从干净 Pig checkout 执行：

```sh
make m2-freeze \
  PIG_PI_ORACLE_CHECKOUT=/path/to/prepared/pi \
  PIG_PI_SOURCE_CHECKOUT=/path/to/pristine/pi
```

两个 Pi checkout 的准备方式见 [M0 冻结门禁](m0-compatibility-skeleton.md#冻结门禁)。它们都必须位于 Code Baseline `936aff00918de1187f085f123c2812d8f2d67745`；prepared checkout 预装依赖和 dist，pristine checkout 不得包含修改、未跟踪或忽略文件。Pig 的 TypeScript 提取器也需要预装锁定依赖。门禁本身不执行下载、安装或构建 Pi。

冻结在离线门禁之外重跑全部 Pi Oracle、源码/API/图片目录 drift，以及要求 `DEEPSEEK_API_KEY` 的真实 DeepSeek text + read continuation 冒烟。缺少 checkout、依赖、密钥或存在 Pig 工作区修改时必须失败。`m2-oracle` 补齐早期入口遗漏的 usage/cost/cache 与 proxy 两个 Oracle；不改动固定基线或 fixture。

## Catalog 与冻结边界

`internal/m2gate` 固定 M2 的 165 个 Catalog ID，拒绝未解释的 `inventoried`、`scaffolded` 或 `deferred`，要求每项至少绑定一条完整、可定位的 Go 执行证据。`partial` 必须同时记录支持范围、剩余范围和说明；Catalog 的现有 fixture/hash 验证继续由各切片门禁执行。CLI 与 SDK 的版本也在这里核对。

本次收口为 21 个尚未绑定证据的矩阵项增加公开 SDK 值状态检查：thinking budgets、逐级 thinking map、signature/redacted、Deferred Handle，以及非数组 reasoning details 和 pending signature 的一次消费。测试通过后绑定到原 Catalog ID，保持 `partial`：Go 验证没有冒充尚未捕获的固定 Pi 差分证据。签名的真实适配器解释、自定义 deferred data/expiry 的 wire 行为继续按已有 Adapter 分期处理。

既有 `partial` 也保留真实边界，包括未穷举的 thinking 字段组合、已登记的 Pi/Pig usage 差异、compat 的后续真实协议/认证/图片分支，以及 broad Agent/Telemetry 条目原有的未实现部分。M1 已存在的 `PrepareNextTurn`、默认 stream 安装和 Telemetry schema 专用 helper 不因本票获得完成声明。本次冻结的是 #60–#69 已交付且有证据的公共行为，不把类型可编译等同于整个 broad contract 已实现。

M10/M11 的其他真实协议和认证、M12 图片路径继续明确返回对应 Capability Stub。没有通过移动 Catalog milestone 或批量设置 `verified` 消除剩余工作。

## 导航与发布

十条链路的 Pi、Go、fixture 和示例对应关系见 [M2 源码导航](../mappings/typescript-to-go/m2-freeze.md)。既有 API snapshots 和编译期表面检查随全量门禁重放；本票不增加或改变 SDK 类型。新增 [proxy 示例](../../examples/agent-proxy/main.go) 通过注入精简事件运行，无外部网络。

发布顺序为：提交候选代码 → 代码审查 → 干净 checkout 的 `m2-freeze` → 将已验证的同一 commit 标记为 `v0.2.0` 并发布。发布说明见 [v0.2.0](../releases/v0.2.0.md)，门禁未完成时不能创建发布 tag。
