# M3.5 Project Trust 与项目设置

Headless 启动先读取 Pig 全局设置，再判定 Project Trust，最后读取被允许的项目设置并选择 Session 目录。`.pig/settings.json` 中的 `sessionDir`、默认模型、资源路径、代理等字段在信任前一律不读取。

## 判定与保存

优先级是显式 `--approve` / `--no-approve`、没有敏感资源时直接 trusted、最近祖先的保存决定、全局 `defaultProjectTrust`。全局策略支持 `ask`、`always`、`never`；Headless 没有 UI，`ask` 返回 untrusted。M7 前不执行 pre-trust extension，M6 前不提供 TUI 信任选择器。

仅当前 cwd 下的 `.pig/settings.json`、`extensions`、`skills`、`prompts`、`themes`、`SYSTEM.md`、`APPEND_SYSTEM.md`，以及 cwd 和祖先的 `.agents/skills` 触发判定。空 `.pig` 不触发；用户 home 的 `.agents/skills` 不算项目资源。探测只检查存在性，不读取资源内容。

`CreateHeadlessSessionOptions.ProjectTrustOverride` 对应本次决定，不持久化。公开 `ProjectTrustStore.Set` 可保存允许或拒绝；`SetMany` 在同一个锁内更新父目录并删除子目录决定，nil 代表删除。`GetEntry` 返回最近的保存路径和决定。路径先绝对化，存在时解析 symlink；最近祖先优先，不用字符串前缀冒充父子关系。

store 使用 Pig agent 目录的 `trust.json`；不继承 `.pi` 或 `PI_*`。写入在目录锁内重读和直接覆写，父目录 `0700`，文件 `0600`。损坏数据和锁竞争失败返回错误，不能被当成允许。Go 对不存在的 store 只读查询不创建目录，保留显式内存模式的无状态约定。

## 项目 settings

信任后将项目设置递归合并到全局设置，对象深合并、数组替换。CLI 和环境的显式 Session 目录仍优先于合并后的 `sessionDir`。拒绝时仅使用可信配置，项目的 `defaultProjectTrust: "always"` 不能自我授权。

`NewSettingsManager` 默认保持 untrusted，调用者可以通过 `SettingsManagerCreateOptions.ProjectTrusted` 或 `SetProjectTrusted(true)` 明确允许项目读取。`SetProjectTrusted(false)` 清除项目视图，`Reload` 保持当前信任状态；加载失败通过 `DrainErrors` 的 `project settings:` 诊断报告。全局 setter 不会把项目覆盖写入全局文件。项目写入仍是后续资源切片的 Capability Stub。

Headless 未注入 SettingsManager 时自动完成上述判定。显式注入的 SettingsManager 由调用者管理信任状态和覆盖；Headless 的显式 ProjectTrustOverride 仍可改变它的项目 gate。

## Context File 例外

**拒绝 Project Trust 仍允许仓库指令进入模型 prompt。** 它不是“禁止项目影响模型”的开关，也不是 Tool 审批或 workspace sandbox。

每个目录依次选择 `AGENTS.override.md`、`AGENTS.md`、`AGENTS.MD`、`CLAUDE.md`、`CLAUDE.MD` 中第一个可读普通文件。加载顺序是 agent 全局目录、根目录到 cwd 的祖先层；嵌套 Git worktree 的同名主仓库指令按固定 Pi 去重。`--no-context-files` 或 SDK 的 `NoContextFiles` 关闭整个搜索。显式自定义 system prompt 仍可包含这些 Context Files。

本切片只实现此例外所需路径。`SYSTEM.md`、`APPEND_SYSTEM.md`、完整资源装载、包安装及扩展执行仍按 M5/M7 边界处理；信任不会提前启用这些运行时。

## 运行与证据

```sh
go run ./examples/project-trust
go test ./internal/parity -run '^TestProjectTrustParity$'
go test ./codingagent -run 'ProjectTrust|ProjectSettings|ProjectContext'
go test ./cmd/pig -run 'TestPigProjectTrust|TestPigUntrustedProject'
```

示例只调用本机 HTTP fixture，拒绝时使用全局模型，允许时使用项目模型，两次请求都收到 Context File 指令。

Oracle 固定在 `936aff00918de1187f085f123c2812d8f2d67745`。`project-trust.mjs` 验证 SDK 的存在性与持久决定；`project-trust-startup.mjs` 运行真实 Pi CLI，验证 ask/approve/deny、项目模型覆盖和 Context File 例外。共享 CLI case 显式设置 Session 目录，避免把 Pi 已知的 pre-trust `sessionDir` 缺口误当作兼容目标；Pig 的 FIFO 与恶意 settings 进程测试单独锁定 ADR-0010 安全修正。FIFO settings 若被打开会阻塞；敏感资源同样放置 FIFO，Context File 则通过 Provider 收到的真实 prompt 验证。跨进程测试验证锁与更新保留。
