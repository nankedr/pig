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
	root, err := os.MkdirTemp("", "pig-session-reload-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(root)
	if err = os.Setenv("HOME", filepath.Join(root, "home")); err != nil {
		return err
	}
	cwd, dir := filepath.Join(root, "repo"), filepath.Join(root, "agent")
	for _, p := range []string{cwd, filepath.Join(dir, "prompts")} {
		if err = os.MkdirAll(p, 0700); err != nil {
			return err
		}
	}
	write := func(version string) error {
		if err := os.WriteFile(filepath.Join(dir, "SYSTEM.md"), []byte(version), 0600); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, "prompts/review.md"), []byte(version+"：审查 $1"), 0600)
	}
	if err = write("第一版"); err != nil {
		return err
	}
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		return err
	}
	model, ok := core.GetModel()
	if !ok {
		return fmt.Errorf("Faux model unavailable")
	}
	response := ai.FauxResponseFactory(func(c ai.Context, _ *ai.SimpleStreamOptions, _ *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
		system, _ := c.SystemPrompt.Value()
		fmt.Printf("模型收到 system prompt：%s\n历史消息数：%d\n", system, len(c.Messages))
		return ai.FauxAssistantMessage(ai.FauxAssistantText("审查完成"))
	})
	core.SetResponses([]ai.FauxResponseStep{response, response})
	ctx := context.Background()
	created, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: cwd, AgentDir: dir, Model: &model, StreamFunction: agent.StreamFunction(core.StreamSimple), Tools: []string{"read"}})
	if err != nil {
		return err
	}
	s := created.Session
	defer s.Dispose()
	if err = s.Prompt(ctx, "/review main.go"); err != nil {
		return err
	}
	id := s.SessionID()
	if err = write("第二版"); err != nil {
		return err
	}
	if err = s.Reload(ctx); err != nil {
		return err
	}
	prompts, err := s.PromptTemplates()
	if err != nil {
		return err
	}
	fmt.Printf("会话 ID 保持：%t；新模板：%s\n", id == s.SessionID(), prompts[0].Content)
	return s.Prompt(ctx, "/review main.go")
}
