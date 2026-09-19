# M6.6 对话框与项目信任映射

| Pi 固定基线边界 | Pig 公开边界 | 验证 |
| --- | --- | --- |
| core/project-trust.ts resolveProjectTrusted | codingagent.PrepareProjectSettings | CLI PTY 双启动、SDK 优先级 / 保存失败 |
| core/trust-manager.ts getProjectTrustOptions | codingagent.GetProjectTrustOptions | 当前 / 父目录、持久 / 本次决定 |
| cli/startup-ui.ts showStartupSelector | tui.ShowSelectDialog / SelectDialog | 输入确认取消、终端恢复 |
| components/select-list.ts SelectList | tui.SelectList | dialogs.json |
| tui.ts showOverlay / OverlayHandle | tui.TUIBase.ShowOverlay / OverlayHandle | overlays.json、焦点 / 缩放 / 遮挡 |

Go 使用 context 传递取消，通过 Terminal 接口注入终端；不执行 M7 的扩展 trust handler。安全偏离与既有 ADR-0010 一致。权威能力与证据登记在 `contract:tui/dialogs-trust`；未验收分支仍为 partial。
