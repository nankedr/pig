# M5：加载与使用 Theme

主题是本地 JSON 资源。运行 `go run ./examples/themes` 查看内置 light 的 ANSI 样式和安全 HTML 导出；`go run ./examples/themes ./custom.json /tmp/theme.html` 可保留自定义主题导出。

## 格式与 SDK

以 `codingagent/themes/dark.json` 或 `light.json` 为完整模板，修改 `name`、`vars`、`colors` 和可选的 `export`。颜色支持六位十六进制 `#RRGGBB`、0–255 整数、空字符串（终端默认色）或变量名。变量可递归引用，缺失变量、循环引用、缺少必需颜色、非法类型和包含 `/` 的名称会失败。`thinkingMax` 缺省继承 `thinkingXhigh`，`scrollbarThumb` 缺省继承 `selectedBg`。

```go
theme, err := codingagent.LoadThemeFromPath("custom.json", codingagent.ColorModeTrueColor)
// 检查 err 后使用：
fmt.Println(theme.Bold(theme.FG(codingagent.ThemeColorAccent, "主题预览")))
colors := theme.ResolvedColors() // 独立 CSS 色值 map
```

`LoadBuiltinTheme` 支持 dark/light；`SelectTheme(name, loaded)` 先从加载结果选取同名主题，再查内置主题；空名称默认 dark，不存在的名称返回错误。`FG/BG`、ANSI 查询、基础样式、thinking/Bash 边框函数均可直接观察；可显式指定 truecolor 或 256color，缺省参考 `COLORTERM`。颜色名称使用公开常量。完整终端自动探测、主题选择对话框、全局主题/文件监听器及 TUI 适配器仍在 M6。

## 本地发现、禁用和信任

`DefaultResourceLoader.Reload(ctx)` 加载以下路径，首个同名主题获胜：

1. 可信项目 `.pig/settings.json` 的 `themes` 路径及 `.pig/themes`。
2. 全局 AgentDir 的 settings `themes` 路径及 `themes` 目录。
3. `AdditionalThemePaths` 或 CLI `--theme` 的显式文件/目录。

目录只读取直接子级 `.json` 普通文件，不递归。settings 相对路径以 `.pig` / AgentDir 为基准；显式路径以 CWD 为基准，支持 `~` 和 `file://`。沿用本地资源 `!pattern`、`+path`、`-path` 筛选。`NoThemes` / `--no-themes` 禁止自动发现，显式路径仍生效，内置主题仍可选择。

`GetThemes()` 返回主题及来源、冲突、无效文件和缺失显式路径诊断。项目主题和设置在首次加载前经过 trust gate；显式路径独立授权。返回的主题元数据、颜色 map、诊断与加载器隔离；重载成功后发布新快照，取消不替换旧结果。

`--theme` 添加资源；settings 的单数 `theme` 选择名称。例如：

```json
{"theme":"custom", "themes":["./my-themes"]}
```

## HTML 导出

```sh
pig --export session.jsonl /tmp/session.html --theme ./custom.json --no-themes
```

以上命令仍由全局或可信项目 settings 的 `theme` 选择 `custom`；未设置时为 dark。CLI、`ExportFromFile`、`AgentSession.ExportToHTML` 和既有 RPC 导出均使用主题。`ExportFromFileWithOptions(ctx, input, HTMLExportOptions{Theme: theme, OutputPath: path})` 可显式指定 SDK 主题，省略 Theme 时使用内置 dark；它不需要 settings。输出保留原会话 JSON，样式通过 CSS 变量注入。

`export.pageBg/cardBg/infoBg` 可覆盖页面、卡片和信息背景，否则按 Pi 从 `userMessageBg` 推导。空终端色在 HTML 中使用可读默认色。所有进入 CSS 的值先严格校验，未知颜色键不写入 CSS；不接受任意 CSS、HTML 或脚本。保留 CSP、转义、离线资产和真实浏览器 XSS 门禁；不读取主题 `$schema` URL。畸形 hex、export 变量错误会明确失败，这是对 Pi 的安全收紧。

## 验证与范围

```sh
node --experimental-strip-types parity/oracle/themes.mjs /path/to/locked-pi --check
go test ./codingagent ./cmd/pig -run 'Test(Themes|PigThemes|Issue104)' -count=1
make m4-html-browser
```

固定 Pi 基线 `936aff00918de1187f085f123c2812d8f2d67745` 的 fixture 包含 SDK ANSI/样式、发现与来源诊断，以及真实 Pi HTML 导出的 CSS；Go SDK、真实 CLI 子进程和 Chromium 重放验证。Catalog `contract:codingagent/themes` 以 partial 列明剩余兼容面，不把本地资源交付当成 M6 完成。按 ADR-0034，包 manifest、登记、npm/git、依赖和 lifecycle 仍由 #99 跟踪；不会下载、安装或执行扩展。完整 minimatch extglob、直接 Theme 构造器和六平台运行对等尚未冻结。
