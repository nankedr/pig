# Interactive Bash 映射

| Pi 固定基线 | Pig | 证据 |
| --- | --- | --- |
| InteractiveMode 输入 `!` / `!!`、handleBashCommand | InteractiveMode.Run → AgentSession.ExecuteBash | bash-cli.json、TestPigBashParity122 |
| BashExecutionComponent | NewBashExecutionComponent / AppendOutput / SetComplete / SetExpanded | TestBashComponent122、issue122_surface_golden.txt |
| onEscape / shutdown | TextInput.Bash 执行身份、取消 Context、Session 收尾 | TestInteractiveBashSDK122、TestPigBashCancelPriorityAndResume122 |
| pendingBashComponents / Session pending messages | Transcript 活动组件与既有 Session 暂存记录 | bash-cli.json 中 deferred_context |
| bashExecution 历史恢复 | Transcript 解析 RawAgentMessage，复用 BashExecutionComponent | CLI 恢复及公开 Session reader |

沿用 ADR-0018 的严格队列接纳和 ADR-0032 的替换前收尾。SDK 已完成组件忽略迟到 AppendOutput；相对 Pi 可继续改写组件的行为，这是明确的 Go 生命周期约束。继承的 Container 操作只保留 API 形状，扩展子组件自定义渲染不在本次对等范围。
