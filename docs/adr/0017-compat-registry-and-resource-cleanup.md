# 兼容入口统一注册表与 Session Resource 清理快照

Issue #65 要求 compat Stream/Complete 与全部 deprecated aliases 复用已注册 Provider。固定 Pi 的 `legacy-api-aliases.ts` 将别名绑定到适配器，覆盖 compat registry 不会改变别名行为。Pig 按 Issue 要求让 16 个别名按固定 API ID 校验 Model，然后读取同一注册表；直接 `OpenAICompletionsAPI()` 等 ProviderStreams 仍绑定真实适配器，避免全局覆盖改变独立 Provider 或产生递归。这是明确的语义偏离，不修改 M1 的事件、Outcome 和 options 契约。

注册表在互斥锁下线性化写入与快照读取。覆盖保留原位置，只保留最新 source 所有权；按 source 注销不会恢复被覆盖项；builtin 恢复只追加缺失项；reset 原子替换为最初的十项。已取得的 Provider/Stream 继续使用取得时的回调。

Session Resource cleanup 每次按注册顺序捕获快照，在锁外同步执行所有回调。回调中的注销与新注册从下次 cleanup 生效，避免死锁和执行期间无限扩展集合。Pi 的 live Set 会跳过尚未访问的已删除回调，并访问新加入回调；独立 deviation fixture 保存这一差异。每次 Go 注册拥有独立注销句柄；Go 函数不可比较，因此不模拟 Pi Set 对同一函数对象的去重。

`SessionResourceCleanup` 保持已发布的 `func(...string)` 签名。未传 session ID 表示全部资源；回调负责筛选、释放资源与重复调用的幂等性。cleanup 不注销回调，不自动关闭所有 Provider，也不清空 Faux 队列或其内部 cache。失败以 panic 跨越此 void 回调边界：逐回调 recover，保留 error 原因，用 `errors.Join` 汇总后返回；其他回调始终被调用。多次并发 cleanup 的回调可重叠，资源所有者负责自己的并发同步。

`compat-session-resources.json` 验证共同语义；`compat-session-resources-deviation.json` 分别断言 Pi 与 Pig 的别名及回调修改行为，不用 normalization 掩盖差异。M10/M11/M12 的真实协议、ambient auth 和图片 Stub 继续保持原阶段边界。
