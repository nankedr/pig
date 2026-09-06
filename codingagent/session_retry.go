package codingagent

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"time"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
)

func (s *AgentSession) willRetry(messages []agent.AgentMessage) bool {
	settings, err := s.settingsManager.GetRetrySettings()
	if err != nil || !*settings.Enabled || s.RetryAttempt() >= *settings.MaxRetries {
		return false
	}
	for i := len(messages) - 1; i >= 0; i-- {
		if message, ok := messages[i].(ai.AssistantMessage); ok {
			return retryableSessionError(message, s.Model().ContextWindow)
		}
	}
	return false
}

func (s *AgentSession) endRetry(success bool, finalError *string) {
	attempt := s.RetryAttempt()
	if attempt > 0 {
		s.emit(AgentSessionAutoRetryEndEvent{Type: AgentSessionEventTypeAutoRetryEnd, Success: success, Attempt: attempt, FinalError: finalError})
	}
	s.mu.Lock()
	s.retryAttempt = 0
	s.mu.Unlock()
}

func (s *AgentSession) prepareRetry(ctx context.Context) (bool, error) {
	s.mu.RLock()
	message := s.lastAssistant
	s.mu.RUnlock()
	if message == nil {
		return false, nil
	}
	settings, err := s.settingsManager.GetRetrySettings()
	if err != nil {
		return false, err
	}
	if !retryableSessionError(*message, s.Model().ContextWindow) || !*settings.Enabled || s.RetryAttempt() >= *settings.MaxRetries {
		if message.StopReason == ai.StopReasonError {
			text, _ := message.ErrorMessage.Value()
			s.endRetry(false, &text)
		}
		return false, nil
	}
	attempt := s.RetryAttempt() + 1
	base := *settings.BaseDelayMS
	if base != 0 && (attempt > 64 || base > math.MaxInt64>>(attempt-1) || base < math.MinInt64>>(attempt-1)) {
		return false, fmt.Errorf("retry delay exceeds int64 milliseconds")
	}
	delay := base << (attempt - 1)
	wait, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.retryAttempt = attempt
	s.retryCancel = cancel
	s.mu.Unlock()
	defer func() { cancel(); s.mu.Lock(); s.retryCancel = nil; s.mu.Unlock() }()
	text, _ := message.ErrorMessage.Value()
	s.emit(AgentSessionAutoRetryStartEvent{Type: AgentSessionEventTypeAutoRetryStart, Attempt: attempt, MaxAttempts: *settings.MaxRetries, DelayMS: delay, ErrorMessage: text})
	messages := s.Messages()
	if len(messages) > 0 && messages[len(messages)-1].MessageRole() == ai.MessageRoleAssistant {
		if err := s.agent.ReplaceMessages(messages[:len(messages)-1]); err != nil {
			return false, err
		}
	}
	waitMS := delay
	// Node setTimeout clamps out-of-range delays to 1ms.
	if waitMS < 1 || waitMS > math.MaxInt32 {
		waitMS = 1
	}
	timer := time.NewTimer(time.Duration(waitMS) * time.Millisecond)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-wait.Done():
	}
	s.mu.Lock()
	cancelled := wait.Err() != nil
	if cancelled {
		s.retryCancelled = true
		s.retryAttempt = 0
	} else {
		s.retryCancel = nil
	}
	s.mu.Unlock()
	if cancelled {
		text := "Retry cancelled"
		s.emit(AgentSessionAutoRetryEndEvent{Type: AgentSessionEventTypeAutoRetryEnd, Success: false, Attempt: attempt, FinalError: &text})
		return false, context.Cause(ctx)
	}
	return true, nil
}

var sessionProviderLimitError = regexp.MustCompile(`(?i)GoUsageLimitError|FreeUsageLimitError|Monthly usage limit reached|available balance|insufficient_quota|out of budget|quota exceeded|billing`)
var sessionTransientError = regexp.MustCompile(`(?i)overloaded|rate.?limit|too many requests|429|500|502|503|504|524|service.?unavailable|server.?error|internal.?error|provider.?returned.?error|exceeded request buffer limit while retrying upstream|network.?error|connection.?error|connection.?refused|connection.?lost|other side closed|fetch failed|getaddrinfo|ENOTFOUND|EAI_AGAIN|upstream.?connect|reset before headers|socket hang up|socket connection was closed|timed? out|timeout|terminated|websocket.?closed|websocket.?error|ended without|stream ended before message_stop|stream ended before a terminal response event|http2 request did not get a response|retry delay|you can retry your request|try your request again|please retry your request|ResourceExhausted`)

func retryableSessionError(message ai.AssistantMessage, contextWindow int64) bool {
	text, _ := message.ErrorMessage.Value()
	return message.StopReason == ai.StopReasonError && !sessionProviderLimitError.MatchString(text) && sessionTransientError.MatchString(text) && !ai.IsContextOverflow(message, contextWindow)
}
