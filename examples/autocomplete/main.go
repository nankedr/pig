package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/tui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	root, err := os.MkdirTemp("", "pig-autocomplete-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	dir := filepath.Join(root, "agent")
	if err = os.MkdirAll(filepath.Join(dir, "prompts"), 0700); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(dir, "prompts/review.md"), []byte("---\ndescription: Review a topic\n---\nReview $1 carefully."), 0600); err != nil {
		return err
	}
	provider, err := ai.NewFauxProvider()
	if err != nil {
		return err
	}
	response, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("Template reached the conversation."))
	provider.SetResponses([]ai.FauxResponseStep{response})
	model, _ := provider.GetModel()
	ctx := context.Background()
	created, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: root, AgentDir: dir, Provider: provider.Provider, Model: &model, NoTools: codingagent.NoToolsAll})
	if err != nil {
		return err
	}
	defer created.Session.Dispose()
	completion, err := codingagent.NewSessionAutocompleteProvider(created.Session, nil)
	if err != nil {
		return err
	}
	suggestions, ok, err := completion.GetSuggestions(ctx, []string{"/rev"}, 0, 4, tui.AutocompleteOptions{})
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("missing review template")
	}
	result, err := completion.ApplyCompletion([]string{"/rev"}, 0, 4, suggestions.Items[0], suggestions.Prefix)
	if err != nil {
		return err
	}
	prompt := result.Lines[0] + "Unicode"
	fmt.Println("Selected:", prompt)
	if err = created.Session.Prompt(ctx, prompt); err != nil {
		return err
	}
	fmt.Println("Faux conversation completed; no network or credentials.")
	return nil
}
