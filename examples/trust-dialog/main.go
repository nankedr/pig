package main

import (
	"context"
	"fmt"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/tui"
	"os"
	"path/filepath"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	root, err := os.MkdirTemp("", "pig-trust-dialog-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	cwd, dir := filepath.Join(root, "project"), filepath.Join(root, "agent")
	if err = os.MkdirAll(filepath.Join(cwd, ".pig"), 0700); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(cwd, ".pig", "settings.json"), []byte(`{"defaultModel":"trusted-model"}`), 0600); err != nil {
		return err
	}
	settings, err := codingagent.NewSettingsManager(cwd, &dir)
	if err != nil {
		return err
	}
	options := codingagent.PrepareProjectSettingsOptions{CWD: cwd, AgentDir: dir, SettingsManager: settings}
	if len(os.Args) > 1 && os.Args[1] == "--interactive" {
		options.Terminal = tui.NewProcessTerminal(os.Stdin, os.Stdout)
	} else {
		options.Override = codingagent.ProjectTrustDecisionTrusted()
	}
	if err = codingagent.PrepareProjectSettings(context.Background(), options); err != nil {
		return err
	}
	trusted, _ := settings.IsProjectTrusted()
	model, _ := settings.GetDefaultModel()
	fmt.Printf("Project Trust: %v; model: %s\n", trusted, model)
	return nil
}
