# M5.1 Context File：TypeScript → Go

固定对等基线：`936aff00918de1187f085f123c2812d8f2d67745`。Pi 源码路径以下均相对 `packages/coding-agent/src/`。

| Pi | Pig | 阅读重点 |
| --- | --- | --- |
| `core/resource-loader.ts: loadProjectContextFiles` | `codingagent/context_files.go: LoadProjectContextFiles` | 全局优先、候选优先级、祖先顺序、symlink 与嵌套 worktree |
| `core/resource-loader.ts: DefaultResourceLoader` | `codingagent/resources.go: NewDefaultResourceLoader / Reload / GetAgentsFiles` | 本地文件快照与独立诊断，无 package manager 依赖 |
| `core/sdk.ts: createAgentSession` | `codingagent/sdk.go: CreateAgentSession` | 默认加载器和显式注入实例进入实际模型输入 |
| `core/agent-session-services.ts` | `codingagent/session_services.go` | 服务装配共享已加载实例 |
| `core/system-prompt.ts: buildSystemPrompt` | `codingagent/prompts.go`、`codingagent/headless.go` | 保留来源的 project_context 块 |
| `cli/args.ts: noContextFiles` | `codingagent/misc.go`、`codingagent/sdk.go` | CLI 与 SDK 禁用选项 |

证据入口：[基线提取器](../../../parity/oracle/context-files.mjs)、[SDK 验收](../../../codingagent/issue100_context_test.go)、[CLI 子进程验收](../../../cmd/pig/issue100_context_test.go)、[可运行示例](../../../examples/context-files/main.go)。

信任处理保留 ADR-0010 的 pre-trust 安全偏离。资源回调仍是 opaque carrier；其余资源与包生态不因 Context File 链路开放而宣称完成。使用说明见[学习材料](../../learning/m5-context-files.md)。
