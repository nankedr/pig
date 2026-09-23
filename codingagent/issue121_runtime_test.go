package codingagent_test

import (
	"context"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestInteractionSettingsQueueDelivery121(t *testing.T) {
	for _, kind := range []string{"steering-mode", "follow-up-mode"} {
		for _, mode := range []string{"all", "one-at-a-time"} {
			t.Run(kind+"/"+mode, func(t *testing.T) {
				core, _ := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
				model, _ := core.GetModel()
				reply, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("reply"))
				core.SetResponses([]ai.FauxResponseStep{reply, reply, reply})
				started, release := make(chan struct{}), make(chan struct{})
				var first sync.Once
				var single, both bool
				created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), Model: &model, NoTools: codingagent.NoToolsAll, StreamFunction: func(ctx context.Context, m ai.Model, input ai.Context, opts ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
					first.Do(func() {
						close(started)
						select {
						case <-release:
						case <-ctx.Done():
						}
					})
					one, two := false, false
					for _, message := range input.Messages {
						if user, ok := message.(ai.UserMessage); ok {
							text := message85Text(user)
							one = one || text == "queued-one"
							two = two || text == "queued-two"
						}
					}
					single = single || one && !two
					both = both || one && two
					return core.StreamSimple(ctx, m, input, opts)
				}})
				if err != nil {
					t.Fatal(err)
				}
				s := created.Session
				defer s.Dispose()
				if err = s.UpdateInteractionSetting(kind, mode); err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				done := make(chan error, 1)
				go func() { done <- s.Prompt(ctx, "start") }()
				select {
				case <-started:
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
				for _, text := range []string{"queued-one", "queued-two"} {
					if kind == "steering-mode" {
						err = s.Steer(text)
					} else {
						err = s.FollowUp(text)
					}
					if err != nil {
						t.Fatal(err)
					}
				}
				close(release)
				if err = <-done; err != nil {
					t.Fatal(err)
				}
				if !both || single != (mode == "one-at-a-time") {
					t.Fatalf("observed both=%t separate request=%t for mode=%s", both, single, mode)
				}
			})
		}
	}
}
func TestInteractionSettingsCompactionExecution121(t *testing.T) {
	for _, enabled := range []string{"false", "true"} {
		t.Run(enabled, func(t *testing.T) {
			summaries, generations := 0, 0
			s := auto90Session(t, func(_ context.Context, m ai.Model, input ai.Context, _ ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
				system, _ := input.SystemPrompt.Value()
				if strings.HasPrefix(system, "You are a context summarization assistant.") {
					summaries++
					return auto90Reply(m, "checkpoint", auto90Response{})
				}
				generations++
				if generations == 1 {
					return auto90Reply(m, "", auto90Response{Input: pointerTo(int64(0)), Error: "prompt is too long"})
				}
				return auto90Reply(m, "recovered", auto90Response{})
			}, nil)
			if err := s.UpdateInteractionSetting("autocompact", enabled); err != nil {
				t.Fatal(err)
			}
			_ = s.Prompt(context.Background(), "go")
			if enabled == "true" && (summaries != 1 || generations != 2) || enabled == "false" && (summaries != 0 || generations != 1) {
				t.Fatalf("summaries=%d generations=%d", summaries, generations)
			}
		})
	}
}
func TestInteractionSettingsDefaultTrustExecution121(t *testing.T) {
	for _, choice := range []string{"Always trust", "Never trust"} {
		t.Run(choice, func(t *testing.T) {
			dir := t.TempDir()
			source := settingsSession121(t, t.TempDir(), dir, nil)
			if err := source.UpdateInteractionSetting("default-project-trust", choice); err != nil {
				t.Fatal(err)
			}
			cwd := t.TempDir()
			if err := os.Mkdir(filepath.Join(cwd, ".pig"), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(cwd, ".pig/settings.json"), []byte(`{"editorPaddingX":3}`), 0600); err != nil {
				t.Fatal(err)
			}
			settings, err := codingagent.NewSettingsManager(cwd, &dir)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err = codingagent.PrepareProjectSettings(ctx, codingagent.PrepareProjectSettingsOptions{CWD: cwd, AgentDir: dir, SettingsManager: settings}); err != nil {
				t.Fatal(err)
			}
			trusted, _ := settings.IsProjectTrusted()
			padding, _ := settings.GetEditorPaddingX()
			if trusted != (choice == "Always trust") || trusted && padding != 3 || !trusted && padding != 0 {
				t.Fatalf("trust=%t project padding=%d", trusted, padding)
			}
		})
	}
}
