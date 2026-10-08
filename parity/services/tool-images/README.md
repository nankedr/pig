# 测试图片来源

这些 PNG、JPEG、GIF、BMP、WebP 均由 Pig 自行生成，无外部照片或第三方媒体。源代码为 `generate.go`；版权与再分发遵循仓库根目录 MIT License（Copyright © 2026 kzz）。损坏 PNG 为刻意截断的人工测试数据。

在仓库根目录运行 `go run ./parity/services/tool-images/generate.go` 可重建色块、EXIF 1–8、BMP 和大逻辑画布/小偏移首帧 GIF。用锁定的 Pi checkout 重跑 `parity/oracle/read-images.mjs` 生成/比较独立语义观测。Go 编解码版本变化可能影响原始字节，应审查后更新 fixture；不能自行将 Pig 输出作为 Pi 预期。

引用的 `../user-image.png` 沿用 #134 的测试图片，不由此生成器覆盖。

WebP 输入由同一自产 wide.png 经锁定 Pi Photon 编码，再添加标准 VP8X/EXIF 1–8 元数据。执行 `node parity/services/tool-images/generate-webp.mjs /locked/pi /absolute/pig/parity/services/tool-images/wide.png /absolute/pig/parity/services/tool-images` 重建；Photon 代码为 Pi 的 MIT 第三方依赖，图片内容为 Pig 自产测试色块，不含外部媒体。
