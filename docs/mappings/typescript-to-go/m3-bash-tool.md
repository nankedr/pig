# M3.10 TypeScript → Go：bash

Code Baseline：936aff00918de1187f085f123c2812d8f2d67745，Issue #80。

| Pi 路径 / 入口 | Pig 路径 / 入口 |
| --- | --- |
| core/tools/bash.ts：createBashTool / createBashToolDefinition | codingagent/bash_tool.go：CreateBashTool / CreateBashToolDefinition |
| bash.ts：createLocalBashOperations | codingagent/bash_local.go：CreateLocalBashOperations |
| bash.ts：resolveSpawnContext / ExtensionContext session metadata | bash_tool.go 与 session.go：执行上下文携带 Session，逐次读取当前状态 |
| core/tools/output-accumulator.ts | codingagent/bash_output.go：有界 UTF-8 尾部与按需临时文件 |
| core/tools/truncate.ts | codingagent.TruncateTail → agent.TruncateTail |
| utils/shell.ts：getShellConfig / getShellEnv | codingagent/bash_local.go、bash_unix.go、bash_other.go |
| utils/shell.ts：killProcessTree | bash_unix.go：独立进程组与 SIGKILL；其他平台留 M13 |
| utils/child-process.ts：waitForChildProcess | bash_local.go：进程等待与输出读取分离，退出后按数据重置空闲计时 |
| tools.test.ts、regressions/5208、regressions/5303 | codingagent/issue80_bash_test.go、issue80_process_unix_test.go |
| Headless tools / shutdown | codingagent/headless.go、cmd/pig/signals_unix.go；issue80_process_test.go、issue80_signal_unix_test.go |
| Pi Oracle | parity/oracle/bash-tool.mjs、fixtures/bash-tool.json |
| Go SDK 示例 | examples/bash-read/main.go |

AbortSignal 对应 context，Promise/output event 对应进程等待 channel 和数据管道；累计 update 的节流与最终 flush 由 mutex/timer 保持顺序，Agent dispatcher 等待 listener barrier。取消时已完成的错误结果保留到 transcript，避免 partial 输出与文件路径丢失。Pi 的 PI_* 和 pi-bash 品牌按 ADR-0008 映射为 PIG_* 和 pig-bash。

公开 API 快照见 codingagent/testdata/issue80_surface_golden.txt；兼容面与尚未支持的范围见 Parity Catalog。
