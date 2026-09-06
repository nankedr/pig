# M3.9：修改文件并核对差异

edit 的执行链是模型 ToolCall → 参数准备 → schema 校验 → 与 write 共用的文件队列 → 读取原文件 → 匹配和整体校验 → 一次写入 → ToolResult → read → 模型继续生成。可运行的离线 SDK 示例：

```sh
go run ./examples/edit-read
```

Headless 显式选择工具，默认仍为 read：

```sh
go run ./cmd/pig --provider deepseek --model deepseek-v4-flash --tools read,edit -p '把 hello.txt 的 world 替换为 Pig，然后读回确认'
go run ./cmd/pig --provider deepseek --model deepseek-v4-flash --tools read,edit,write --mode json -p '创建文件，编辑两处内容，再读回'
```

SDK 将 `CreateEditTool(cwd)` 返回值放进 `CreateAgentSessionOptions.AgentTools`，并在 `Tools` 选择 `edit`。参数形如：

```json
{"path":"hello.txt","edits":[{"oldText":"world","newText":"Pig"}]}
```

每项 oldText 都针对同一份原文件匹配，按位置逆序应用，不能利用前一项替换产生的文字匹配下一项。空 edits、空 oldText、缺失、规范化后的多次出现、重叠和完全无变化都会返回错误，所有匹配通过前不会写入。newText 可以为空，用于删除。直接调用 `CreateEditToolDefinition` 时先调用 `PrepareArguments`，再 `Execute`；旧式顶层 oldText/newText 会追加进 edits，字符串形式的 edits 会先解析为数组。公开 schema 仍只声明 path/edits。

匹配先尝试精确文本，再按固定 Oracle 的 Unicode 16 做 NFKC、行尾空白、智能引号、Unicode 连字符及空格规范化。即使精确匹配成功，也在规范化空间检查唯一性。任一项需要模糊匹配时，整批在规范化空间定位，再按实际触及的行覆盖原文；未触及行保留原始字节，不会误选相邻的相同行。

编辑前去除 BOM 参与匹配，写入时恢复 BOM；CRLF 和单独 CR 先转 LF，结果按原文件最先出现的 LF/CRLF 恢复整份文件的换行。diff/patch 比较去 BOM 的 LF 原文与结果。`details.diff` 是带行号、四行上下文的展示文本，`details.patch` 是带 EOF 换行标记的标准 unified patch，`details.firstChangedLine` 指向新文件中的第一个变更位置。它们保存在 Session ToolResult 和 CLI JSON 事件中；模型收到成功文本和后续 read 内容，Provider 对 ToolResult details 的投影沿用已有契约。

路径与权限继承 write/read 的宿主契约：相对/绝对路径、父目录、home、file URL 与已有 symlink，不增设 workspace 边界或逐 Tool 审批。edit 与 write 共享进程内按文件串行的队列，不同文件可并行。排队取消保持 FIFO 位置；在途 access/read/write 必须结束后才释放队列。匹配失败不会部分应用多个 edits；宿主 I/O 错误或取消不承诺撤销已经写入的字节。它不是跨进程锁或原子文件事务。

验证入口：

```sh
node --experimental-strip-types parity/oracle/edit-tool.mjs <locked-pi-checkout> --check
go test -race ./codingagent -run '^TestEditTool' -count=1
go test ./cmd/pig -run '^TestPigEditReadContinuation$' -count=1
```

Oracle 运行固定 Pi 的 edit definition/read，错误投影沿用 Pi Agent 的空 details；Pig 在公开 AgentSession 重放输入，核对内容、ToolResult、diff、patch、首个变更行和后续模型上下文。用例覆盖 Unicode/BOM/UTF-8、换行、删除、缺失与歧义、重叠、模糊保留、重复行和长 diff。队列测试另以文件系统完成屏障验证混合 write/edit 和 Session Abort。CLI 测试运行真实进程与本地 Provider fixture，包括未信任项目设置的隔离。

交互 diff 预览/渲染、扩展宿主及默认完整工具集合仍由后续切片处理。能力与证据见 `contract:codingagent/edit-tool`；参数准备入口见 ADR-0020。
