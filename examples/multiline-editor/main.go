package main

import (
	"fmt"

	"github.com/nankedr/pig/tui"
)

func main() {
	editor := tui.NewEditor(nil, tui.EditorTheme{})
	editor.SetFocusState(true)
	editor.SetOnSubmit(func(text string) { fmt.Printf("提交文本：%q\n", text) })
	_ = editor.SetText("你好\nsecond line")
	_ = editor.HandleInput("\x01")
	_ = editor.InsertTextAtCursor("第二行：")
	_ = editor.HandleInput("\x1b[200~\n粘贴\r\n保留换行\x1b[201~")
	fmt.Printf("光标（行、UTF-8 字节列）：%+v\n", editor.GetCursor())
	expanded, _ := editor.GetExpandedText()
	fmt.Printf("展开文本：%q\n", expanded)
	_ = editor.HandleInput("\r")
	_ = editor.AddToHistory(expanded)
	_ = editor.HandleInput("\x1b[A")
	fmt.Printf("历史回取：%q\n", editor.GetText())
}
