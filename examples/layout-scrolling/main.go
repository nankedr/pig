package main

import (
	"context"
	"fmt"
	"github.com/nankedr/pig/tui"
	"io"
	"os"
	"strings"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	ui := tui.NewTextUI(tui.NewProcessTerminal(os.Stdin, os.Stdout), tui.TextUIOptions{Mode: tui.TUIModeFullscreen, Scrollbar: tui.ScrollViewScrollbarAlways, PreserveScreen: true})
	if err := ui.Start(); err != nil {
		return err
	}
	defer ui.Stop()
	var text strings.Builder
	for i := 0; i < 80; i++ {
		fmt.Fprintf(&text, "历史 %02d — 中文与 emoji 👋\n", i)
	}
	text.WriteString("输入 /mode 切换普通/全屏，/top、/end 进入全屏并跳到首尾，/quit 退出。\n全屏支持 Home/End、PageUp/PageDown、鼠标滚轮与滚动条拖动。\n")
	if err := ui.Append(text.String()); err != nil {
		return err
	}
	for {
		input, err := ui.ReadLine(context.Background())
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch input {
		case "/quit":
			return nil
		case "/mode":
			mode := tui.TUIModeRegular
			if ui.Mode() == mode {
				mode = tui.TUIModeFullscreen
			}
			err = ui.SetMode(mode)
		case "/top":
			if err = ui.SetMode(tui.TUIModeFullscreen); err == nil {
				err = ui.ScrollToTop()
			}
		case "/end":
			if err = ui.SetMode(tui.TUIModeFullscreen); err == nil {
				err = ui.ScrollToBottom()
			}
		default:
			err = ui.Append(input + "\n")
		}
		if err != nil {
			return err
		}
	}
}
