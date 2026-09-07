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
	dir, err := os.MkdirTemp("", "pig-configuration-")
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
	if err = s.SetScopedModels([]codingagent.ScopedModel{{Model: first}, {Model: second, ThinkingLevel: "max"}}); err != nil {
		return err
	}
	if _, err = s.CycleModel(ctx); err != nil {
		return err
	}
	if err = s.SetActiveToolsByName([]string{"read", "write"}); err != nil {
		return err
	}
	return s.Prompt(ctx, "继续修改")
}
