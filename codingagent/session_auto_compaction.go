package codingagent

import (
	"context"
	"errors"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
)

func (s *AgentSession) checkAutoCompaction(ctx context.Context, message ai.AssistantMessage, skipAborted bool) bool {
	settings, err := s.settingsManager.GetCompactionSettings()
	if err != nil || !settings.Enabled || ctx.Err() != nil || skipAborted && message.StopReason == ai.StopReasonAborted {
		return false
	}
	model := s.Model()
	branch := s.sessionManager.GetBranch()
	var boundary int64
	for i := len(branch) - 1; i >= 0; i-- {
		if branch[i].Type == "compaction" {
			boundary = sessionTimestampMillis(branch[i].Timestamp)
			break
		}
	}
	if boundary != 0 && message.Timestamp <= boundary {
		return false
	}
	if message.Provider == model.Provider && message.Model == model.ID && (ai.IsContextOverflow(message, model.ContextWindow) || ai.IsRecoverableLength(message, model.MaxTokens)) {
		willRetry := message.StopReason != ai.StopReasonStop
		if willRetry {
			s.mu.Lock()
			attempted := s.overflowRecoveryAttempted
			s.overflowRecoveryAttempted = true
			s.compactionOutcome = true
			s.mu.Unlock()
			if attempted {
				text := "Context overflow recovery failed after one compact-and-retry attempt. Try reducing context or switching to a larger-context model."
				s.emit(AgentSessionCompactionEndEvent{Type: AgentSessionEventTypeCompactionEnd, Reason: CompactionReasonOverflow, ErrorMessage: &text})
				return false
			}
			messages := s.Messages()
			if len(messages) > 0 && messages[len(messages)-1].MessageRole() == ai.MessageRoleAssistant {
				if err := s.agent.ReplaceMessages(messages[:len(messages)-1]); err != nil {
					return false
				}
			}
		}
		return s.runAutoCompaction(ctx, CompactionReasonOverflow, willRetry, settings)
	}
	tokens := CalculateContextTokens(message.Usage)
	if message.StopReason == ai.StopReasonError || tokens == 0 {
		messages := s.Messages()
		estimate := agent.EstimateContextTokens(messages)
		if estimate.LastUsageIndex < 0 {
			return false
		}
		source, ok := sessionAssistantMessage(messages[estimate.LastUsageIndex])
		if ok && boundary != 0 && source.Timestamp <= boundary {
			return false
		}
		tokens = estimate.Tokens
	}
	if ShouldCompact(tokens, model.ContextWindow, settings) {
		return s.runAutoCompaction(ctx, CompactionReasonThreshold, false, settings)
	}
	return false
}

func (s *AgentSession) runAutoCompaction(ctx context.Context, reason CompactionReason, willRetry bool, settings CompactionSettings) bool {
	entries := s.sessionManager.GetBranch()
	preparation := PrepareCompaction(entries, settings)
	if preparation == nil {
		return false
	}
	run, cancel := context.WithCancel(ctx)
	defer cancel()
	s.mu.Lock()
	if s.disposed || s.compactionCancel != nil || run.Err() != nil {
		s.mu.Unlock()
		return false
	}
	s.compactionCancel = cancel
	s.autoCompacting = true
	s.mu.Unlock()
	s.emit(AgentSessionCompactionStartEvent{Type: AgentSessionEventTypeCompactionStart, Reason: reason})
	result, err := s.compactPrepared(run, entries, preparation, reason, "")
	event := AgentSessionCompactionEndEvent{Type: AgentSessionEventTypeCompactionEnd, Reason: reason}
	if err != nil {
		event.Aborted = run.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
		if event.Aborted {
			s.mu.Lock()
			s.retryCancelled = true
			s.mu.Unlock()
		}
		if !event.Aborted {
			text := "Auto-compaction failed: " + err.Error()
			if reason == CompactionReasonOverflow {
				text = "Context overflow recovery failed: " + err.Error()
			}
			event.ErrorMessage = &text
		}
	} else {
		event.Result = &result
		event.WillRetry = willRetry
	}
	s.emit(event)
	s.mu.Lock()
	s.compactionCancel = nil
	s.autoCompacting = false
	s.mu.Unlock()
	if err != nil {
		return false
	}
	if willRetry {
		messages := s.Messages()
		if len(messages) > 0 {
			if last, ok := sessionAssistantMessage(messages[len(messages)-1]); ok && (last.StopReason == ai.StopReasonError || last.StopReason == ai.StopReasonLength) {
				if err := s.agent.ReplaceMessages(messages[:len(messages)-1]); err != nil {
					return false
				}
			}
		}
	}
	return willRetry || s.agent.HasQueuedMessages() || s.hasCompactionQueue()
}

func (s *AgentSession) continueAfterCompaction(ctx context.Context) error {
	s.mu.Lock()
	messages := s.agent.State().Messages
	var prompts []agent.AgentMessage
	if len(messages) > 0 && messages[len(messages)-1].MessageRole() == ai.MessageRoleAssistant && !s.agent.HasQueuedMessages() {
		queue, mode := &s.compactionSteering, s.agent.SteeringMode()
		if len(*queue) == 0 {
			queue, mode = &s.compactionFollowUp, s.agent.FollowUpMode()
		}
		count := len(*queue)
		if mode == agent.QueueOneAtATime && count > 0 {
			count = 1
		}
		prompts = append(prompts, (*queue)[:count]...)
		*queue = (*queue)[count:]
		s.delayCompactionSteering = len(prompts) > 0
	}
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.delayCompactionSteering = false; s.mu.Unlock() }()
	if len(prompts) > 0 {
		return s.agent.Prompt(ctx, prompts...)
	}
	return s.agent.Continue(ctx)
}

func (s *AgentSession) flushCompactionQueues(assistantStarted bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.delayCompactionSteering || assistantStarted {
		for len(s.compactionSteering) > 0 {
			if err := s.agent.Steer(s.compactionSteering[0]); err != nil {
				return err
			}
			s.compactionSteering = s.compactionSteering[1:]
		}
	}
	for len(s.compactionFollowUp) > 0 {
		if err := s.agent.FollowUp(s.compactionFollowUp[0]); err != nil {
			return err
		}
		s.compactionFollowUp = s.compactionFollowUp[1:]
	}
	return nil
}

func (s *AgentSession) hasCompactionQueue() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.compactionSteering)+len(s.compactionFollowUp) > 0
}
