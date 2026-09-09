# RPC 会话替换与 v3 数据边界

Issue #96 接通固定 Pi `936aff00918de1187f085f123c2812d8f2d67745` 已有的 `new_session`、`switch_session`、`fork`、`clone`、`get_fork_messages`、`get_entries`、`get_tree`、`set_session_name`、`compact` 和 `set_auto_compaction`，保留公开 RPCClient 的既有签名。返回 bool 表示 cancelled；Fork 同时返回选中的用户文本。Clone 使用当前 leaf 和 position=at；Fork 使用用户节点之前的位置。GetEntries 的 since 为排他游标，不存在时返回错误。Session name 去掉首尾空白后不得为空。

正式 v3 的 marshalSessionEntry/decodeSessionEntry 同时承担 RPC entry 编解码，保留 Raw 和已知消息类型。树只增加私有 wire 投影，不改变已发布 SessionEntry/SessionTreeNode 的字段或默认 JSON 行为。查询返回全部树条目；压缩保留完整历史，改变后续模型上下文。RPC 不输出额外 Session header，文件 header、身份和 parentSession 由 SessionManager 负责。

替换期间关闭旧 Session 的新操作入口，取消并等待 Prompt、摘要与 Bash 收尾，再读取 fork/switch 的来源，避免丢失最后的持久化记录。查询和 Abort 仍可运行。创建目标运行时成功后才销毁旧 Session；创建失败或提交前 Context 取消保留原 Session 可用。这调整了 #73 的 create/invalidate 顺序，是相对固定 Pi 先 teardown 再 create 的显式 Go 偏离。提交后取消不撤销已成功的替换。通用 Runtime 的外部 rebind callback 若失败，仍沿用旧契约：目标失效并返回错误；RPC 内部 rebind 仅订阅刚创建的 Session，不调用扩展。

RPC 转发通过绑定锁同时发布 Session 与 listener ID。订阅事件和 Bash 直接输出回调都检查所属 Session；旧 Session 已排队的事件保持在切换响应之前，迟到事件不进入新绑定。响应仍携带原命令 id，不用事件代替命令的完成响应。手动压缩响应等待提交，自动压缩事件和后续生成属于原 Prompt；本地请求 Context 只取消等待，远端取消使用 Abort。并发修改沿用 Session admission。

创建目标时重新装配 cwd、Project Trust、设置、Tools、上下文文件和模型服务，继续使用固定 CLI 输入；显式 --model/--thinking 保持原有优先级，未显式指定时从目标历史和设置恢复。Provider endpoint 和显式凭证继续由 Headless factory 绑定。旧会话资源清理后，写入目标和 Session 身份切到新实例。

固定 Pi **没有** `navigate_tree`、`abort_compaction` wire 或 RPCClient.NavigateTree。rpc-mode.ts 的 navigateTree 只存在于扩展 command context，可触发 #92 所实现的摘要导航；Pig 扩展运行时尚未交付，因此此 RPC 切片不宣称支持生成分支摘要。公开 SDK NavigateTree 及已有 branch_summary 的 v3 读取不受影响；不为满足 Issue 的宽泛描述发明 wire 命令。图片、导出、资源/扩展/UI、自定义摘要和未交付的 Provider Adapter 仍在后继切片。

证据包含固定 Pi 源码 RPC dispatch、公开 RpcClient 的持久化生命周期 Oracle、真实 pig/RPCClient 的压缩与重开，以及 race 构建子进程中的查询、取消和替换。此次只新增源码 Oracle 证据，不宣称 Pi dist 重建或 M4 整体冻结。
