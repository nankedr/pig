package main

import (
	"context"
	"fmt"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func main() {
	provider, err := ai.NewFauxProvider()
	must(err)
	model, ok := provider.GetModel()
	if !ok {
		panic("Faux model unavailable")
	}
	created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{Model: &model, Provider: provider.Provider, NoTools: codingagent.NoToolsAll})
	must(err)
	defer created.Session.Dispose()
	session := created.Session
	config, err := session.GetInteractionSettings()
	must(err)
	must(session.UpdateInteractionSetting("steering-mode", "all"))
	config, err = session.GetInteractionSettings()
	must(err)
	fmt.Printf("steering=%s, thinking=%s, layout=%s\n", config.SteeringMode, config.ThinkingLevel, config.TUIMode)
	menu := codingagent.NewSettingsSelectorComponent(config, codingagent.SettingsCallbacks{OnCancel: func() { fmt.Println("settings closed") }})
	must(menu.HandleInput("Steering mode"))
	lines, err := menu.Render(90)
	must(err)
	for _, line := range lines {
		fmt.Println(line)
	}
	must(menu.HandleInput("\x1b"))
}
func must(err error) {
	if err != nil {
		panic(err)
	}
}
