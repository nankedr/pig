package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	dir, err := os.MkdirTemp("", "pig-prompts-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	if err = os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("请用中文解释修改。"), 0600); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(dir, "SYSTEM.md"), []byte("你是代码审查助手。"), 0600); err != nil {
		return err
	}
	loader, err := codingagent.NewDefaultResourceLoader(codingagent.DefaultResourceLoaderOptions{CWD: dir, AgentDir: dir, AppendSystemPrompt: []string{"先解释问题，再提出修改。"}})
	if err != nil {
		return err
	}
	if err = loader.Reload(ctx); err != nil {
		return err
	}
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		return err
	}
	response := ai.FauxResponseFactory(func(input ai.Context, _ *ai.SimpleStreamOptions, _ *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
		prompt, _ := input.SystemPrompt.Value()
		if !strings.Contains(prompt, "请用中文解释修改。") || !strings.HasPrefix(prompt, "你是代码审查助手。\n\n先解释问题，再提出修改。") {
			return ai.AssistantMessage{}, fmt.Errorf("资源提示词未进入模型请求")
		}
		return ai.FauxAssistantMessage(ai.FauxAssistantText("已收到项目指令，继续任务。"))
	})
	core.SetResponses([]ai.FauxResponseStep{response, response})
	model, _ := core.GetModel()
	created, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: dir, AgentDir: dir, Model: &model, StreamFunction: agent.StreamFunction(core.StreamSimple), ResourceLoader: loader, NoTools: codingagent.NoToolsAll})
	if err != nil {
		return err
	}
	defer created.Session.Dispose()
	source, err := loader.GetSystemPromptSource()
	if err != nil {
		return err
	}
	fmt.Printf("System 来源：%s\n", source.Path)
	fmt.Println(created.Session.SystemPrompt())
	files, err := created.Session.ResourceLoader().GetAgentsFiles()
	if err != nil {
		return err
	}
	for _, f := range files {
		fmt.Printf("来源：%s\n%s\n", f.Path, f.Content)
	}
	for _, prompt := range []string{"开始任务", "继续任务"} {
		if err = created.Session.Prompt(ctx, prompt); err != nil {
			return err
		}
		text, err := created.Session.GetLastAssistantText()
		if err != nil {
			return err
		}
		if text != nil {
			fmt.Println(*text)
		}
	}
	return nil
}
