# 采用强类型编写与类型擦除注册表

Provider、API Adapter 和 Tool 的作者侧 API 使用强类型描述符，异构运行时注册表在经过校验后擦除具体类型；开放消息保留 raw fallback，原始 JSON Schema 始终是 Tool 与协议校验的权威。仅为真实可替换 seam 定义小接口，三态字段显式表达 absent/null/zero，内建动态 schema 通过生成器得到强类型 helper。这样同时保留 Go 编译期体验与 Pi 的动态扩展能力，而无需仿造 TypeScript 类型技巧。
