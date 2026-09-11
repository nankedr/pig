# M5 先交付本地资源，延期包生态兼容

2026-09-10 已接受：为优先交付原生 Go Agent，M5 实现本地 Context File、system prompt、Skill、Prompt Template、Theme、项目信任、来源优先级、冲突诊断和重载；Pi package manifest、本地资源包登记、npm/git 管理、依赖及 lifecycle 从 M5 延期，保留在 V1 范围，由 [#99](https://github.com/nankedr/pig/issues/99) 跟踪。资源格式兼容可以独立交付，而完整包安装尚不能兑现可执行扩展的价值；本决策替代 ADR-0009 中“M5 完成 package 获取”的阶段安排，不改变 M7 先决策运行时的要求。

包生态当前未排期，不阻塞调整后的 M5/M6，也不自动安排到 M7/M14；出现具体包需求、团队版本管理需求或 M7 分发需求时再评估。既有包 API 保持明确 Capability Stub，普通本地资源加载不依赖包管理器，不扫描 manifest 后隐式安装；Skill 自身的辅助脚本仍可能依赖外部工具。Catalog 和发布门禁必须区分已交付本地资源与尚未完成的包生态，V1 不能以延期掩盖兼容缺口。依据见[阶段评估](../research/m5-package-ecosystem-assessment.md)。
