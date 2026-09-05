package codingagent_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func TestCreateAgentSessionReopensPersistedHistoryWithoutDuplicatingIt(t *testing.T) {
	dir := t.TempDir()
	cwd := filepath.Join(dir, "project")
	manager, err := codingagent.NewSessionManager(cwd, &dir, codingagent.NewSessionOptions{ID: "sdk-session"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := ai.FauxAssistantMessage(ai.FauxAssistantText("first reply"), ai.FauxAssistantMessageOptions{Timestamp: ai.Some[int64](2)})
	if err != nil {
		t.Fatal(err)
	}
	provider, err := ai.NewFauxProvider()
	if err != nil {
		t.Fatal(err)
	}
	provider.SetResponses([]ai.FauxResponseStep{first})
	model, _ := provider.GetModel()
	created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{
		CWD: cwd, Model: &model, Provider: provider.Provider, ThinkingLevel: ai.ModelThinkingLevelHigh, SessionManager: manager,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := created.Session.Prompt(context.Background(), "first prompt"); err != nil {
		t.Fatal(err)
	}
	path := *created.Session.SessionFile()
	if err := created.Session.Dispose(); err != nil {
		t.Fatal(err)
	}

	reopened, err := codingagent.OpenSessionManager(path, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ai.FauxAssistantMessage(ai.FauxAssistantText("second reply"), ai.FauxAssistantMessageOptions{Timestamp: ai.Some[int64](4)})
	if err != nil {
		t.Fatal(err)
	}
	provider, err = ai.NewFauxProvider()
	if err != nil {
		t.Fatal(err)
	}
	provider.SetResponses([]ai.FauxResponseStep{ai.FauxResponseFactory(func(input ai.Context, _ *ai.SimpleStreamOptions, _ *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
		roles := make([]ai.MessageRole, len(input.Messages))
		for i := range input.Messages {
			roles[i] = input.Messages[i].MessageRole()
		}
		if want := []ai.MessageRole{ai.MessageRoleUser, ai.MessageRoleAssistant, ai.MessageRoleUser}; !reflect.DeepEqual(roles, want) {
			t.Fatalf("reopened Provider roles = %v, want %v", roles, want)
		}
		return second, nil
	})})
	model, _ = provider.GetModel()
	created, err = codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{
		CWD: cwd, Model: &model, Provider: provider.Provider, SessionManager: reopened,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := created.Session.Prompt(context.Background(), "second prompt"); err != nil {
		t.Fatal(err)
	}
	if err := created.Session.Dispose(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	entries := codingagent.ParseSessionEntries(string(data))
	if len(entries) != 7 {
		t.Fatalf("persisted records = %d, want header + model/thinking + two rounds", len(entries))
	}
	roles := []ai.MessageRole{}
	for _, item := range entries {
		if item.Entry != nil && item.Entry.Type == "message" {
			roles = append(roles, item.Entry.Message.MessageRole())
		}
	}
	if want := []ai.MessageRole{ai.MessageRoleUser, ai.MessageRoleAssistant, ai.MessageRoleUser, ai.MessageRoleAssistant}; !reflect.DeepEqual(roles, want) {
		t.Fatalf("persisted roles = %v, want %v", roles, want)
	}
}

func TestNewSessionManagerUsesPigDefaultLayoutAndValidatesBeforeIO(t *testing.T) {
	agentDir := filepath.Join(t.TempDir(), "agent")
	t.Setenv("PIG_CODING_AGENT_DIR", agentDir)
	cwd := filepath.Join(t.TempDir(), "project")
	manager, err := codingagent.NewSessionManager(cwd, nil, codingagent.NewSessionOptions{ID: "custom-session"})
	if err != nil {
		t.Fatal(err)
	}
	if !manager.UsesDefaultSessionDir() || manager.GetSessionID() != "custom-session" {
		t.Fatalf("default manager = dir %q, id %q, default %t", manager.GetSessionDir(), manager.GetSessionID(), manager.UsesDefaultSessionDir())
	}
	relative, err := filepath.Rel(filepath.Join(agentDir, "sessions"), *manager.GetSessionFile())
	if err != nil || strings.HasPrefix(relative, "..") {
		t.Fatalf("default Session file = %q, want under Pig agent dir", *manager.GetSessionFile())
	}
	if fileExistsForIssue71(*manager.GetSessionFile()) {
		t.Fatal("new Session created a file before its first Assistant message")
	}

	invalidDir := filepath.Join(t.TempDir(), "invalid")
	if _, err := codingagent.NewSessionManager(cwd, &invalidDir, codingagent.NewSessionOptions{ID: "-invalid"}); err == nil {
		t.Fatal("invalid Session ID succeeded")
	}
	if _, err := os.Stat(invalidDir); !os.IsNotExist(err) {
		t.Fatalf("invalid Session ID created storage: %v", err)
	}
}

func TestPersistedAgentSessionKeepsProviderFailureAndReportsStorageFailure(t *testing.T) {
	t.Run("cancellation and cleanup", func(t *testing.T) {
		dir := t.TempDir()
		manager, err := codingagent.NewSessionManager(dir, &dir)
		if err != nil {
			t.Fatal(err)
		}
		rate := 100.0
		minTokenSize := 1
		response, err := ai.FauxAssistantMessage(ai.FauxAssistantText("partial response that must survive cancellation"))
		if err != nil {
			t.Fatal(err)
		}
		provider, err := ai.NewFauxProvider(ai.RegisterFauxProviderOptions{
			TokensPerSecond: &rate,
			TokenSize:       &ai.FauxTokenSize{Min: &minTokenSize},
		})
		if err != nil {
			t.Fatal(err)
		}
		provider.SetResponses([]ai.FauxResponseStep{response})
		model, _ := provider.GetModel()
		created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: dir, Model: &model, Provider: provider.Provider, SessionManager: manager})
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancelCause(context.Background())
		var once sync.Once
		prompt := "cancel"
		outcome, err := codingagent.RunHeadless(ctx, codingagent.NewAgentSessionRuntime(created.Session, codingagent.AgentSessionServices{CWD: dir}, nil, nil, nil), codingagent.HeadlessRunOptions{
			InitialMessage: &prompt,
			OnEvent: func(event codingagent.AgentSessionEvent) {
				update, ok := event.(codingagent.AgentSessionMessageUpdateEvent)
				if !ok {
					return
				}
				assistant, ok := update.Message.(ai.AssistantMessage)
				if ok && len(assistant.Content) != 0 {
					if text, ok := assistant.Content[0].(ai.TextContent); ok && text.Text != "" {
						once.Do(func() { cancel(context.Canceled) })
					}
				}
			},
		})
		if err != nil || !outcome.Canceled || outcome.FinalMessage == nil || outcome.FinalMessage.StopReason != ai.StopReasonAborted || outcome.Text[0] == "" {
			t.Fatalf("cancellation outcome = %#v, err = %v", outcome, err)
		}
		if !errors.Is(context.Cause(ctx), context.Canceled) {
			t.Fatalf("cancellation cause = %v", context.Cause(ctx))
		}
		if err := created.Session.Dispose(); err != nil {
			t.Fatal(err)
		}
		reopened, err := codingagent.OpenSessionManager(*manager.GetSessionFile(), nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		messages := reopened.BuildSessionContext().Messages
		assistant, ok := messages[len(messages)-1].(ai.AssistantMessage)
		if !ok || assistant.StopReason != ai.StopReasonAborted {
			t.Fatalf("persisted canceled Assistant = %#v", messages[len(messages)-1])
		}
	})

	t.Run("provider failure", func(t *testing.T) {
		dir := t.TempDir()
		manager, err := codingagent.NewSessionManager(dir, &dir)
		if err != nil {
			t.Fatal(err)
		}
		failure, err := ai.FauxAssistantMessage(ai.FauxAssistantText("partial"), ai.FauxAssistantMessageOptions{
			StopReason: ai.Some(ai.StopReasonError), ErrorMessage: ai.Some("provider failed"),
		})
		if err != nil {
			t.Fatal(err)
		}
		provider, err := ai.NewFauxProvider()
		if err != nil {
			t.Fatal(err)
		}
		provider.SetResponses([]ai.FauxResponseStep{failure})
		model, _ := provider.GetModel()
		created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: dir, Model: &model, Provider: provider.Provider, SessionManager: manager})
		if err != nil {
			t.Fatal(err)
		}
		prompt := "fail"
		outcome, err := codingagent.RunHeadless(context.Background(), codingagent.NewAgentSessionRuntime(created.Session, codingagent.AgentSessionServices{CWD: dir}, nil, nil, nil), codingagent.HeadlessRunOptions{InitialMessage: &prompt})
		if err != nil || outcome.FinalMessage == nil || outcome.FinalMessage.StopReason != ai.StopReasonError || outcome.Text[0] != "partial" {
			t.Fatalf("provider failure outcome = %#v, err = %v", outcome, err)
		}
		if !fileExistsForIssue71(*manager.GetSessionFile()) {
			t.Fatal("provider failure outcome was not persisted")
		}
		reopened, err := codingagent.OpenSessionManager(*manager.GetSessionFile(), nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		messages := reopened.BuildSessionContext().Messages
		assistant, ok := messages[len(messages)-1].(ai.AssistantMessage)
		if !ok || assistant.StopReason != ai.StopReasonError || assistant.Content[0].(ai.TextContent).Text != "partial" {
			t.Fatalf("persisted Provider failure = %#v", messages[len(messages)-1])
		}
		if err := created.Session.Dispose(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("storage failure", func(t *testing.T) {
		dir := t.TempDir()
		manager, err := codingagent.NewSessionManager(dir, &dir)
		if err != nil {
			t.Fatal(err)
		}
		reply, err := ai.FauxAssistantMessage(ai.FauxAssistantText("completed in Provider"))
		if err != nil {
			t.Fatal(err)
		}
		provider, err := ai.NewFauxProvider()
		if err != nil {
			t.Fatal(err)
		}
		provider.SetResponses([]ai.FauxResponseStep{reply})
		model, _ := provider.GetModel()
		created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: dir, Model: &model, Provider: provider.Provider, SessionManager: manager})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.RemoveAll(dir); err != nil {
			t.Fatal(err)
		}
		prompt := "persist this"
		outcome, err := codingagent.RunHeadless(context.Background(), codingagent.NewAgentSessionRuntime(created.Session, codingagent.AgentSessionServices{CWD: dir}, nil, nil, nil), codingagent.HeadlessRunOptions{InitialMessage: &prompt})
		if err == nil || !strings.Contains(err.Error(), "persist session") {
			t.Fatalf("storage error = %v", err)
		}
		if outcome.FinalMessage == nil || outcome.Text[0] != "completed in Provider" {
			t.Fatalf("storage failure lost Provider outcome: %#v", outcome)
		}
		if fileExistsForIssue71(*manager.GetSessionFile()) {
			t.Fatal("storage failure reported a session file")
		}
		messages := manager.BuildSessionContext().Messages
		if len(messages) != 2 || messages[1].MessageRole() != ai.MessageRoleAssistant {
			t.Fatalf("storage failure lost the in-memory Stream outcome: %#v", messages)
		}
		if err := created.Session.Dispose(); err != nil {
			t.Fatal(err)
		}
	})
}

func fileExistsForIssue71(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
