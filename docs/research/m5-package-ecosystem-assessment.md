# M5 包生态兼容阶段评估

日期：2026-09-10。状态：结论已由用户采纳，正式决策见 [ADR-0034](../adr/0034-defer-package-ecosystem.md)，延期范围由 [#99](https://github.com/nankedr/pig/issues/99) 跟踪。

## 建议

在“优先交付原生 Go Agent”的当前目标下，M5 先实现本地 Context File、system prompt、Skill、Prompt Template、Theme、项目信任、资源优先级和重载。延期 Pi package manifest 解析、受管本地包、npm/git 安装更新、依赖和 lifecycle。保留资源格式的对等目标，保留既有包 API 的明确未实现状态。

延期依据是当前收益和依赖关系，而非 Go 不能使用 npm。M5 完成后，可在 M7 扩展运行时决策时重新评估分发需求；这不自动承诺 M7 实现包生态，也不取消 V1 的既定兼容目标。

## 核实的事实

- Pi 包可以同时包含扩展、Skill、模板和主题；manifest 放在 `package.json` 的 `pi` 字段，也支持约定目录。本地包登记不复制目录。因此纯资源包即使没有扩展运行时也有分享与复用价值。[固定基线包文档](https://github.com/badlogic/pi-mono/blob/936aff00918de1187f085f123c2812d8f2d67745/packages/coding-agent/docs/packages.md)
- 包兼容包含版本固定、user/project/temporary scope、来源去重、manifest 与配置筛选、更新和持久化。固定源码还分别处理 npm、pnpm、Bun 和 wrapper；git 安装遇到 `package.json` 会调用配置的 npm 命令安装依赖。部分安装参数专门处理由 Pi 扩展宿主提供的 peer dependencies。因此完整包兼容的边界明显大于下载文件。[固定基线包管理实现](https://github.com/badlogic/pi-mono/blob/936aff00918de1187f085f123c2812d8f2d67745/packages/coding-agent/src/core/package-manager.ts)
- Pi 的资源加载器支持独立的显式 Skill、模板和主题路径；这些格式可以与包获取分别实现。上游内部仍通过 PackageManager 汇集部分资源来源，Pig 拆开交付时需保持本阶段声明支持路径的可观察语义。[固定基线资源加载器](https://github.com/badlogic/pi-mono/blob/936aff00918de1187f085f123c2812d8f2d67745/packages/coding-agent/src/core/resource-loader.ts)
- Pi 明确采用 Agent Skills standard，并记录从 Claude Code / Codex 技能目录加载的方法。Skill 可引用辅助脚本或外部工具，因此原生 Go 的资源加载能力不等于任何第三方 Skill 都无需其他运行环境。[固定基线 Skill 文档](https://github.com/badlogic/pi-mono/blob/936aff00918de1187f085f123c2812d8f2d67745/packages/coding-agent/docs/skills.md)
- Pig 已分别声明 ResourceLoader 和 PackageManager，资源与包方法仍有明确 Stub。settings 同时保留 packages 和独立 skills/prompts/themes 配置。这是可复用的边界，不需要先创建新的包格式或插件框架。[资源契约](../../codingagent/resources.go)、[包契约](../../codingagent/packages.go)、[设置契约](../../codingagent/settings.go)
- M6 的交付范围是布局、编辑器、按键、滚动、主题、对话框和会话选择；M7 首项要求先比较扩展运行时并形成 ADR。[M6](https://github.com/nankedr/pig/issues/8)、[M7](https://github.com/nankedr/pig/issues/9)
- 既有规定明确 M7 前不承诺 TypeScript/JavaScript 扩展源码兼容。受信包依赖和 lifecycle 属于宿主代码，需要项目信任、外部工具、Offline 和可见副作用的验收。[扩展规范](../specs/extensions.md)、[安全与网络规范](../specs/security-and-network.md)

以上上游源码已在本地 checkout 核对 HEAD 为 `936aff00918de1187f085f123c2812d8f2d67745`。

## 取舍与推论

| 能力 | 当前价值 | 建议 |
| --- | --- | --- |
| 本地资源及 Pi 资源格式 | 直接改善任务指令、复用流程和主题；为 TUI 提供资源基础 | M5 实现 |
| 本地资源包 manifest | 将多种资源统一登记，减少手动路径配置 | 有具体资源包需求时再补；当前直接路径足够 |
| npm/git 安装、更新与 lifecycle | 简化第三方资源分发和团队版本管理 | 延期完整兼容 |
| Pi TS/JS 扩展执行 | 提供可执行工具、命令、Provider 和 UI 扩展 | 按 M7 先决定运行时 |

推论：M6 对 npm/git 安装没有已识别的功能硬依赖；其现有 GitHub 阻塞关系是里程碑顺序，不能单独证明包管理是 TUI 的技术前置。M7 的候选运行时验证也可先使用本地受控扩展。

推论：当前安装能力能兑现纯资源分发收益，但不能让 Pi 可执行扩展直接工作。且部分包依赖行为与上游宿主有关，在 Pig 运行时未决定前全面冻结这些行为可能增加后续调整成本。

代价：延期后暂时失去一条命令安装、自动更新、声明包依赖及复现团队包集合的体验。用户可以自行获取文件，再通过显式资源路径加载。只需要静态资源的场景可用这一流程；依赖安装脚本生成资源或附带可执行程序的包不保证可用。

不将“当前没有必要”推成“生态没有价值”。本次未统计第三方包数量、用户使用率或人日成本，也没有已指定的必须复用资源包，不能据此估算生态收益规模。

## 重新评估条件与范围记录

出现以下任一条件时重新评估：明确要复用的真实包清单；团队需要声明式安装和版本管理；M7 已选择扩展运行时并需要可交付的分发方案。届时可选择只读 manifest 或有限资源获取作为小切片，无需默认一次接回所有兼容分支。

本决策调整 [#7](https://github.com/nankedr/pig/issues/7)、[#1](https://github.com/nankedr/pig/issues/1)、[ADR-0009](../adr/0009-defer-extension-runtime.md) 及扩展规范中“M5 完成包获取”的安排。Catalog 和发布说明须保留未完成包生态，不能按原 M5 全范围宣称完成；本记录保留评估事实与推论，正式范围以 ADR-0034 和更新后的规格为准。
