# M6.9：模型与 thinking 选择

模型选择沿用 AgentSession 的配置事务，UI 只负责收集选择。模型目录来自已有 ModelRuntime 的本地可用快照；不提前实现 M10 动态目录或 M11 完整认证。

| 入口 | 行为 |
| --- | --- |
| `/model`、Ctrl+L | 打开模型选择器，展示 ID、provider、名称和当前模型 |
| `/model provider/id` | 唯一精确匹配直接切换，否则以参数作为搜索词打开选择器 |
| 搜索、↑↓、Enter、Esc | 按 provider／ID／名称模糊搜索、循环导航、选择、取消；每次非空搜索重置高亮至首项 |
| Tab | 已有范围时，在 all 与 scoped 之间切换；回到范围时定位当前模型 |
| `/settings` → Thinking level | 只列出当前模型支持的 thinking 等级；选择或取消 |
| Shift+Tab | 按当前模型能力轮换 thinking |
| Ctrl+P、Ctrl+Shift+P | 按当前模型范围向前／向后轮换 |
| `/scoped-models` | 编辑模型范围，修改立即作用于当前 Session |
| 范围面板 Enter／Ctrl+A／Ctrl+X | 切换单项／启用全部／清空；搜索时批量操作只影响匹配项 |
| 范围面板 Ctrl+P、Alt+↑↓ | 切换当前 provider 的所有模型；调整已启用项顺序 |
| 范围面板 Ctrl+S、Esc | 保存范围到 settings；关闭面板。关闭不会撤销已成功应用的 Session 范围，符合 Pi 基线 |

nil、全部可用模型或没有可用候选的范围，采用基线的全目录轮换回退。显式保存会保留本次传入的不可用 ID。重新打开面板时，非空 Session scope 优先于 settings，与 Pi 基线一致；因此重新保存可能移除原 settings 中不在当前 scope 的不可用 ID。只有一个可用候选时，轮换返回空结果，不强行切换当前模型。范围本身不写入 v3 Session。

选择成功才更新界面状态、AgentSession、settings 的默认模型／thinking 以及 v3 Session 的配置历史。下一次 Provider 请求使用更新后的模型和有效 thinking。非 reasoning 模型会裁剪到 off，切回时恢复保存偏好，沿用 ADR-0025。生成中、重试等待或收尾中切换会明确报 busy；凭证已撤销、不支持的适配器和写入失败也不留下半更新配置。模型选择器取消保留编辑器草稿。

## SDK 与例子

运行 `go run ./examples/model-selection`，使用 Faux 离线展示选择器、thinking 选择和下一请求。公开构造器接收本地模型快照与强类型回调；ModelSelectorComponent 不直接写 settings，宿主应在选择回调中调用 AgentSession.SetModel。范围回调返回 error，失败时面板保留原选择；AgentSession.SetEnabledModels(ids, persist) 原子提交范围与可选保存。SDK 组件由宿主串行调用，TextUI.SetDialog 串行管理输入与渲染，组件回调不要重入 TextUI。

## 对等证据

固定 Pi `936aff00918de1187f085f123c2812d8f2d67745` 提取公开选择器、模型范围和真实 CLI PTY 三个 fixture。普通 Go 验收仅用锁定 fixture、Faux 和本地受控 SSE 服务，不访问在线 Provider。

```sh
go test -race ./codingagent -run '117|TestSessionConfiguration' -count=1
PIG_TEST_RACE=1 go test -race ./cmd/pig -run 117 -count=1
node parity/oracle/model-selector.mjs /path/to/locked-pi --check
node parity/oracle/model-scope.mjs /path/to/locked-pi --check
node parity/oracle/model-selection-cli.mjs /path/to/locked-pi --check
```

CLI 比较请求模型与 reasoning_effort、v3 配置历史、保存的范围和默认值，以及退出后重开 Session 继续生成。SDK 覆盖空列表、搜索定位、取消、不可用 ID、持久化失败、忙碌拒绝及凭证／适配器失败。

Catalog `contract:codingagent/model-selection` 保留 partial：`/settings` 当前仅交付 thinking 子菜单；网络刷新、完整设置面板、全部终端像素／Unicode／IME 行为、扩展运行时、图片与六平台运行验收不在本票完成声明中。包生态 #99 仍延期。本票不修改父 Issue #8。
