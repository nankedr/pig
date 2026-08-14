# 模型目录规范

## 两种 Catalog 不可混淆

- **Parity Catalog**：所有源码能力、映射、状态和证据的唯一权威。
- **Catalog Snapshot**：固定基线使用的不可变模型目录数据制品。

Catalog Snapshot 是 Parity Catalog 中受追踪的一组 artifact，但它本身不记录 Pig 的实现完成度。

## 为什么源码 commit 不够

Pi commit `936aff00918de1187f085f123c2812d8f2d67745` 没有提交完整的 chat model 真实数据；发布过程会通过网络生成模型 shard。只锁定源码无法重现当时使用的 Provider、model ID、API、价格、context window、max tokens 和 compatibility metadata，因此 Parity Baseline 必须同时锁定 Catalog Snapshot。

不能拿调查当天的实时目录冒充 commit 当天数据，也不能在普通 build 中联网后把结果当成固定基线。

## Snapshot 获取顺序

1. 优先寻找由该 commit 生成、不可变且可校验的历史发布 artifact。
2. 若不存在，使用该 commit 的生成器进行一次受控抓取。
3. 受控抓取必须记录真实抓取时间，并明确它晚于源码 commit。
4. 对数据、manifest、原始响应和工具链计算 hash，人工审查来源与许可。
5. 锁定结果后，任何变化都必须通过显式 baseline upgrade；运行时 refresh 不得修改 Snapshot。

## Snapshot 内容

Snapshot 至少包含：

- 按 Provider/API 划分的规范化 chat model 数据；
- image model 基线或其独立 manifest；
- 生成时间、生成器 commit、工具版本和输入来源；
- 每个 artifact 的 SHA-256；
- Provider、model 数量及完整性 manifest；
- 手工修正、过滤、派生字段和兼容标记的来源；
- 许可证、转载条件和第三方归属；
- 原始网络响应，或在许可/体积不允许提交时对应的不可变内容地址与 hash。

Snapshot 不包含真实 API key、OAuth token、cookie、账号标识或请求日志中的敏感 header。

## 普通构建与测试

普通 build、test 和 release 只读取已锁定 Snapshot，不隐式刷新，也不要求网络、Node 或 Provider 凭证。M0 必须校验 manifest、hash、Provider 引用、model 唯一性和 schema；校验失败立即终止，不能回退到 live fetch。

M1 的 faux 与 DeepSeek/OpenAI Chat Completions 路径从 Snapshot 或明确的测试 fixture 获得模型定义。真实 DeepSeek smoke 验证服务连通与基本行为，不更新价格、窗口或 compatibility 基线。

## V1 仍需复刻生成管线

Snapshot 只是早期可复现输入，不能代替 Pi 的模型目录功能。V1 必须用 Go 复刻：

- chat 与 image 目录抓取器；
- Provider 数据过滤、修正、合并和派生；
- schema 校验、manifest 和 catalog diff；
- remote overlay、cache、ETag、Last-Modified、TTL 与失败语义；
- Radius 动态目录；
- 用户 `models.json` overlay 及固定快照的优先级；
- 相同录制网络输入下 Pi/Pig 的语义等价输出。

生成器测试使用录制响应和合成凭证。只有专门的 catalog capture/conformance 工作允许访问真实来源；抓取结果先进入审查区，不能自动覆盖已发布 Snapshot。

## 运行时目录层次

运行时区分以下数据：

1. 内嵌 Catalog Snapshot，提供离线 baseline；
2. 持久化 remote overlay/cache；
3. 用户显式 `models.json` 配置；
4. Provider 自身的动态目录，例如 Radius。

合并键、覆盖优先级、过期判断、404/501/304、瞬态错误、validator 和 cache 保留行为必须以 Pi 固定快照及 Parity Case 为准。文档不能用“后写覆盖”之类简化规则替代实际算法。

remote endpoint 从接口设计开始即可配置，但 Pig 没有自有目录服务时默认禁用网络 refresh。Pig 不请求 `pi.dev`，不把 Pig 流量归因给 Pi，也不复用 `radius.pi.dev` 作为默认值。受控测试服务必须验证完整 HTTP 和缓存契约。

Offline Mode 开启时，模型目录、版本、包更新和工具下载使用同一开关并全部禁止可选网络访问。Provider 推理请求是用户明确发起的核心操作，不因“目录 refresh”概念而被混入后台刷新。

## 阶段门禁

| 阶段 | 模型目录要求 |
| --- | --- |
| M0 | 锁定源码与 Snapshot；提交 manifest/hash/来源；生成对应 Parity Catalog 项；离线完整性校验通过 |
| M1 | faux 与 DeepSeek 使用固定模型定义；Chat Completions 逐字段 matrix 完整；不做后台目录刷新 |
| M10 | 完成 OpenAI 系、chat 生成器、校验、diff、remote cache/overlay 和剩余 Chat Completions Stub |
| M11 | 完成其余 Chat API、Provider、认证与完整模型 compatibility matrix |
| M12 | 完成 image model/generation 目录与消息图片相关行为 |
| M14 | 所有 Snapshot、生成器、runtime overlay 和许可项均有 verified 证据 |

模型目录的字段级 compatibility matrix 与 Chat Completions matrix 都属于 Parity Catalog 或其生成视图，不单独维护手工“完成”标记。

## Baseline 升级

升级 Pi baseline 时必须显式修改 upstream lock 和 Catalog Snapshot，重新运行提取、生成、hash、diff、Oracle case 与许可审查。禁止只更新源码 commit 或只抓取新模型数据。旧 release 必须仍能定位其对应的两件基线制品。
