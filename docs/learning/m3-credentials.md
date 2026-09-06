# M3.6：从 canonical 凭证恢复请求

本切片用真实 Headless 子进程验证：一个进程通过 CredentialStore 保存 API key，新的 `pig` 进程直接从文件恢复身份。可运行示例仅使用临时文件、合成 key 和注入 transport：

```sh
go run ./examples/credentials
go test ./internal/parity -run '^TestCredentialStorageParity$'
go test ./codingagent -run '^TestCredential76'
go test ./cmd/pig -run '^TestPigCanonicalCredentials$'
```

公开入口是 `codingagent.NewAuthStorage(path...)`。省略路径得到 `~/.pig/agent/auth.json`，或 `PIG_CODING_AGENT_DIR/auth.json`；`ResolveAuthPath` 可单独查询布局。`CreateHeadlessSessionOptions.AuthPath` 选择显式文件，`Credentials` 可以注入其他已有 CredentialStore。CLI 显式 `--api-key` 最优先，不写入文件；没有显式 key 时 store 的已有 entry 拥有该 Provider 身份，只有 entry 缺失才查询 Provider 环境变量。

`Modify` 是锁内唯一写入口，回调收到原始表达式，返回 nil 保持现状；`Delete` 删除指定 Provider。`ReadStoredCredential` 不解析表达式，`List` 不执行命令、不返回 secret。`Read` 解析 key，支持直接值、`$VAR`、`${VAR}`、混合模板、`$$` / `$!` 转义。credential.env 的非空值先于宿主环境，环境值不缓存。命令由 `!` 开头，stdout trim 后作为 key，stderr 丢弃，最多 10 秒，成功/失败都按命令文本缓存至进程结束。

文件锁覆盖整个读改写，因此两个进程修改不同 Provider 不会覆盖彼此。新目录为 0700，文件写入为 0600，使用直接覆写保持固定 Pi 的崩溃语义；不能把它描述为事务式原子替换。额外 credential 字段通过 RawMessage 保留，包括大整数。损坏文件和无法解析的 entry 明确报错。

Headless 先得出 Project Trust 结论，再读取会执行命令的凭证。全局 auth 文件是用户提供的可信可执行配置；项目被拒绝信任时仍可使用全局 key。原始读取和枚举可用于不执行引用的状态显示。不会从项目目录或 Pi 状态自动导入 key。

合成 secret 检查覆盖请求 body、进程 stdout/stderr、Provider 错误回显、持久化 Session 和显式 Telemetry recorder。当前适配器不产生认证 Telemetry span，默认 telemetry 仍为 NOOP。记录内容只包含非 secret 元数据，示例不打印 key。

OAuth/ambient 登录、ModelRuntime、`pig auth` 和 `pig-ai login/list` 的运行能力仍按 M11 明确失败；本切片不把这些入口改成伪成功。pig-ai 的路径解析与 pig 共用布局。文件执行平台为 Darwin/Linux，其余平台锁等待 M13。对等目录 `contract:config/auth-json` 记录已交付范围和偏离，详见 ADR-0021。
