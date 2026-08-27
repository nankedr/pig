# 采用双来源对等基线

Pig V1 的 Code Baseline 固定为 Pi commit `936aff00918de1187f085f123c2812d8f2d67745`，Catalog Baseline 固定为 Pi v0.84.1 官方 source tar（来源 commit `53fa77ccd8a279eb87e92294ef3687b03ff80112`，39 个 Provider、1220 个 chat model，SHA-256 `294d8067eb42327be0db4792d3be792daff588d8fc22549270a972ec9e5407e7`）。`53fa77c` 比 `936aff0` 早 40 个 commit；`936aff0` 当次生成的目录 artifact 已过期，且没有可恢复的发布副本，已知日志与 hash 不能重建其内容，因此选择最近的较早官方不可变 MIT 发布物，而不使用调查当天的实时抓取。

这组制品构成可审计的双来源 Parity Baseline，但不是 fixed-run parity：运行时语义、API 和 Oracle 结论以 Code Baseline 为准，内嵌模型目录内容以 Catalog Baseline 为准；任何证据都不得声称 `936aff0` 的生成器曾产出 v0.84.1 目录。
