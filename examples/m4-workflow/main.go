package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

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
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	dir, err := os.MkdirTemp("", "pig-m4-workflow-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	manager, err := codingagent.NewSessionManager(dir, &dir)
	if err != nil {
		return err
	}
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		return err
	}
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
	modelRuntime, err := codingagent.NewModelRuntime(ctx, codingagent.CreateModelRuntimeOptions{Offline: true, Credentials: credentials})
	if err != nil {
		return err
	}
	model, _, err := modelRuntime.GetModel("deepseek", "deepseek-v4-flash")
	if err != nil {
		return err
	}
	delay := int64(1)
	settings, err := codingagent.NewInMemorySettingsManager(codingagent.Settings{
		Retry:      &codingagent.RetrySettings{BaseDelayMS: &delay},
		Compaction: &codingagent.CompactionSettings{Enabled: false, ReserveTokens: 100, KeepRecentTokens: 10},
	})
	if err != nil {
		return err
	}
	var session *codingagent.AgentSession
	calls, retries := 0, 0
	checkConfig, sawConfig, sawSummary := false, false, false
	summaryText, expectedSummary := "", ""
	stream := func(ctx context.Context, m ai.Model, input ai.Context, options ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
		calls++
		reply, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("完成任务并保留验证结果。"))
		if summaryText != "" {
			reply, _ = ai.FauxAssistantMessage(ai.FauxAssistantText(summaryText))
		} else if calls == 1 {
			if e := session.Steer("先检查接口兼容性"); e != nil {
				err = e
			}
			if e := session.FollowUp("随后验证恢复行为"); e != nil {
				err = e
			}
			reply, _ = ai.FauxAssistantMessage(ai.FauxAssistantText("partial"), ai.FauxAssistantMessageOptions{StopReason: ai.Some(ai.StopReasonError), ErrorMessage: ai.Some("503")})
		} else {
			if checkConfig {
				sawConfig = m.ID == "deepseek-v4-pro" && len(input.Tools) == 1 && input.Tools[0].Name == "read" && options.Reasoning != nil && string(*options.Reasoning) == "max"
			}
			data, _ := json.Marshal(input.Messages)
			sawSummary = strings.Contains(string(data), expectedSummary)
		}
		core.SetResponses([]ai.FauxResponseStep{reply})
		return core.StreamSimple(ctx, m, input, options)
	}
	config := codingagent.CreateAgentSessionOptions{CWD: dir, AgentDir: dir, Model: &model, ModelRuntime: modelRuntime, StreamFunction: stream, SessionManager: manager, SettingsManager: settings}
	created, err := codingagent.CreateAgentSession(ctx, config)
	if err != nil {
		return err
	}
	session = created.Session
	defer session.Dispose()
	_, err = session.Subscribe(func(e codingagent.AgentSessionEvent) {
		if _, ok := e.(codingagent.AgentSessionAutoRetryStartEvent); ok {
			retries++
		}
	})
	if err != nil {
		return err
	}
	if e := session.SendUserMessage(ai.UserText("实现并验证 M4 编排")); e != nil {
		return e
	}
	if err != nil {
		return err
	}
	pending, err := session.PendingMessageCount()
	if err != nil {
		return err
	}
	stats, err := session.GetSessionStats()
	if err != nil {
		return err
	}
	if retries != 1 || pending != 0 || stats.UserMessages != 3 {
		return fmt.Errorf("delivery/retry: retries=%d pending=%d stats=%+v", retries, pending, stats)
	}
	first := *manager.GetLeafID()
	model, _, err = modelRuntime.GetModel("deepseek", "deepseek-v4-pro")
	if err != nil {
		return err
	}
	if err = session.SetModel(model); err != nil {
		return err
	}
	if err = session.SetThinkingLevel("max"); err != nil {
		return err
	}
	if err = session.SetActiveToolsByName([]string{"read"}); err != nil {
		return err
	}
	checkConfig = true
	if err = session.Prompt(ctx, "使用新配置继续原分支并保留验证结论"); err != nil {
		return err
	}
	if !sawConfig {
		return fmt.Errorf("next request did not use the new model, thinking and tools")
	}
	original := *manager.GetLeafID()
	if _, err = session.NavigateTree(ctx, first); err != nil {
		return err
	}
	if err = session.Prompt(ctx, "探索另一个分支并记录发现"); err != nil {
		return err
	}
	branch := *manager.GetLeafID()
	summaryText = "M4 离开分支的探索结论"
	result, err := session.NavigateTree(ctx, original, codingagent.NavigateTreeOptions{Summarize: true, Label: "探索摘要"})
	if err != nil {
		return err
	}
	if result.Cancelled || result.SummaryEntry == nil || !strings.Contains(result.SummaryEntry.Summary, summaryText) {
		return fmt.Errorf("branch summary missing")
	}
	expectedSummary, summaryText, sawSummary = summaryText, "", false
	if err = session.Prompt(ctx, "结合探索摘要继续原任务"); err != nil {
		return err
	}
	if !sawSummary {
		return fmt.Errorf("branch continuation did not receive its summary")
	}
	summaryText = "M4 压缩检查点：保留任务目标和验证结论。"
	compacted, err := session.Compact(ctx, "保留 M4 工作流检查点")
	if err != nil {
		return err
	}
	if !strings.Contains(compacted.Summary, summaryText) {
		return fmt.Errorf("compaction summary missing")
	}
	expectedSummary, summaryText, sawSummary = summaryText, "", false
	if err = session.Prompt(ctx, "压缩后继续并检查摘要"); err != nil {
		return err
	}
	if !sawSummary {
		return fmt.Errorf("continuation did not receive the checkpoint")
	}
	stats, err = session.GetSessionStats()
	if err != nil {
		return err
	}
	before := session.Messages()
	file := *session.SessionFile()
	if err = session.Dispose(); err != nil {
		return err
	}
	reopened, err := codingagent.OpenSessionManager(file, nil, nil)
	if err != nil {
		return err
	}
	if reopened.GetEntry(branch) == nil || reopened.GetEntry(original) == nil {
		return fmt.Errorf("reopen lost original branches")
	}
	config.SessionManager, config.Model, config.ThinkingLevel = reopened, &model, "max"
	restored, err := codingagent.CreateAgentSession(ctx, config)
	if err != nil {
		return err
	}
	defer restored.Session.Dispose()
	after, err := restored.Session.GetSessionStats()
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(before, restored.Session.Messages()) || !reflect.DeepEqual(stats, after) {
		return fmt.Errorf("reopen changed context or stats")
	}
	if err = restored.Session.Prompt(ctx, "重开后继续"); err != nil {
		return err
	}
	fmt.Println("PASS: delivery, retry, configuration, compaction, branch summary, stats and reopen")
	return nil
}
