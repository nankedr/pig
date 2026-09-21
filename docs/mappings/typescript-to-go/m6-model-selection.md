# M6.9 TypeScript → Go：模型与 thinking

| Pi 基线 | Pig 公开边界 |
| --- | --- |
| ModelSelectorComponent | NewModelSelectorComponent；本地 snapshot + current + scope + 回调 |
| ThinkingSelectorComponent | NewThinkingSelectorComponent；当前等级与能力列表 |
| ScopedModelsSelectorComponent / ModelsConfig / ModelsCallbacks | 同名 Go 类型；OnChange / OnPersist 用 error 表示失败 |
| InteractiveMode 的 model/settings/scoped-models 路由 | InteractiveMode.Run；TextUI.SetDialog 保留原编辑器 |
| session.setModel / setThinkingLevel / cycleModel | 既有 AgentSession 方法与 ADR-0025 事务 |
| scope 修改 + settings 保存 | AgentSession.SetEnabledModels；未保存时只变更 Session 范围 |
| fuzzyFilter / Input | tui.FuzzyFilter 复用既有排名；tui.Input 复用 Editor 编辑能力 |

Go 构造器不启动网络刷新，也不接受能让 UI 提前写 settings 的依赖。选择成功后的事务由 AgentSession 负责。此处主动保留既有 ADR-0025 的忙碌拒绝和失败原子性，避免 Pi 选择器先存默认模型、回调再失败的半更新。

API snapshot：`codingagent/testdata/issue117_surface_golden.txt`。Catalog：`contract:codingagent/model-selection`，证据与未覆盖分支见学习说明。
