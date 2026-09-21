package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	ctx := context.Background()
	dir, err := os.MkdirTemp("", "pig-model-selection-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	credentials, err := codingagent.NewAuthStorage(filepath.Join(dir, "auth.json"))
	if err != nil {
		return err
	}
	_, err = credentials.Modify(ctx, "deepseek", func(context.Context, ai.Credential) (ai.Credential, error) {
		return ai.APIKeyCredential{Type: ai.AuthTypeAPIKey, Key: ai.Some("offline-example")}, nil
	}, ai.AuthOperationOptions{})
	if err != nil {
		return err
	}
	runtime, err := codingagent.NewModelRuntime(ctx, codingagent.CreateModelRuntimeOptions{Offline: true, Credentials: credentials})
	if err != nil {
		return err
	}
	first, _, err := runtime.GetModel("deepseek", "deepseek-v4-flash")
	if err != nil {
		return err
	}
	second, _, err := runtime.GetModel("deepseek", "deepseek-v4-pro")
	if err != nil {
		return err
	}
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		return err
	}
	response := ai.FauxResponseFactory(func(input ai.Context, options *ai.SimpleStreamOptions, _ *ai.FauxProviderState, model ai.Model) (ai.AssistantMessage, error) {
		names := []string{}
		for _, tool := range input.Tools {
			names = append(names, tool.Name)
		}
		thinking := "off"
		if options.Reasoning != nil {
			thinking = string(*options.Reasoning)
		}
		fmt.Printf("请求：model=%s thinking=%s tools=%v\n", model.ID, thinking, names)
		return ai.FauxAssistantMessage(ai.FauxAssistantText("完成"))
	})
	core.SetResponses([]ai.FauxResponseStep{response, response})
	created, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: dir, Model: &first, ModelRuntime: runtime, StreamFunction: agent.StreamFunction(core.StreamSimple), ThinkingLevel: "off"})
	if err != nil {
		return err
	}
	s := created.Session
	defer s.Dispose()
	if err = s.Prompt(ctx, "读取项目"); err != nil {
		return err
	}

	available, err := runtime.GetAvailable(ctx)
	if err != nil {
		return err
	}
	var selectionErr error
	selector := codingagent.NewModelSelectorComponent(available, &first, nil, func(model ai.Model) { selectionErr = s.SetModel(model) }, func() { fmt.Println("已取消") }, "v4-pro")
	if lines, err := selector.Render(80); err != nil {
		return err
	} else {
		for _, line := range lines {
			fmt.Println(line)
		}
	}
	if err = selector.HandleInput("\r"); err != nil {
		return err
	}
	if selectionErr != nil {
		return selectionErr
	}
	levels, err := s.GetAvailableThinkingLevels()
	if err != nil {
		return err
	}
	thinking := codingagent.NewThinkingSelectorComponent(s.ThinkingLevel(), levels, func(level agent.ThinkingLevel) { selectionErr = s.SetThinkingLevel(level) }, nil)
	if err = thinking.HandleInput("\x1b[A"); err != nil {
		return err
	}
	if err = thinking.HandleInput("\r"); err != nil {
		return err
	}
	if selectionErr != nil {
		return selectionErr
	}
	if err = s.SetEnabledModels([]string{string(second.Provider) + "/" + second.ID}, true); err != nil {
		return err
	}
	return s.Prompt(ctx, "使用选定模型继续")
}
