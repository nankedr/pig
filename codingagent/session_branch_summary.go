package codingagent

import (
	"context"
	"errors"
	"fmt"

	"github.com/nankedr/pig/agent"
)

func (s *AgentSession) summarizeNavigationLocked(ctx context.Context, target string, option NavigateTreeOptions) (result BranchSummaryResult, err error) {
	m := s.sessionManager
	old := cloneStringPointer(m.leafID)
	oldID := ""
	if old != nil {
		oldID = *old
	}
	snapshot := &SessionManager{entries: cloneSessionEntries(m.entries)}
	collected, err := CollectEntriesForBranchSummary(ctx, snapshot, oldID, target)
	if err != nil || len(collected.Entries) == 0 {
		return result, err
	}
	count := len(m.entries)
	run, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	s.branchSummaryCancel, s.branchSummaryDone = cancel, done
	m.mu.Unlock()
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		m.mu.Lock()
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) || run.Err() != nil {
			result = BranchSummaryResult{Aborted: true}
			err = ctx.Err()
		}
		if err == nil && !result.Aborted {
			if s.disposed {
				result = BranchSummaryResult{Aborted: true}
				return
			}
			if len(m.entries) != count || (m.leafID == nil) != (old == nil) || old != nil && *m.leafID != *old {
				err = fmt.Errorf("Session changed during branch summary")
			}
		}
	}()
	o := GenerateBranchSummaryOptions{Model: s.Model(), StreamFn: s.agent.StreamFunction(), CustomInstructions: option.CustomInstructions, ReplaceInstructions: option.ReplaceInstructions}
	if s.modelRuntime != nil && !s.runtimeStream {
		auth, e := s.modelRuntime.GetModelAuth(run, o.Model)
		if value, ok := auth.Value(); e == nil && ok {
			if key, ok := value.Auth.APIKey.Value(); ok {
				o.APIKey = &key
			}
			o.Env = value.Env
			o.Headers = map[string]string{}
			for k, v := range value.Auth.Headers {
				if v != nil {
					o.Headers[k] = *v
				}
			}
			if base, ok := value.Auth.BaseURL.Value(); ok {
				o.Model.BaseURL = base
			}
		}
	}
	settings, err := s.settingsManager.GetBranchSummarySettings()
	if err != nil {
		return result, err
	}
	retry, err := s.settingsManager.GetRetrySettings()
	if err != nil {
		return result, err
	}
	o.Retry = &agent.RetryPolicy{Enabled: *retry.Enabled, MaxRetries: *retry.MaxRetries, BaseDelayMS: *retry.BaseDelayMS}
	o.Callbacks = &agent.RetryCallbacks{
		OnRetryScheduled: func(attempt, maxAttempts int, delay int64, text string) error {
			s.emit(AgentSessionSummarizationRetryScheduledEvent{Type: AgentSessionEventTypeSummarizationRetryScheduled, Attempt: attempt, MaxAttempts: maxAttempts, DelayMS: delay, ErrorMessage: text})
			return nil
		},
		OnRetryAttemptStart: func() error {
			s.emit(AgentSessionBranchSummaryRetryAttemptStartEvent{Type: AgentSessionEventTypeSummarizationRetryAttemptStart, Source: SummarizationRetrySourceBranchSummary})
			return nil
		},
		OnRetryFinished: func(bool, int, *string) error {
			s.emit(AgentSessionSummarizationRetryFinishedEvent{Type: AgentSessionEventTypeSummarizationRetryFinished})
			return nil
		},
	}
	result, err = generateBranchSummary(run, collected.Entries, o, settings.ReserveTokens)
	if err == nil && run.Err() != nil {
		err = context.Cause(run)
	}
	if err == nil && result.Error != "" {
		err = fmt.Errorf("%s", result.Error)
	}
	return result, err
}
