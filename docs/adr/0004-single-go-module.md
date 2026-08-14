# 使用单一 Go Module

Pig 使用单一 Go module `github.com/nankedr/pig`，顶层保持七个模块包，命令位于 `cmd/pig` 和 `cmd/pig-ai`。这些包已经表达 Pi 的依赖边界，而拆成多个 module 只会增加统一版本、开发和发布成本；确有独立生命周期时再另行决策。
