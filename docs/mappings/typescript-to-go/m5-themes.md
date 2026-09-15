# M5 Theme：TypeScript → Go

| 固定 Pi 入口 | Pig 入口 | 验证点 |
| --- | --- | --- |
| `modes/interactive/theme/theme.ts`: loadThemeFromPath / Theme | `codingagent/themes.go`、`ui.go` | JSON、变量、ANSI、CSS 与样式 |
| 内置 `dark.json` / `light.json` | `codingagent/themes/` + go:embed | 独立运行、manifest 哈希与 MIT 来源 |
| `core/resource-loader.ts`: getThemes / reload | `codingagent/theme_resources.go`、`resources.go` | 来源、优先级、trust、禁用、冲突和快照 |
| `core/export-html/index.ts` | `codingagent/theme_export.go`、`export_html.go` | 颜色推导、安全 CSS、SDK/CLI/RPC 导出 |
| CLI `--theme` / `--no-themes` | `codingagent/cli.go`、`misc.go`、`headless.go` | 显式路径与诊断 |

- 对等用例：`parity/oracle/themes.mjs` → `fixtures/themes.json`。
- 公开 SDK：`codingagent/issue104_themes_test.go`、`issue104_trust_test.go`。
- 真实 CLI：`cmd/pig/issue104_themes_test.go`；浏览器：`parity/export-html/check.mjs`。
- API snapshot：`codingagent/testdata/issue104_surface_golden.txt`。
- [可运行示例](../../../examples/themes/main.go) · [中文学习材料](../../learning/m5-themes.md)。
- 精确能力范围：Catalog `contract:codingagent/themes`；M6 交互主题能力仍延期。
