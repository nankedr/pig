# V1 发布验收：TypeScript → Go

固定 Pi Code Baseline 为 `936aff00918de1187f085f123c2812d8f2d67745`，Catalog Baseline 独立锁定为 `53fa77ccd8a279eb87e92294ef3687b03ff80112`。V1 为范围子集，完整七模块对等归 #127。

| 兼容面 | Pig 验收入口 | 证据来源 |
| --- | --- | --- |
| legacy Agent 与 production v3、M0–M6 | `make m6-gate`、`internal/m6gate` | 原有独立契约、固定 fixture、SDK/CLI 普通/全屏 PTY |
| DeepSeek Responses 与本地历史 | `ai/openai_responses.go`、`codingagent`、`cmd/pig` Responses tests | 共同协议固定 Pi Oracle；当前服务独立 fixture 与受保护 smoke |
| 用户/工具图片与附件/预览/HTML | `codingagent/*images*`、`tui/image_preview.go`、`cmd/pig/image_workflow*` | 图片固定 Oracle、公开进程、NSPasteboard、浏览器与双 API smoke |
| 分支范围与 V2 剩余项 | `parity/v1-acceptance.json`、`scripts/v1-audit.py` | Catalog 的 134 个 V1 锚点及独立最高层契约哈希，状态不提升 |
| 独立 SDK/安装 | `examples/v1-workflow`、`scripts/v1-release.py` | 合成图片、工具 continuation、v3 恢复/导出/压缩，公开 module Origin |
| 发布门禁 | `make v1-freeze`、`scripts/v1-live.py`、`internal/v1gate` | 确切 commit、无 skip 的执行结果、制品与日志 SHA-256 |

功能源码导航继续见 [Responses Coding Agent](v1-responses-codingagent.md)、[用户图片](v1-user-images.md)、[工具图片](v1-tool-images.md)和[图片工作流](v1-image-workflow.md)。复现与发布顺序见 [V1 冻结](../../learning/v1-freeze.md)。
