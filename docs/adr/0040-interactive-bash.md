# 交互 Bash 的执行身份与历史投影

Issue #122 在 Pi `936aff00918de1187f085f123c2812d8f2d67745` 的 `!` / `!!` 输入边界复用 AgentSession.ExecuteBash，不另建 shell 执行器、审批或 Sandbox。AgentSession 仍是记录、截断与上下文转换的唯一所有者；交互层只拥有活动展示组件。

Bash 与 Agent 生成可并行，交互层禁止第二条 Bash，SDK 原有并行 ExecuteBash 契约不变。输入携带独立 Bash 执行身份，避免上一条执行期间排队的 Escape 在新命令中生效。Escape 在活动生成时优先处理 #116 的生成/重试取消，随后才取消独立 Bash。会话替换和退出取消并等待 Bash，保留最终部分输出后再销毁 Session。

BashExecutionComponent.SetComplete 固定结果并复制参数，之后 AppendOutput 无作用。这是为 #122 的迟到回调要求采用的 Go 偏离；固定 Pi 组件本身不阻止完成后的 appendOutput。交互文本组件不拥有 Pi 的定时 spinner，而由既有 TextUI 刷新与文字状态表达运行中。精确边框、动画和 Container 自定义子组件渲染保持 partial。

持久化 reader 返回 RawAgentMessage，Transcript 显式解码 bashExecution；渲染使用 SafeTerminalText，并保留 excludeFromContext 的淡色提示。展示不会将原始消息改写成 Provider 输入。除这两条执行身份/组件收尾约束外，生成期 Bash 暂存、follow-up 队列和上下文转换继续沿用已有 Session 与 ADR-0018 契约。
