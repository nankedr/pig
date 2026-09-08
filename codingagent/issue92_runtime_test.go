package codingagent_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func TestBranchSummaryCancellationAndExclusion(t *testing.T) {
	for _, mode := range []string{"branch", "abort", "context", "dispose"} {
		t.Run(mode, func(t *testing.T) {
			entered := make(chan struct{})
			settle := make(chan struct{})
			s, m := compact89Session(t, func(ctx context.Context, _ ai.Model, _ ai.Context, _ ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
				stream := ai.NewAssistantMessageEventStream()
				close(entered)
				go func() {
					<-ctx.Done()
					<-settle
					stream.End(ai.AssistantMessage{Role: ai.MessageRoleAssistant, StopReason: ai.StopReasonAborted})
				}()
				return stream
			}, true)
			target := m.GetEntries()[0].ID
			before := tree91Snapshot(t, s)
			file := *m.GetSessionFile()
			data, _ := os.ReadFile(file)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			done := make(chan struct{})
			var result codingagent.NavigateTreeResult
			var err error
			go func() {
				defer close(done)
				result, err = s.NavigateTree(ctx, target, codingagent.NavigateTreeOptions{Summarize: true})
			}()
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("summary did not start")
			}
			compacting, e := s.IsCompacting()
			if e != nil || !compacting {
				t.Fatal("summary not included in IsCompacting", e)
			}
			if e := s.Prompt(context.Background(), "overlap"); e == nil {
				t.Fatal("Prompt admitted during summary")
			}
			if e := s.SetThinkingLevel("off"); e == nil {
				t.Fatal("configuration admitted during summary")
			}
			if _, e := s.NavigateTree(context.Background(), target); e == nil {
				t.Fatal("navigation admitted during summary")
			}
			if _, e := s.Compact(context.Background()); e == nil {
				t.Fatal("compaction admitted during summary")
			}
			wait, stop := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer stop()
			if e := s.WaitForIdle(wait); !errors.Is(e, context.DeadlineExceeded) {
				t.Fatal("WaitForIdle returned during summary", e)
			}
			if e := s.AbortCompaction(); e != nil {
				t.Fatal(e)
			}
			select {
			case <-done:
				t.Fatal("AbortCompaction cancelled branch summary")
			default:
			}
			switch mode {
			case "branch":
				_ = s.AbortBranchSummary()
			case "abort":
				_ = s.Abort()
			case "context":
				cancel()
			case "dispose":
				_ = s.Dispose()
			}
			close(settle)
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("summary did not cancel")
			}
			if !result.Cancelled || !result.Aborted || result.SummaryEntry != nil {
				t.Fatalf("cancel result %+v", result)
			}
			if mode == "context" {
				if !errors.Is(err, context.Canceled) {
					t.Fatal(err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if tree91Snapshot(t, s) != before {
				t.Fatal("cancellation changed original branch")
			}
			after, _ := os.ReadFile(file)
			if string(after) != string(data) {
				t.Fatal("cancellation changed persisted branch")
			}
			if e := s.WaitForIdle(context.Background()); e != nil {
				t.Fatal(e)
			}
			compacting, _ = s.IsCompacting()
			if compacting {
				t.Fatal("summary remained busy")
			}
		})
	}
}

func TestBranchSummaryWriteFailureAndOwnership(t *testing.T) {
	s, core, target := tree91Session(t)
	response, _ := ai.FauxAssistantMessage(ai.FauxAssistantText("checkpoint"))
	core.SetResponses([]ai.FauxResponseStep{response, response})
	m := s.SessionManager()
	before := tree91Snapshot(t, s)
	file := *s.SessionFile()
	if err := os.Rename(file, file+".saved"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(file, 0700); err != nil {
		t.Fatal(err)
	}
	r, err := s.NavigateTree(context.Background(), target, codingagent.NavigateTreeOptions{Summarize: true, Label: "summary"})
	if err == nil || r.SummaryEntry != nil || tree91Snapshot(t, s) != before {
		t.Fatal("failed commit partially navigated", r, err)
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(file+".saved", file); err != nil {
		t.Fatal(err)
	}
	r, err = s.NavigateTree(context.Background(), target, codingagent.NavigateTreeOptions{Summarize: true, Label: "summary"})
	if err != nil || r.SummaryEntry == nil {
		t.Fatal(err)
	}
	id := r.SummaryEntry.ID
	owned := m.GetEntry(id)
	r.SummaryEntry.Summary = "changed"
	r.SummaryEntry.Details[0] = '!'
	*r.SummaryEntry.FromHook = true
	if !reflect.DeepEqual(owned, m.GetEntry(id)) {
		t.Fatal("caller mutated persisted summary")
	}
	if label := m.GetLabel(id); label == nil || *label != "summary" {
		t.Fatal("summary label missing")
	}
}

func TestBranchSummaryRuntimeAuthAndReopen(t *testing.T) {
	runtime, _, store := config87Runtime(t)
	requests := []map[string]any{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		body["key"] = r.Header.Get("Authorization")
		requests = append(requests, body)
		w.Header().Set("Content-Type", "text/event-stream")
		if len(requests) == 1 {
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n")
			return
		}
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"checkpoint\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	model, _, _ := runtime.GetModel("deepseek", "deepseek-v4-flash")
	model.BaseURL = server.URL
	_, manager := compact89Session(t, nil, true)
	settings, _ := codingagent.NewInMemorySettingsManager(codingagent.Settings{Retry: &codingagent.RetrySettings{Enabled: pointerTo(true), MaxRetries: pointerTo(1), BaseDelayMS: pointerTo(int64(1))}})
	created, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: manager.GetCWD(), AgentDir: t.TempDir(), Model: &model, ModelRuntime: runtime, SessionManager: manager, SettingsManager: settings, Tools: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	defer created.Session.Dispose()
	credential76Set(t, store, "deepseek", "summary-key")
	result, err := created.Session.NavigateTree(context.Background(), manager.GetEntries()[0].ID, codingagent.NavigateTreeOptions{Summarize: true})
	if err != nil || result.SummaryEntry == nil {
		t.Fatal(err)
	}
	if len(requests) != 2 || requests[0]["key"] != "Bearer summary-key" || requests[1]["key"] != "Bearer summary-key" {
		t.Fatalf("auth or truncated SSE retry: %+v", requests)
	}
	reopened, err := codingagent.OpenSessionManager(*manager.GetSessionFile(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	credential76Set(t, store, "deepseek", "continue-key")
	restored, err := codingagent.CreateAgentSession(context.Background(), codingagent.CreateAgentSessionOptions{CWD: manager.GetCWD(), Model: &model, ModelRuntime: runtime, SessionManager: reopened, SettingsManager: settings, Tools: []string{}})
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Session.Dispose()
	if err := restored.Session.Prompt(context.Background(), "continue"); err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(requests[2]["messages"])
	if requests[2]["key"] != "Bearer continue-key" || !strings.Contains(string(data), "Summary of that exploration:") || strings.Contains(string(data), "partial") {
		t.Fatalf("continuation missing committed summary: %s", data)
	}
}

func TestBranchSummaryWaitForIdleIncludesPersistence(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	summary := strings.Repeat("context ", 512*1024)
	s, m := compact89Session(t, func(context.Context, ai.Model, ai.Context, ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
		stream := ai.NewAssistantMessageEventStream()
		close(entered)
		go func() {
			<-release
			message, _ := ai.FauxAssistantMessage(ai.FauxAssistantText(summary))
			stream.End(message)
		}()
		return stream
	}, true)
	done := make(chan error, 1)
	go func() {
		_, err := s.NavigateTree(context.Background(), m.GetEntries()[0].ID, codingagent.NavigateTreeOptions{Summarize: true})
		done <- err
	}()
	<-entered
	path := *s.SessionFile()
	time.AfterFunc(20*time.Millisecond, func() { close(release) })
	if err := s.WaitForIdle(context.Background()); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	persisted := strings.Contains(string(data), `"type":"branch_summary"`)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !persisted {
		t.Fatal("WaitForIdle returned before branch_summary persistence")
	}
}
