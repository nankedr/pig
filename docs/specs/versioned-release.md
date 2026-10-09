# V1/V2 冻结与发布验收

本规范执行 [ADR-0043](../adr/0043-versioned-delivery-and-gates.md)，替代旧“全量 V1”的发布门槛。交付归属由 [Catalog 版本范围](../../parity/delivery-scope.json) 给出；Capability Status 和证据继续来自 `parity/catalog.jsonl`。V1 和 V2 分别发布 v1.0.0、v2.0.0，旧 M 编号不再绑定新 release 号。

## V1 gate

- 完成 DeepSeek Chat Completions 保留路径及 Responses：文本、工具 continuation、reasoning、多轮完整历史重放、错误/取消的部分结果，覆盖 SDK、headless、Interactive 与 JSONL RPC。
- 图片输入、工具结果图片、处理与上下文、v3 持久化/恢复、附件交互、终端预览及 HTML 导出完整闭环；不能以输入看图成功代替图片生成/编辑。
- 回归 M0–M6 全部已交付行为，包含超出上述产品承诺但已经有证据的通用能力。历史 `make m6-gate` 是回归入口；新 Responses/图片必须另补确定性用例和最高层公开边界验收。
- V2 缺口保持可见；尚未实现的非默认分支返回结构化 `ErrNotImplemented`，验证无网络、持久化、成功事件或伪成功值。固定 Pi 的 no-op/ignore 与相同未实现结果分别建 case，不混为 Stub。
- V1 范围内的每个分支都有可复核证据。跨版本的 `partial` 条目允许继续存在，但必须指明 V1 已验收分支、V2 剩余分支及跟踪票；不能仅凭整条状态判定 V1 通过，也不能用改版本掩盖 V1 缺口。
- `darwin-arm64` 原生测试、无 CGO 构建、race、进程/终端/安装与公开 SDK 验证通过；其他目标交叉编译不能冒充正式支持。新的终端图片行为需要自身自动或人工证据，M6 的版本绑定人工豁免不能自动继承。
- CLI/SDK、示例、中文学习资料、TypeScript → Go 导航、API snapshot、来源许可及 Catalog 同步。Pi 共同语义使用固定 Oracle；额外服务能力使用独立 fixture/conformance，不伪造 Oracle 结果。

全厂商认证、完整目录生成器、Harness、Remote、包生态、其他平台、扩展和图片生成/编辑的未完成状态不阻塞 V1；对应已交付分支回归与 Stub 安全边界仍是 V1 必测项。

## 受保护的 freeze/release

普通测试使用 Faux、已提交 fixture、本地 HTTP/SSE 假服务和合成凭证，默认离线，不要求 Provider secret。freeze/release 必须从干净的确切 commit 验证基线、源码/API drift，并使用受保护 `DEEPSEEK_API_KEY`：

| 验证 | 要求 |
| --- | --- |
| 文本 | 新 Responses 文本流/终态与既有 Chat Completions 回归；受限 token，不用自由生成文本作精确 golden |
| 工具 | 函数调用 → 真实 ToolResult → continuation；验证 call_id 与本地历史重放 |
| 视觉 | 使用官方支持的视觉模型，合成图片经用户输入与 read/ToolResult 进入后续模型上下文，并验证恢复后的视觉路径 |

缺凭证、模型未确认支持、服务失败、未执行或被 skip 都不能记为通过；release 必须阻断，不能用离线 fake 代替 live 证据。当前配置和文档日期见 [Responses 矩阵](../../parity/responses-matrix.json)，其中 `smoke_verified: false` 只是本次资料登记时尚未实测的说明；发布通过以绑定 commit 的新执行证据为准。

证据必须记录：commit/tag、平台、执行命令和强制 live 开关、UTC 执行时间、官方文档查阅日期、endpoint、明确 model ID、API、thinking/图片等实际配置，以及文本/工具/视觉分别通过、失败或未执行的结果。不得存储真实 secret、原始 header、用户内容；使用合成输入并记录可复核的语义断言。模型改名、服务变更或参数差异另留 provenance，不修改历史模型快照、fixture、价格或能力标记。

本票 #130 定义规则；#131–#136 实现能力及用例，#16 通过 `make v1-gate`、`make v1-freeze` 和 `scripts/v1-release.py` 接通执行入口。范围及最高层契约关联见 `parity/v1-acceptance.json`；`scripts/v1-live.py` 强制文本/工具/视觉及原生剪贴板并拒绝 skip。完整发布结论以绑定 commit 的 Release 附件为准。复现见 [V1 冻结与安装](../learning/v1-freeze.md)。`make m1-live-smoke` 仅覆盖历史文本/工具，不可冒充新视觉门禁。

## V2 gate

#127 要求约定兼容面内同一固定基线的完整七模块对等：所有剩余 Provider/认证/协议/目录、图片生成编辑、Harness/Remote/平台/扩展自身图片集成、包生态、六目标与 Termux 均关闭缺口。无证据的实现、无 ADR 的延期、未解释 partial 或遗漏 artifact 均阻断；固定 Pi 自身未实现的操作以相同未实现语义验证，不按未来设计补完。

V2 回归 V1 和全部后续交付，保留受保护服务 smoke、平台/终端、CLI/SDK 安装、API snapshot、许可、全量 Catalog 与跨模块闭环。扩展是最后功能阶段，运行时调查、grilling 和接受的 ADR 必须先于 ABI。#127 只收口已登记缺口，不新增功能。升级 Code/Catalog Baseline 仍需独立决策。

## 历史证据

M0–M6 的已关闭 Issue、v0.x 发布说明、验证制品、hash、fixture 和人工豁免保持原状，继续用于历史复现。新版本的范围划分不撤销旧结论，也不能把历史“未执行/豁免”改成通过。
