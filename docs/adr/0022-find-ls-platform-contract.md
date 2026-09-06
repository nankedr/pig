# find/ls 的本机平台契约

Issue #84 开放 find/ls 的 Tool、内建 definition 和显式 SDK/CLI 装配。默认四工具仍是 read/bash/edit/write；渲染与扩展宿主仍按 ADR-0020 留到 M7。

find 沿用固定 Pi 的 fd 参数、忽略规则和结果处理。依次探测 `GetAgentDir()/bin/fd`（Windows 为 fd.exe）、PATH 中的 fd、fdfind。PATH 候选使用 `--version` 探测能否启动；非零退出状态不等于不可启动，坏解释器等启动错误才继续 fallback。遵守 `docs/specs/cli-storage-and-platform.md`：缺失时返回可操作错误，普通调用不下载、不联网，也不创建工具目录。这是对 Pi `ensureTool("fd", true)` 静默下载分支的明确偏离；用户可以提前安装二进制。

Go 自定义 Operations 沿用已发布的 context 同步方法。取消后检查 context，fd 探测或搜索取消会终止整个进程组并等待直接子进程回收；自定义 Operations 必须响应 context，调用返回前不会抛下执行中的工作。`FindGlobOptions.Limit` 保持 Go int，不能表示的分数或溢出限制明确报错。本机 fd 仍接收原始 JSON number，ls 保留 number 比较语义（例如 limit=2.5 最多列三项）。

ls 用 Go 的 English collation 对小写名称稳定排序，对固定 fixture 中的大小写、重音字母和隐藏项验证。Pi 的运行环境 locale、ICU 与 Go Unicode 排序版本存在差异，其他 locale 和六个平台的运行对等仍留在 M13。当前默认本机 fd 执行复用 darwin/linux 进程组机制，其余平台返回 `FindOperations.platform` Capability Stub；自定义 Operations 与 ls 不依赖该进程机制。

Go Agent dispatcher 的工具错误携带空 details 对象，Pi definition 抛错没有 details；Oracle 只在错误投影中把二者归一为 null，错误文本和成功 details 保持完整比较。fd 本身的行分隔及 trim 语义也保持基线：换行文件名不能无损表示，名称两端的空白可能丢失；不声称这些名称可可靠回读。
