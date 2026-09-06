# Canonical 文件凭证与身份所有权

Issue #76 沿用 ADR-0008 的 Pig 状态隔离。`internal/statepath` 是 pig / pig-ai 共享的布局实现，`codingagent.ResolveAuthPath` 是公开入口。默认 `~/.pig/agent/auth.json`，`PIG_CODING_AGENT_DIR` 覆盖 agent 目录，显式路径优先；不查询 cwd auth 或 Pi 状态。Headless 的显式 `AuthPath` 优先于 `AgentDir`，注入 `Credentials` 则由调用者拥有存储实现。

`AuthStorage` 实现已有 `ai.CredentialStore`，构造时不做文件 I/O。新父目录 0700、写入文件 0600；Darwin/Linux 使用内核 flock 序列化整个文件的读改写，并保持锁内直接覆写，不使用 rename。内核在进程退出后释放锁，不依赖固定 Pi 的 proper-lockfile 目录租约；两个实现不共享活动状态。其他平台文件锁保持显式 Capability Stub，等待 M13。

`Read` 解析 API key，`Modify` 回调和 `ReadStoredCredential` 读取原始表达式，`List` 仅返回非 secret 元数据。修改沿用 Pi 的 nil/undefined 表示保持现有值；删除使用 Delete。额外字段经 JSON RawMessage 保留精度。空、缺失 key 或解析失败的已有 API key entry 显式失败，损坏文件也明确失败；不沿用 Pi 的 undefined 或 stale snapshot fallback。这落实项目既有身份所有权契约，避免失败后换用环境身份。OAuth 记录可无损保存、读取和枚举，但 Headless 消费保持 M11 Stub。

`!command` 使用宿主 shell、10 秒 timeout、丢弃 stderr，缓存进程内成功和失败结果。取消会终止进程组；调用者取消不污染后续命令缓存。Headless 在 trust 结论及设置检查后才接入 store；全局 auth 文件自身是可信可执行配置，trust 拒绝项目不禁止全局凭证命令。直接调用公开 Read 的宿主负责先完成自己的 trust 流程。

请求层显式 key 优先于 store，store 缺失才进入后续环境层。未交付的 Provider config、OAuth/ambient 登录、ModelRuntime 和 pig-ai 凭证命令仍为原有 Stub。已知请求 key 和认证 header 从 Provider 错误文本中脱敏，避免错误回显进入 Session；这刻意偏离 Pi 对 Provider 原始错误文本的透传。
