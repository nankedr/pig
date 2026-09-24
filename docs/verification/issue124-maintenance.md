# Issue #124 维护命令证据

平台：darwin-arm64。固定 Pi `936aff00918de1187f085f123c2812d8f2d67745`，普通验收只连接受控 loopback Provider。

`maintenance-cli.json` 来自真实 Pi CLI 与 PTY。相同 harness 验证 Pig：四条命令不进入用户历史；压缩摘要进入之后的模型输入；本地模板、Skill 和 Context File 修改经重载生效；带空格路径导出 HTML 内含压缩摘要；导出错误、摘要失败和取消后可以继续生成；取消/失败没有额外压缩记录；进程退出并恢复终端。

Pig 专项断言：两轮 usage 为 input 240 / output 40 / total 280，压缩后完整历史 total 420，当前上下文为 unknown。公开 SDK 注入可取消的资源加载器，验证失败、慢重载、输入回填、取消后继续、再次重载与 quit/Stop 等待工作结束。CLI 补充生成期 reload 忙提示、统计/导出与新增无效主题诊断。不是人工目视或六平台验收，精确布局/动画及其他 partial 边界见 ADR-0042。

复现命令见中文学习说明。fixture 的 input_hash / observation_hash 与 Catalog evidence 文件 hash 由测试校验。
