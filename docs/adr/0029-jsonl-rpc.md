# JSONL RPC 的进程与等待边界

Issue #94 在固定 Pi `936aff00918de1187f085f123c2812d8f2d67745` 的 JSONL RPC 上实现基础对话。CLI 与 text/json 共用 Session、认证、设置、Project Trust 和上下文文件装配；RPC 不先把 stdin 读作 prompt，不输出 Session header，不使用 Remote Session Protocol 的 CBOR decoder。`RunCLI` 仍是无副作用的模式路由，`Main` 消费其模式标记并启动运行时。

运行时仅按 LF 分帧，剥去行末单个 CR，支持 UTF-8 分片、任意长度行及 EOF 前没有尾换行。writer 复用 JSON 事件投影，在同一锁内写入一个完整 JSON value 和 LF。JSON 字符串转义形式允许不同，但 U+2028/U+2029 不构成帧边界。JSON.parse 后的宽松分派是兼容边界：未知字段不拒绝；未知命令保留任意 JSON id/type；缺失 type 不补空字符串；缺失、null、非字符串 message 保留已观察到的错误形态。语法错误正文保留解析器自身描述，Parity Case 归一为 syntax。Pi 对顶层 null 的未处理异常在 Pig 中映射为进程错误退出，避免 panic。

每条命令独立处理。响应关联依赖 id；发送顺序不保证完成顺序。prompt 只在预检和 Session 接纳后响应一次，不等待生成结束；Provider 失败与取消通过原有事件和消息记录表示。Session 的私有预检回调仅供 RPC 接纳路径使用，公开 PromptOptions.PreflightResult/Source 及扩展拦截仍未交付。streamingBehavior 沿用 Pi 的真值分支：followUp 进入后续队列，其他真值进入 steering 队列。图片和未交付命令明确失败。查询、abort 和事件可以在运行期间并发。

Go RPCClient 的 CLIPath 指向 pig 可执行文件，默认在 PATH 查找 pig。构造器不启动进程；Start 使用 get_state 握手确认可用，代替 Pi 的固定 100ms 猜测。Start 的 Context 控制启动等待，成功之后每个请求独立接收 Context。请求取消只释放本地 waiter，使用递增且不重用的 id，丢弃迟到响应；需要取消模型运行时调用 Abort。没有隐式请求超时。WaitForIdle 先订阅再查询当前状态，已 idle 立即返回，避免错过 settled；CollectEvents 收集订阅后的事件直到下一次 settled，PromptAndWait 在发送之前订阅。收集时即使进程紧接着退出，也允许读取已到达的完整终态事件。

stdout reader 与订阅回调分离，每个订阅保持无界 FIFO 和独立解码值。回调可调用查询、退订或 Stop；回调自身应及时返回。Stop/退出取消订阅并唤醒请求与收集等待，Start 失败也清理启动阶段订阅。Stop 尝试 SIGTERM，至多等 1s 后强杀并回收；调用方更短的 Context 提前触发强杀。macOS/Linux 进程组清理辅助子进程；Windows 采用进程句柄终止，不承诺完整 Job Object 后代树清理。Unix 运行时把继承的 stdin/stdout 包装成可取消的 pollable 文件，防止 stdout 失败后 reader 卡住导致进程不退出。EOF 后给 stdout 最多 1s 排空时间，并在命令退出后传播最终写错误，避免背压无限阻塞或误报退出成功。

RPCCommand.Data/RPCResponse.ID 仍保持已发布的载体签名：前者不作为扁平化命令的公共通用发送 API，后者承载 RPCClient 自身生成的字符串 id。原始 CLI 的任意 JSON id 由宽松对象分派保留，不经这两个载体收窄。JSON parser 文案、就绪握手、Context 和 idle 探测是本切片明确的 Go 映射；认证和 trust 沿用此前 ADR。

合并自动压缩能力后，RPCClient 同时解码 compaction、summarization retry 与 Bash update 会话事件；真实 pig 子进程覆盖 overflow → 摘要重试 → 恢复生成，不把已接收的这些事件静默丢弃。对应 RPC 控制命令仍保持上述未交付边界。

输入前历史压缩属于 prompt 预检；成功响应在该检查及取消检查后发送。若在这段预检期间 Abort，prompt 返回失败，不把尚未入历史的新消息报告为成功。
