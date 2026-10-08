# Responses 会话的协议选择与恢复

#133 为同一个 DeepSeek 模型保留 Chat Completions 和 Responses 两条可运行路径。只保存 provider/modelId 无法区分协议，因此 production v3 的 model_change 增加可选 api；正式 reader/writer、fork 和树导航保留此字段。已有 Assistant 的 Responses API 同样能恢复选择。旧记录缺省时沿用固定 Catalog API，不修改 Session 版本或原始模型 Snapshot。

这是 Pig 对固定 Pi 的显式偏离：固定 Pi 从模型目录确定协议，Pig 在本地目录模型上允许选择 API，增加的可选元数据用于恢复该选择。不把此恢复策略当作 Pi Oracle 行为。

API 优先级为创建时显式 API、可恢复 Session、匹配 Provider 的 settings.defaultAPI、Catalog 默认。显式模型仍使用匹配 Provider 的 defaultAPI；显式 API 可覆盖设置。模型切换通过已有配置事务写入 v3 和模型偏好；Responses 写入 defaultAPI，已有该字段时切换到其他协议也更新它。未使用 Responses 的旧会话不增加协议偏好。

Headless 创建的 Runtime 保留所选协议及显式 endpoint 覆盖，TUI/RPC/SDK 的模型切换在发布状态和写入文件前应用这些覆盖，生成和摘要直接使用该模型。RPC set_model 返回实际生效模型，get_available_models 的 API 与该 Runtime 选择一致。原始 ModelRuntime Snapshot 保持不可变，独立 Session 不互相修改目录。

认证、Project Trust、settings 的锁与回滚、整轮 retry、摘要 retry、取消与终态仍使用既有实现。本地完整历史是唯一恢复依据；响应 ID 仅用于观察，不用 previous_response_id、conversation、background 或服务端缓存。CredentialStore 与环境/显式凭证优先级不变，凭证与 endpoint 不写入新增会话元数据。

Responses 并不解锁其他 Provider、远端模型同步或 models.json 自定义运行时；这些保持 #129 的明确 Stub。JSONL RPC 不增加固定 Pi 没有的 navigate_tree 命令；树导航和分支摘要从公开 SDK/TUI 验证。
