# M3.4 全局设置驱动 Headless 会话

Pig 从 `~/.pig/agent/settings.json` 读取全局设置；`PIG_CODING_AGENT_DIR` 可覆盖 agent 目录。示例配置：

```json
{
  "defaultProvider": "deepseek",
  "defaultModel": "deepseek-v4-flash",
  "defaultThinkingLevel": "high",
  "sessionDir": "~/pig-sessions"
}
```

配置好 `DEEPSEEK_API_KEY` 后运行 `pig -p "hello"` 即可使用这些默认值。当前内建运行路径支持 DeepSeek V4 Flash 和 Pro；其他 Provider 仍受现有能力边界限制。

## 启动优先级

模型优先使用显式 `--model`（可配 `--provider`，或使用 `provider/model`），然后是旧 Session 的模型、全局默认值、可用 Provider 的默认模型。只有 `--provider` 而没有 `--model` 时，不覆盖已保存的模型对，这与固定 Pi CLI 一致。当前模型选择支持精确 ID。

thinking 优先级是 `--thinking`、模型 ID 的 `:thinking` 后缀、旧 Session 的 thinking、全局默认值、`medium`；最后根据模型能力限制取值。因此 DeepSeek 的 `low` / `medium` 会提升到支持的 `high`。

Session 存储位置依次使用 `--session-dir`、`PIG_CODING_AGENT_SESSION_DIR`、全局 `sessionDir`、Pig 默认布局。`--session <path>` 重开指定文件；`--no-session` 使用内存 Session。退出后修改全局配置，新进程会重新读取；旧 Session 的模型和 thinking 优先于新默认值。

## SettingsManager 的数据和保存

`NewSettingsManager` 读文件，`NewSettingsManagerFromStorage` 接受存储边界，`NewInMemorySettingsManager` 支持无文件的测试和嵌入。构造读取不存在的配置不会创建目录。返回的 Settings 和集合均有独立所有权。

ApplyOverrides 递归合并对象，以新数组替换旧数组；nil 表示未提供，空切片表示显式清空。覆盖只作用于当前内存视图；setter 保存或 Reload 后重新构建视图，不保留覆盖。原始稀疏字段、null 和未知字段在序列化和无关字段保存中保留。

迁移覆盖 `queueMode → steeringMode`、`websockets → transport`、旧 skills 对象与 `retry.maxDelayMs → retry.provider.maxRetryDelayMs`。这只迁移配置数据，不扫描或迁移 Pi 的目录。

setter 在目录锁内重读配置，只写回修改字段，并保留外部进程修改的无关字段和嵌套成员。Go setter 同步完成写入；Flush 是等待并发 setter 完成的边界。锁采用固定 Pi 的 10 次尝试、20ms 重试间隔和 10 秒 stale 判定。读写失败通过 DrainErrors 收集，错误以 `global settings:` 标明作用域并保留原因；Flush 不代替 DrainErrors。损坏配置不会被 setter 覆盖，成功 Reload 修复后的文件才能恢复保存。

## 当前边界

本切片完全不探测项目 settings，包括 sessionDir、httpProxy、默认 trust 等字段。`ProjectTrusted: true` 和项目写入返回 Capability Stub；默认只读全局设置。全局 `defaultProjectTrust: "always"` 也不会打开项目读取，遵循 ADR-0010。

未来的 UI、compaction、retry、shell、analytics 等配置只保留数据语义，不启动相关行为。需要资源或代理运行时的非空 packages、extensions、skills、prompts、themes、httpProxy 配置会明确报 Capability Stub。保存 analytics 开关不会启动外联。

运行离线示例：

```sh
go run ./examples/global-settings
```

它在临时目录保存设置，重新创建 SettingsManager，以本地 HTTP fixture 执行 Headless 请求并确认 Session 落盘。Oracle 证据见 `parity/oracle/global-settings.mjs` 和 `settings-startup.mjs`；后者通过固定 Pi 的真实 CLI 进程验证启动和重启优先级。Go 在公开 SettingsManager、Headless SDK 和真实 pig 进程边界复核这些行为。
