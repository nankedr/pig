# 采用无 CGO 核心与分阶段平台门

Pig 使用原生 Go 1.24，核心保持 CGO-free，不承担 browser、WebAssembly 或 Workers 兼容；最终目标是 macOS、Linux、Windows 的 amd64/arm64 及可安装的 Termux。M0/M1 只以本机 darwin-arm64 为硬门，六个原生目标及平台行为在 M13 验收。该分期让早期聚焦架构学习，同时保留可发布的跨平台终局。
