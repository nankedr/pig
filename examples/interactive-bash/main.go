package main

import (
	"context"
	"fmt"
	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"os"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	dir, err := os.MkdirTemp("", "pig-interactive-bash-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		return err
	}
	reply, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("Bash output received"))
	core.SetResponses([]ai.FauxResponseStep{reply})
	model, _ := core.GetModel()
	created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: dir, AgentDir: dir, Model: &model, StreamFunction: agent.StreamFunction(core.StreamSimple), NoTools: codingagent.NoToolsAll})
	if err != nil {
		return err
	}
	defer created.Session.Dispose()
	for _, excluded := range []bool{false, true} {
		command := "printf 'hello from Bash\\n'"
		if excluded {
			command = "printf 'local only\\n'"
		}
		component := codingagent.NewBashExecutionComponent(command, excluded)
		result, err := created.Session.ExecuteBash(context.Background(), command, codingagent.ExecuteBashOptions{ExcludeFromContext: excluded, OnChunk: func(chunk string) { _ = component.AppendOutput(chunk) }})
		if err != nil {
			return err
		}
		_ = component.SetComplete(result.ExitCode, result.Cancelled, &codingagent.TruncationResult{Truncated: result.Truncated}, result.FullOutputPath)
		lines, err := component.Render(80)
		if err != nil {
			return err
		}
		for _, line := range lines {
			fmt.Println(line)
		}
	}
	fmt.Printf("history=%d, model messages=%d\n", len(created.Session.Messages()), len(codingagent.ConvertToLLM(created.Session.Messages())))
	return created.Session.Prompt(context.Background(), "summarize the command output")
}
