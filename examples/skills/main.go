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
	dir, err := os.MkdirTemp("", "pig-skills-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "SKILL.md")
	if err = os.WriteFile(path, []byte("---\nname: review\ndescription: 审查代码\ndisable-model-invocation: true\n---\n请审查用户指定的文件；辅助脚本 scripts/check.sh 相对于技能目录运行。"), 0600); err != nil {
		return err
	}
	loader, err := codingagent.NewDefaultResourceLoader(codingagent.DefaultResourceLoaderOptions{CWD: dir, AgentDir: dir, NoSkills: true, AdditionalSkillPaths: []string{path}})
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
	model, _ := core.GetModel()
	response := ai.FauxResponseFactory(func(input ai.Context, _ *ai.SimpleStreamOptions, _ *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
		message := input.Messages[len(input.Messages)-1].(ai.UserMessage)
		blocks, _ := message.Content.Blocks()
		for _, block := range blocks {
			if text, ok := block.(ai.TextContent); ok {
				fmt.Println("模型输入：" + text.Text)
			}
		}
		return ai.FauxAssistantMessage(ai.FauxAssistantText("已完成审查，可以继续提问。"))
	})
	core.SetResponses([]ai.FauxResponseStep{response, response})
	created, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: dir, AgentDir: dir, Model: &model, StreamFunction: agent.StreamFunction(core.StreamSimple), ResourceLoader: loader, NoTools: codingagent.NoToolsAll})
	if err != nil {
		return err
	}
	defer created.Session.Dispose()
	loaded, err := created.Session.ResourceLoader().GetSkills()
	if err != nil {
		return err
	}
	for _, p := range loaded.Skills {
		fmt.Printf("/skill:%s — %s\n来源：%s\n", p.Name, p.Description, p.FilePath)
	}
	for _, prompt := range []string{`/skill:review "src/my file.go"`, "继续解释测试方案"} {
		if err = created.Session.Prompt(ctx, prompt); err != nil {
			return err
		}
		reply, err := created.Session.GetLastAssistantText()
		if err != nil {
			return err
		}
		if reply != nil {
			fmt.Println(*reply)
		}
	}
	return nil
}
