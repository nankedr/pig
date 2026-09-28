# Responses 能力与 DeepSeek 服务边界

V1 适配 Responses 线协议并接通 DeepSeek，不承诺 OpenAI 官方 Provider、Azure、Codex 或全部 Responses 功能。它们与剩余兼容分支由 V2 #129/#13 负责。版本决策见 [ADR-0043](../adr/0043-versioned-delivery-and-gates.md)。

[机器矩阵](../../parity/responses-matrix.json) 是 Parity Catalog 的服务兼容组成部分；[可读矩阵](../../parity/reports/responses.md) 从它和 `catalog.jsonl` 生成。每行分别列出标准协议、DeepSeek 支持状态及实际行为、Pig Capability Status、交付版本/票和待补证据。#130 只登记规则，不提升任何实现状态。共享 Catalog 锚点只是初始盘点；实施前必须按字段/行为拆分证据，不能因为一个入口通过就整体提升所有矩阵行。

官方文档查阅日期为 **2026-09-29**，并非官方发布日期。来源为 [DeepSeek Responses 指南](https://api-docs.deepseek.com/guides/responses_api/)、[请求参考](https://api-docs.deepseek.com/api/create-response/)、[视觉指南](https://api-docs.deepseek.com/guides/vision/)和 [OpenAI 会话状态指南](https://developers.openai.com/api/docs/guides/conversation-state)。当前资料支持 `deepseek-flash`、`deepseek-v4-pro` 文本请求，视觉配置使用 `deepseek-flash`；endpoint 为 `https://api.deepseek.com`，凭证变量为 `DEEPSEEK_API_KEY`。这是官方支持配置登记，尚无本票的 Pig live 验证；不修改历史 Catalog Snapshot 或旧 smoke 记录。

## 矩阵的解释规则

- 标准协议支持服务端状态管理不代表 DeepSeek 提供相同能力。DeepSeek 无状态，Pig 必须由本地 Session 管理并重放完整历史、工具结果和适用 reasoning；不能依赖 `previous_response_id` 或 `conversation` 恢复上下文。
- 服务“支持”“忽略”“不支持”与 Pig“已实现/未实现”是不同轴。不支持的顶层参数也可能被服务接受后忽略；请求未报错不能成为能力已支持的证据。
- `inventoried` 表示行为尚未实现；`scaffolded` 仅有契约/Stub；`partial` 必须限定分支；`implemented` 尚待充分对等证据；只有绑定所需证据的 `verified` 才能宣称相应验证通过。
- 固定 Pi 明确的 no-op/ignore 必须由源码和 Oracle 确認，按相同无操作语义验证，不能误标 Stub。DeepSeek 额外忽略行为单独登记；尚未实现的 Pig 非默认分支仍明确失败，不能借服务忽略而静默丢弃。
- developer 角色、reasoning/summary/encrypted content、并行工具开关及采样参数等差异必须逐项取证；不支持路径不能制造成功事件。新服务模型/图片能力不属于历史 Pi Snapshot 的事实。

#131 覆盖协议请求、SSE、错误/取消/重试及终态；#132 覆盖函数工具、reasoning、历史重放和服务限制；#133 接通可恢复 Coding Agent 与各公开入口。#134–#136 补齐图片与展示。剩余高级选项、托管/custom 工具和其他厂商路径归 V2，但 V1 必须证明相应未实现边界。逐字段补齐时保留 absent/null/false/zero/empty/default、hook/header、usage/cache、终态和副作用语义。

普通测试用本地服务和 fixture；固定 Pi 共同语义用 Oracle，额外 DeepSeek 能力用独立服务 conformance。V1 freeze/release 另要求受保护文本、工具 continuation、视觉 smoke，详见[版本验收](versioned-release.md)。

矩阵校验与生成均离线：`go test ./internal/catalog` 检查版本、Catalog 引用和报告同步；修改机器矩阵后运行 `go test ./internal/catalog -run TestResponsesMatrixReport -update` 重生成可读矩阵。不要手改生成报告或从模型支持状态推导 Pig 完成状态。
