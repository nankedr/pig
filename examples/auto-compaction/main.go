package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
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
	ctx := context.Background()
	dir, err := os.MkdirTemp("", "pig-auto-compaction-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	sessionDir := filepath.Join(dir, "sessions")
	manager, err := codingagent.NewSessionManager(dir, &sessionDir)
	if err != nil {
		return err
	}
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		return err
	}
	model, _ := core.GetModel()
	model.ContextWindow = 1000
	model.MaxTokens = 100
	generations := 0
	stream := func(ctx context.Context, m ai.Model, input ai.Context, o ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
		system, _ := input.SystemPrompt.Value()
		text := "当前步骤已完成。"
		if strings.Contains(system, "context summarization assistant") {
			text = "## Goal\n完成 Go 项目；保持接口兼容。\n## Next Steps\n继续验证测试。"
		} else {
			generations++
			if len(input.Messages) > 0 {
				data, _ := json.Marshal(input.Messages[0])
				if strings.Contains(string(data), "## Goal") {
					fmt.Println("后续模型输入已包含压缩摘要。")
				}
			}
		}
		response, _ := ai.FauxAssistantMessage(ai.FauxAssistantText(text))
		response.Usage = ai.Usage{Input: 20, TotalTokens: 20}
		if generations == 3 && !strings.Contains(system, "context summarization assistant") {
			response.StopReason = ai.StopReasonError
			response.ErrorMessage = ai.Some("prompt is too long")
			response.Usage = ai.Usage{}
		}
		core.SetResponses([]ai.FauxResponseStep{response})
		return core.StreamSimple(ctx, m, input, o)
	}
	settings, err := codingagent.NewInMemorySettingsManager(codingagent.Settings{Compaction: &codingagent.CompactionSettings{Enabled: true, ReserveTokens: 100, KeepRecentTokens: 10}})
	if err != nil {
		return err
	}
	config := codingagent.CreateAgentSessionOptions{CWD: dir, Model: &model, StreamFunction: stream, SessionManager: manager, SettingsManager: settings, NoTools: codingagent.NoToolsAll}
	created, err := codingagent.CreateAgentSession(ctx, config)
	if err != nil {
		return err
	}
	defer created.Session.Dispose()
	for _, prompt := range []string{"完成这个 Go 项目，保持接口兼容。", "最近的任务是验证测试和确认所有变更满足接口兼容要求。"} {
		if err = created.Session.Prompt(ctx, prompt); err != nil {
			return err
		}
	}
	created.Session.Subscribe(func(event codingagent.AgentSessionEvent) {
		if end, ok := event.(codingagent.AgentSessionCompactionEndEvent); ok {
			fmt.Printf("自动压缩：reason=%s willRetry=%v\n", end.Reason, end.WillRetry)
		}
	})
	runtime := codingagent.NewAgentSessionRuntime(created.Session, codingagent.AgentSessionServices{}, nil, nil, nil)
	outcome, err := codingagent.RunHeadless(ctx, runtime, codingagent.HeadlessRunOptions{Messages: []string{"继续，模拟这轮上下文溢出。"}})
	if err != nil {
		return err
	}
	fmt.Printf("恢复生成：%s\n", strings.Join(outcome.Text, "\n"))
	reopened, err := codingagent.OpenSessionManager(*manager.GetSessionFile(), nil, nil)
	if err != nil {
		return err
	}
	config.SessionManager = reopened
	next, err := codingagent.CreateAgentSession(ctx, config)
	if err != nil {
		return err
	}
	defer next.Session.Dispose()
	runtime = codingagent.NewAgentSessionRuntime(next.Session, codingagent.AgentSessionServices{}, nil, nil, nil)
	outcome, err = codingagent.RunHeadless(ctx, runtime, codingagent.HeadlessRunOptions{Messages: []string{"继续"}})
	if err != nil {
		return err
	}
	fmt.Printf("v3 会话重开后继续完成：%s\n", strings.Join(outcome.Text, "\n"))
	return nil
}
