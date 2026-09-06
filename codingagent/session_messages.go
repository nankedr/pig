package codingagent

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
)

type sessionQueuedMessage struct {
	text      string
	timestamp int64
}

func (s *AgentSession) SendUserMessage(content ai.UserMessageContent, options ...SendUserMessageOptions) error {
	var delivery UserMessageDelivery
	if len(options) > 0 {
		delivery = options[0].DeliverAs
	}
	if delivery != "" && delivery != UserMessageDeliverySteer && delivery != UserMessageDeliveryFollowUp {
		return fmt.Errorf("invalid user message delivery: %q", delivery)
	}
	text, compact := content.Text()
	if !compact {
		blocks, _ := content.Blocks()
		parts := make([]string, 0, len(blocks))
		for _, block := range blocks {
			switch block := block.(type) {
			case ai.TextContent:
				parts = append(parts, block.Text)
			case *ai.TextContent:
				if block == nil {
					return fmt.Errorf("nil user text block")
				}
				parts = append(parts, block.Text)
			default:
				return notImplemented("AgentSession.SendUserMessage.Images")
			}
		}
		text = strings.Join(parts, "\n")
	}
	return s.prompt(context.Background(), text, PromptOptions{StreamingBehavior: string(delivery)})
}

func (s *AgentSession) Steer(text string) error {
	return s.queueMessage(text, UserMessageDeliverySteer)
}
func (s *AgentSession) FollowUp(text string) error {
	return s.queueMessage(text, UserMessageDeliveryFollowUp)
}

func (s *AgentSession) queueMessage(text string, delivery UserMessageDelivery) error {
	s.mu.Lock()
	err := s.queueMessageLocked(text, delivery)
	s.mu.Unlock()
	s.dispatchQueueEvents()
	return err
}

func (s *AgentSession) queueMessageLocked(text string, delivery UserMessageDelivery) error {
	if s.disposed {
		return fmt.Errorf("AgentSession is disposed")
	}
	if s.agent == nil {
		return fmt.Errorf("AgentSession has no Agent")
	}
	if delivery == "" {
		return fmt.Errorf("AgentSession is already processing; specify steer or followUp delivery")
	}
	message := s.userMessageLocked(text)
	var err error
	if delivery == UserMessageDeliverySteer {
		err = s.agent.Steer(message)
	} else {
		err = s.agent.FollowUp(message)
	}
	if err != nil {
		return err
	}
	pending := sessionQueuedMessage{text: text, timestamp: message.Timestamp}
	if delivery == UserMessageDeliverySteer {
		s.steeringMessages = append(s.steeringMessages, pending)
	} else {
		s.followUpMessages = append(s.followUpMessages, pending)
	}
	s.queueUpdateLocked()
	return nil
}

func (s *AgentSession) userMessageLocked(text string) ai.UserMessage {
	// Distinguish identical queued text and a new prompt within the same millisecond.
	s.lastMessageTimestamp = max(time.Now().UnixMilli(), s.lastMessageTimestamp+1)
	return ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserBlocks(ai.TextContent{Type: ai.ContentTypeText, Text: text}), Timestamp: s.lastMessageTimestamp}
}

func (s *AgentSession) consumeQueuedMessage(message agent.AgentMessage) {
	user, ok := message.(ai.UserMessage)
	if !ok {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, queue := range []*[]sessionQueuedMessage{&s.steeringMessages, &s.followUpMessages} {
		for i, queued := range *queue {
			if queued.timestamp == user.Timestamp && queued.text == sessionUserText(user) {
				*queue = slices.Delete(*queue, i, i+1)
				s.queueUpdateLocked()
				return
			}
		}
	}
}

func queuedSessionTexts(queue []sessionQueuedMessage) []string {
	texts := make([]string, len(queue))
	for i, message := range queue {
		texts[i] = message.text
	}
	return texts
}
func (s *AgentSession) GetSteeringMessages() ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return queuedSessionTexts(s.steeringMessages), nil
}
func (s *AgentSession) GetFollowUpMessages() ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return queuedSessionTexts(s.followUpMessages), nil
}
func (s *AgentSession) PendingMessageCount() (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.steeringMessages) + len(s.followUpMessages), nil
}
func (s *AgentSession) ClearQueue() error {
	s.mu.Lock()
	if s.disposed {
		s.mu.Unlock()
		return fmt.Errorf("AgentSession is disposed")
	}
	if s.agent == nil {
		s.mu.Unlock()
		return fmt.Errorf("AgentSession has no Agent")
	}
	s.agent.ClearAllQueues()
	s.steeringMessages, s.followUpMessages = nil, nil
	s.queueUpdateLocked()
	s.mu.Unlock()
	s.dispatchQueueEvents()
	return nil
}

func (s *AgentSession) SetSteeringMode(mode agent.QueueMode) error { return s.setQueueMode(mode, true) }
func (s *AgentSession) SetFollowUpMode(mode agent.QueueMode) error {
	return s.setQueueMode(mode, false)
}
func (s *AgentSession) setQueueMode(mode agent.QueueMode, steering bool) error {
	if mode != agent.QueueAll && mode != agent.QueueOneAtATime {
		return fmt.Errorf("invalid queue mode: %q", mode)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.disposed {
		return fmt.Errorf("AgentSession is disposed")
	}
	if s.agent == nil {
		return fmt.Errorf("AgentSession has no Agent")
	}
	if s.settingsManager != nil {
		var err error
		if steering {
			err = s.settingsManager.SetSteeringMode(mode)
		} else {
			err = s.settingsManager.SetFollowUpMode(mode)
		}
		if err != nil {
			return err
		}
	}
	if steering {
		return s.agent.SetSteeringMode(mode)
	}
	return s.agent.SetFollowUpMode(mode)
}

func (s *AgentSession) queueUpdateLocked() {
	s.queueEvents = append(s.queueEvents, AgentSessionQueueUpdateEvent{Type: AgentSessionEventTypeQueueUpdate, Steering: queuedSessionTexts(s.steeringMessages), FollowUp: queuedSessionTexts(s.followUpMessages)})
}

// Queue callbacks may query, enqueue, clear or abort without holding session locks.
func (s *AgentSession) dispatchQueueEvents() {
	s.mu.Lock()
	if s.queueDispatchDone != nil || len(s.queueEvents) == 0 {
		s.mu.Unlock()
		return
	}
	s.queueDispatchDone = make(chan struct{})
	for len(s.queueEvents) > 0 {
		event := s.queueEvents[0]
		s.queueEvents = s.queueEvents[1:]
		s.mu.Unlock()
		s.emit(event)
		s.mu.Lock()
	}
	close(s.queueDispatchDone)
	s.queueDispatchDone = nil
	s.mu.Unlock()
}

func (s *AgentSession) flushQueueEvents() {
	s.dispatchQueueEvents()
	s.mu.RLock()
	done := s.queueDispatchDone
	s.mu.RUnlock()
	if done != nil {
		<-done
	}
}
