package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func main() {
	theme, err := codingagent.LoadBuiltinTheme("light", codingagent.ColorModeTrueColor)
	must(err)
	if len(os.Args) > 1 {
		theme, err = codingagent.LoadThemeFromPath(os.Args[1], codingagent.ColorModeTrueColor)
		must(err)
	}
	fmt.Println(theme.Bold(theme.FG(codingagent.ThemeColorAccent, "Pig Theme: "+theme.Name)))
	fmt.Println("accent:", theme.ResolvedColors()["accent"])
	dir, err := os.MkdirTemp("", "pig-themes-")
	must(err)
	defer os.RemoveAll(dir)
	manager, err := codingagent.NewSessionManager(dir, &dir)
	must(err)
	_, err = manager.AppendMessage(ai.UserMessage{Role: "user", Content: ai.UserText("# Theme 示例\n\n本地主题与 **安全 HTML 导出**。"), Timestamp: 1})
	must(err)
	_, err = manager.AppendMessage(ai.AssistantMessage{Role: "assistant", Content: []ai.AssistantContent{ai.TextContent{Type: "text", Text: "主题颜色由公开 SDK 解析，导出不依赖终端。"}}, StopReason: ai.StopReasonStop, Timestamp: 2})
	must(err)
	output := filepath.Join(dir, "theme.html")
	if len(os.Args) > 2 {
		output = os.Args[2]
	}
	path, err := codingagent.ExportFromFileWithOptions(context.Background(), *manager.GetSessionFile(), codingagent.HTMLExportOptions{OutputPath: output, Theme: theme})
	must(err)
	fmt.Println("HTML:", path, "（传入第二个参数可保留输出）")
}
func must(err error) {
	if err != nil {
		panic(err)
	}
}
