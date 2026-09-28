# 按 V1/V2 交付固定基线，并按范围验收

2026-09-28 已接受，依据 [#1 当前决策](https://github.com/nankedr/pig/issues/1)，由 [#130](https://github.com/nankedr/pig/issues/130) 同步仓库。

## 替代关系

本决策替代 ADR-0001、ADR-0003、ADR-0009、ADR-0034 和模型目录规范中“全部能力在 V1 完成”的交付版本、旧 M7→M14 顺序及 v0.7.0–v0.13.0 排期；同时更新 ADR-0005/0011 的阶段映射。原文保留为历史记录。固定基线、七模块最终范围、运行时研究先于扩展 ABI、语义和验证标准继续有效。

Code Baseline 仍为 `936aff00918de1187f085f123c2812d8f2d67745`；Catalog Baseline 仍为 v0.84.1 source tar（`53fa77ccd8a279eb87e92294ef3687b03ff80112`），双来源说明沿用 ADR-0014。V2 先完成同一基线，升级必须另行决策。当前 DeepSeek 服务能力和模型配置是独立来源，不能替换历史 Snapshot 或冒充 Pi Oracle 结果。

## 交付范围

| 版本 | 承诺 |
| --- | --- |
| V1 / v1.0.0 | 保留 M0–M6 已交付能力；仅承诺 DeepSeek Provider，保留 Chat Completions 并新增 Responses 线协议；用户图片、工具结果图片、处理、上下文、持久化/恢复、终端展示与导出；公开 SDK、headless、Interactive、JSONL RPC 一致；darwin-arm64 发布验证 |
| V2 / v2.0.0 | 其他厂商与认证、OpenAI 官方/Azure/Codex、剩余协议与完整目录；图片生成/编辑与剩余跨模块路径；Harness v4、Remote Protocol/Client、包生态、六目标与 Termux；最后完成扩展，再收口全量对等 |

最终完整移植 `ai`、`agent`、`codingagent`、`telemetry`、`tui`、`protocol`、`client`。Production legacy Agent/v3 与固定快照 Harness/v4 双轨不变；Server、evals、sqlite-node、browser/Worker/WASM 仍排除。扩展不自动承诺 TS/JS 源码兼容；运行时专项调查、grilling 与接受的新 ADR 必须先于 ABI 实现。

M0–M6（#2–#8）已关闭。新功能前沿是 V1 Responses：#130 范围同步 → #12（#131–#133）→ #14（#134–#136）→ #16。V2 按真实依赖推进，扩展 #9 是最后功能阶段，#127 收口；旧 M 编号仅保留范围溯源，不表示新执行顺序。既有 release、fixture、freeze 记录及 M6 人工豁免原样保留，不能追溯改写为新版本通过。

## 追踪与验收

交付版本独立于 Capability Status。Catalog 的 [版本范围](../../parity/delivery-scope.json) 与 [Responses 服务矩阵](../../parity/responses-matrix.json) 是 Catalog 的组成部分，关联原条目；不得因移入 V2 提升状态、删除缺口或把整个 partial 条目算作完成。共同契约分别记录 V1 分支与 V2 剩余分支。

Responses 矩阵分开记录标准线协议、DeepSeek 支持/忽略/不支持、Pig 实现状态和验证来源。接通 DeepSeek 不代表支持 OpenAI 厂商或全部 Responses 功能。固定 Pi 明确的 no-op 必须按 Oracle 验证，不能误标 Stub；额外 DeepSeek 服务差异单独记录。无状态服务由本地 Session 完整重放历史，不能依赖服务端 response ID 保存上下文。

V1 gate 只要求 V1 范围语义闭环，同时回归全部已交付能力并验证 V2 Stub 无网络、持久化、成功事件及伪成功值。V2 gate 才承担完整固定基线对等。两版都保留错误/取消部分结果、并发与顺序、Session 恢复、信任/离线/凭证/状态隔离和许可要求。具体规则见[版本验收](../specs/versioned-release.md)。

普通测试离线、不要求凭证；V1 freeze/release 必须以受保护凭证完成 DeepSeek 文本、工具 continuation 和视觉冒烟，缺密钥必须失败，未执行不能当作已验证。官方资料查阅日期、endpoint、模型 ID、配置及实际 smoke 结论与历史快照分开记录。新增文档与配置不构成功能或发布验证证据。
