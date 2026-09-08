package codingagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nankedr/pig/agent"
)

func (s *AgentSession) Compact(ctx context.Context, instructions ...string) (result CompactionResult, err error) {
	if ctx == nil {
		return result, fmt.Errorf("compaction context must not be nil")
	}
	if len(instructions) > 1 {
		return result, fmt.Errorf("expected at most one custom instruction")
	}
	s.mu.Lock()
	if s.disposed || s.compactionCancel != nil || s.configurationNotifying || s.queueDispatchDone != nil {
		s.mu.Unlock()
		return result, fmt.Errorf("AgentSession is busy or disposed")
	}
	if s.agent == nil || s.sessionManager == nil {
		s.mu.Unlock()
		return result, fmt.Errorf("AgentSession requires Agent and SessionManager")
	}
	run, cancel := context.WithCancel(ctx)
	s.compactionCancel = cancel
	done := make(chan struct{})
	s.compactionDone = done
	activeCancel := s.activeCancel
	s.mu.Unlock()
	defer cancel()
	if activeCancel != nil {
		activeCancel(context.Canceled)
	}
	s.agent.Abort()
	s.mu.RLock()
	idle := s.idle
	s.mu.RUnlock()
	<-idle
	s.mu.Lock()
	s.compactionCancel = cancel
	s.mu.Unlock()
	s.emit(AgentSessionCompactionStartEvent{Type: AgentSessionEventTypeCompactionStart, Reason: CompactionReasonManual})
	defer func() {
		s.mu.Lock()
		s.compactionCancel = nil
		s.compactionDone = nil
		close(done)
		s.mu.Unlock()
		event := AgentSessionCompactionEndEvent{Type: AgentSessionEventTypeCompactionEnd, Reason: CompactionReasonManual}
		if err != nil {
			result = CompactionResult{}
			event.Aborted = run.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
			if !event.Aborted {
				text := "Compaction failed: " + err.Error()
				event.ErrorMessage = &text
			}
		} else {
			value := cloneCompactionResult(result)
			event.Result = &value
		}
		s.emit(event)
	}()
	if err != nil {
		return result, err
	}
	if run.Err() != nil {
		return result, context.Cause(run)
	}
	model := s.Model()
	options := SummaryOptions{ThinkingLevel: s.ThinkingLevel(), StreamFn: s.agent.StreamFunction()}
	if len(instructions) > 0 {
		options.CustomInstructions = instructions[0]
	}
	if s.modelRuntime != nil && !s.runtimeStream {
		auth, e := s.modelRuntime.GetModelAuth(run, model)
		if value, ok := auth.Value(); e == nil && ok {
			if key, ok := value.Auth.APIKey.Value(); ok {
				options.APIKey = &key
			}
			options.Env = value.Env
			options.Headers = map[string]string{}
			for k, v := range value.Auth.Headers {
				if v != nil {
					options.Headers[k] = *v
				}
			}
			if base, ok := value.Auth.BaseURL.Value(); ok {
				model.BaseURL = base
			}
		}
	}
	entries := s.sessionManager.GetBranch()
	settings, e := s.settingsManager.GetCompactionSettings()
	if e != nil {
		return result, e
	}
	preparation := PrepareCompaction(entries, settings)
	if preparation == nil {
		if len(entries) > 0 && entries[len(entries)-1].Type == "compaction" {
			return result, fmt.Errorf("Already compacted")
		}
		return result, fmt.Errorf("Nothing to compact (session too small)")
	}
	retry, e := s.settingsManager.GetRetrySettings()
	if e != nil {
		return result, e
	}
	options.Retry = &agent.RetryPolicy{Enabled: *retry.Enabled, MaxRetries: *retry.MaxRetries, BaseDelayMS: *retry.BaseDelayMS}
	options.Callbacks = &agent.RetryCallbacks{
		OnRetryScheduled: func(attempt, maxAttempts int, delay int64, text string) error {
			s.emit(AgentSessionSummarizationRetryScheduledEvent{Type: AgentSessionEventTypeSummarizationRetryScheduled, Attempt: attempt, MaxAttempts: maxAttempts, DelayMS: delay, ErrorMessage: text})
			return nil
		},
		OnRetryAttemptStart: func() error {
			s.emit(AgentSessionCompactionRetryAttemptStartEvent{Type: AgentSessionEventTypeSummarizationRetryAttemptStart, Source: SummarizationRetrySourceCompaction, Reason: CompactionReasonManual})
			return nil
		},
		OnRetryFinished: func(bool, int, *string) error {
			s.emit(AgentSessionSummarizationRetryFinishedEvent{Type: AgentSessionEventTypeSummarizationRetryFinished})
			return nil
		},
	}
	result, err = Compact(run, preparation, model, options)
	if err != nil {
		return result, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if run.Err() != nil {
		return CompactionResult{}, context.Cause(run)
	}
	if s.disposed {
		return CompactionResult{}, context.Canceled
	}
	manager := s.sessionManager
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.leafID == nil || *manager.leafID != entries[len(entries)-1].ID {
		return CompactionResult{}, fmt.Errorf("Session changed during compaction")
	}
	details, err := json.Marshal(result.Details)
	if err != nil {
		return CompactionResult{}, err
	}
	fromHook := false
	_, err = manager.appendCompactionLocked(run, result.Summary, result.FirstKeptEntryID, result.TokensBefore, AppendCompactionOptions{Details: details, Usage: result.Usage, FromHook: &fromHook})
	if err != nil {
		return CompactionResult{}, err
	}
	messages := BuildSessionContext(manager.entries, manager.leafID).Messages
	if err = s.agent.ReplaceMessages(messages); err != nil {
		return CompactionResult{}, err
	}
	var tokens int64
	for _, m := range messages {
		tokens += EstimateTokens(m)
	}
	result.EstimatedTokensAfter = &tokens
	return result, nil
}

func cloneCompactionResult(result CompactionResult) CompactionResult {
	data, _ := json.Marshal(result.Details)
	result.Details = nil
	_ = json.Unmarshal(data, &result.Details)
	if result.Usage != nil {
		value := *result.Usage
		result.Usage = &value
	}
	if result.EstimatedTokensAfter != nil {
		value := *result.EstimatedTokensAfter
		result.EstimatedTokensAfter = &value
	}
	return result
}

func (m *SessionManager) AppendCompaction(summary, firstKept string, tokens int64, options ...AppendCompactionOptions) (string, error) {
	if len(options) > 1 {
		return "", fmt.Errorf("expected at most one AppendCompactionOptions")
	}
	var o AppendCompactionOptions
	if len(options) > 0 {
		o = options[0]
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.appendCompactionLocked(context.Background(), summary, firstKept, tokens, o)
}
func (m *SessionManager) appendCompactionLocked(ctx context.Context, summary, firstKept string, tokens int64, o AppendCompactionOptions) (string, error) {
	entry := m.newEntryLocked("compaction")
	entry.Summary = summary
	entry.FirstKeptEntryID = firstKept
	entry.TokensBefore = tokens
	entry.Details = o.Details
	entry.Usage = o.Usage
	entry.FromHook = o.FromHook
	if len(o.Details) > 0 && !json.Valid(o.Details) {
		return "", fmt.Errorf("invalid compaction details")
	}
	entries := append(cloneSessionEntries(m.entries), cloneSessionEntry(entry))
	if m.sessionFile != "" {
		data, err := encodeSessionFile(m.header, entries)
		if err != nil {
			return "", err
		}
		file, err := os.CreateTemp(filepath.Dir(m.sessionFile), ".session-compaction-*")
		if err != nil {
			return "", err
		}
		defer os.Remove(file.Name())
		_, writeErr := file.Write(data)
		closeErr := file.Close()
		if err = errors.Join(writeErr, closeErr); err != nil {
			return "", err
		}
		if ctx.Err() != nil {
			return "", context.Cause(ctx)
		}
		if err = os.Rename(file.Name(), m.sessionFile); err != nil {
			return "", err
		}
		m.flushed = true
	}
	m.entries = entries
	id := entry.ID
	m.leafID = &id
	return id, nil
}
