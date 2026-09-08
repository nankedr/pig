package main

import (
	"context"
	"fmt"
	"os"

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
	dir, err := os.MkdirTemp("", "pig-session-bash-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		return err
	}
	model, ok := core.GetModel()
	if !ok {
		return fmt.Errorf("Faux model unavailable")
	}
	reply, err := ai.FauxAssistantMessage(ai.FauxAssistantText("已读取 Bash 结果"))
	if err != nil {
		return err
	}
	core.SetResponses([]ai.FauxResponseStep{reply})
	created, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: dir, AgentDir: dir, Model: &model, NoTools: codingagent.NoToolsAll, StreamFunction: func(ctx context.Context, m ai.Model, in ai.Context, o ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
		fmt.Printf("模型收到 %d 条消息（不包含排除的 Bash）\n", len(in.Messages))
		return core.StreamSimple(ctx, m, in, o)
	}})
	if err != nil {
		return err
	}
	session := created.Session
	defer session.Dispose()
	id := "build-check"
	result, err := session.ExecuteBash(ctx, "printf 'hello from Bash\\n'", codingagent.ExecuteBashOptions{ID: &id, OnChunk: func(chunk string) { fmt.Print("chunk: ", chunk) }})
	if err != nil {
		return err
	}
	fmt.Printf("final: exit=%d cancelled=%v truncated=%v\n", *result.ExitCode, result.Cancelled, result.Truncated)
	if _, err := session.ExecuteBash(ctx, "printf 'local-only'", codingagent.ExecuteBashOptions{ExcludeFromContext: true}); err != nil {
		return err
	}
	if err := session.Prompt(ctx, "根据执行结果继续"); err != nil {
		return err
	}
	_, err = session.ExecuteBash(ctx, "printf 'ready\\n'; sleep 30", codingagent.ExecuteBashOptions{OnChunk: func(string) { session.AbortBash() }})
	return err
}
