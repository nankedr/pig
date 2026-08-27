# Pig

Pig 是 Pi 固定版本的 Go 语义兼容实现。当前 Milestone Frontier 是 M1；M0 已建立七包、双命令、Parity Catalog、Capability Stub 与冻结门禁骨架。

- [文档导航](docs/README.md)
- [M0 兼容骨架](docs/learning/m0-compatibility-skeleton.md)
- [M0 TypeScript 到 Go 导航](docs/mappings/typescript-to-go/m0.md)

```sh
make m0-gate
```

`m0-gate` 只重放仓库内已提交的 fixture，全程离线且不需要 Pi checkout。需要重新对照上游源码时，按 [M0 兼容骨架](docs/learning/m0-compatibility-skeleton.md#冻结门禁) 准备两个独立 checkout 后运行 `m0-freeze`。
