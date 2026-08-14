# 延后扩展运行时并先固定边界

早期阶段只完整盘点 Extension Surface、保留能力边界和显式 Stub，不承诺 TypeScript/JavaScript 源码兼容，也不冻结某种加载或通信 ABI。M5 先完成 package 获取与非扩展资源，M7 必须先比较 Go plugin、WASM、sidecar、脚本等方案并形成新的 ADR，之后才能实现扩展宿主。延后选择可避免一个未经验证的 Go 扩展形式演变为难以更换的事实标准。
