# Issue #123 本机编辑器往返

平台：darwin-arm64；Pi 对等基线：`936aff00918de1187f085f123c2812d8f2d67745`。真实 `/usr/bin/vim -Nu NONE -n`，由受控 PTY 自动发送编辑和保存按键，Provider 为 loopback SSE，不使用在线凭证。

命令：`python3 parity/terminal/external-editor.py /tmp/pig-external-editor --pig --vim --fullscreen --failures`（先按学习说明构建）。

实际观察：后台生成期间终端无 Pig 输出；编辑器启动时为 cooked mode；初始文本为 `本次草稿 🐷\nsecond line`；参数为 `arg`、空字符串、字面 `"literal"`；保存后 Provider 收到 `外部编辑 ✅\nsecond changed`。退出 7、130、命令不存在和文件删除后的读取失败均保留对应原稿，继续提交成功。临时目录均删除，CLI 退出码 0，控制终端恢复到启动前状态。

这是已执行的本机编辑器演示，不代表六平台门禁或人工目视验收。普通离线测试不要求安装 Vim；锁定 Pi fixture 由受控编辑器产生。
