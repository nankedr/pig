package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

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
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	dir, err := os.MkdirTemp("", "pig-maintenance-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	if err = os.MkdirAll(filepath.Join(dir, "prompts"), 0700); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(dir, "prompts", "demo.md"), []byte("维护模板：$1"), 0600); err != nil {
		return err
	}
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		return err
	}
	model, _ := core.GetModel()
	stream := func(ctx context.Context, m ai.Model, input ai.Context, o ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
		text := "离线回复完成。可输入 /compact、/reload、/session、/export；Ctrl+D 退出。"
		if system, _ := input.SystemPrompt.Value(); strings.Contains(system, "context summarization assistant") {
			text = "## Goal\n验证交互维护命令。\n## Next Steps\n继续检查资源与会话。"
		}
		response, _ := ai.FauxAssistantMessage(ai.FauxAssistantText(text))
		core.SetResponses([]ai.FauxResponseStep{response})
		return core.StreamSimple(ctx, m, input, o)
	}
	settings, err := codingagent.NewInMemorySettingsManager(codingagent.Settings{Compaction: &codingagent.CompactionSettings{Enabled: false, ReserveTokens: 1000, KeepRecentTokens: 10}})
	if err != nil {
		return err
	}
	created, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: dir, AgentDir: dir, Model: &model, StreamFunction: stream, SettingsManager: settings, NoTools: codingagent.NoToolsAll})
	if err != nil {
		return err
	}
	runtime := codingagent.NewAgentSessionRuntime(created.Session, codingagent.AgentSessionServices{AgentDir: dir}, nil, nil, nil)
	return codingagent.NewInteractiveMode(runtime, codingagent.InteractiveModeOptions{InitialMessages: []string{"请解释如何保留会话历史并压缩当前上下文。", "下一步是检查本地资源重载和安全导出。"}}).Run(ctx)
}
