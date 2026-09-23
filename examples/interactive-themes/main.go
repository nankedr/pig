package main

import (
	"fmt"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/tui"
)

func main() {
	setting := "light/dark"
	settings, err := codingagent.NewInMemorySettingsManager(codingagent.Settings{Theme: &setting})
	must(err)
	controller := codingagent.NewThemeController(settings, codingagent.ThemeLoadResult{})
	must(controller.ApplySettings())
	must(controller.SetTerminalTheme(tui.TerminalColorSchemeLight))
	fmt.Println("automatic:", controller.Current().Name)
	must(controller.Preview("dark"))
	fmt.Println("preview:", controller.Current().Name)
	must(controller.ApplySettings())
	fmt.Println("cancel:", controller.Current().Name)
	must(controller.SetTheme("dark"))
	saved, err := settings.GetThemeSetting()
	must(err)
	fmt.Println("saved:", saved)
	language := "go"
	for _, line := range controller.Current().MarkdownTheme().HighlightCode("func main() { println(42) }", &language) {
		fmt.Println(line)
	}
}
func must(err error) {
	if err != nil {
		panic(err)
	}
}
