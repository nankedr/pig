# M5.8：发现扩展入口，执行仍未实现

运行 `go run ./examples/local-extensions`。示例写入一个导入就会报错的文件，通过公开 SDK 查询其来源，再确认执行查询返回 `ErrNotImplemented`。

`DefaultResourceLoader.Reload(ctx)` 更新本地资源及扩展入口快照；`GetExtensionDiscovery()` 返回独立的 `Entries` 和 `Diagnostics`。入口的 `Enabled` 只表示配置选择，不代表代码已加载、命令已注册或扩展已生效。`GetExtensions()` 和 `DiscoverAndLoadExtensions()` 仍返回可由 `errors.Is(err, codingagent.ErrNotImplemented)` 和 `errors.As` 识别的结构化错误。

## 本地发现规则

- 读取全局 agent 目录的 `extensions`、可信项目 `.pig/extensions`，以及设置中的本地 `extensions` 路径。项目未信任时不扫描目录、不读取项目设置和忽略规则。
- 自动目录先检查自身 `index.ts`，再检查 `index.js`；没有 index 时扫描一层 `.ts` / `.js` 文件及子目录 index。忽略隐藏项、`node_modules` 和 `.gitignore` / `.ignore` / `.fdignore` 命中的项，不无限递归。
- 先登记全部设置路径，再登记自动入口；同一原始路径保留首次来源与选择状态，随后按项目先于全局稳定排序并规范路径去重。配置 `!`、`+`、`-` 和基础 glob 影响 `Enabled`，完整 extglob 未实现。
- 显式路径优先，独立于项目信任；可指向普通文件或按上述规则发现目录。不读取扩展源文件内容，不解析 `package.json`。`.ts` / `.js` 只是固定基线的文件发现规则，不承诺语言兼容。
- `NoExtensions` / `--no-extensions` 跳过配置和自动发现，保留显式入口。SDK 来源保留 scope/source/baseDir；显式来源为 `temporary`、`top-level`，不伪装成已登记的包。

CLI `pig --no-extensions -e ./extension.js` 显示来源和 `not executed`，然后以 `codingagent.extension.discovery: not implemented` 非零退出。缺失本地路径和 `npm:` / `git:` 来源同样有稳定诊断且不会安装。未知长 flag 仍按固定顺序路由到 `extension.flag.<name>` Stub。

普通 Headless/RPC 会话可继续使用模板、Skill、Theme；发现扩展时诊断明确其未执行，配置禁用的入口明确标为 disabled。`--no-extensions` 不输出自动发现警告。会话 Reload 只刷新入口快照，不运行扩展 reload。扩展命令、工厂、信任钩子、override、资源回调和 `ExtendResources` 继续 Stub；M7 先研究并形成运行时 ADR。

## 对等证据与边界

```sh
node --experimental-strip-types parity/oracle/local-extensions.mjs .upstream/pi --check
go test -race ./codingagent -run '^TestLocalExtensions' -count=1
go test ./cmd/pig -run '^TestPigLocalExtensions' -count=1
```

固定 Pi 提取器通过公开 PackageManager 查询入口，再调用加载器触发无有效 factory 的失败；fixture 记录原始失败，Pig 验收路径、顺序、来源和选择状态，并按本票要求返回未实现错误。它不声称与 Pi 成功执行扩展对等。显式目录无 manifest 扫描、显式来源 top-level、执行 Stub 是 [ADR-0034](../adr/0034-defer-package-ecosystem.md) 的范围限制。

真实 CLI 子进程用模块导入标记、进程陷阱、本地 HTTP 探针和文件快照验证无代码执行、网络、扩展状态写入或扩展成功输出；SDK 会话验证删除和取消后的快照，POSIX FIFO 探针验证未信任项目与 manifest 不被打开。Catalog `contract:codingagent/local-extensions` 保持 partial；包生态由 #99 跟踪，六平台运行对等仍待验证。
