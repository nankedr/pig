package codingagent

import (
	"encoding/json"
	"fmt"

	"github.com/nankedr/pig/agent"
)

func decodeRPCEvent(raw []byte) (JSONAgentSessionEvent, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, err
	}
	var kind AgentSessionEventType
	if err := json.Unmarshal(fields["type"], &kind); err != nil {
		return nil, err
	}
	var event JSONAgentSessionEvent
	switch kind {
	case AgentSessionEventTypeMessageUpdate:
		event = &JSONAgentSessionMessageUpdateEvent{}
	case AgentSessionEventTypeAgentSettled:
		event = &AgentSessionAgentSettledEvent{}
	case AgentSessionEventTypeQueueUpdate:
		event = &AgentSessionQueueUpdateEvent{}
	case AgentSessionEventTypeEntryAppended:
		event = &AgentSessionEntryAppendedEvent{}
	case AgentSessionEventTypeSessionInfoChanged:
		event = &AgentSessionInfoChangedEvent{}
	case AgentSessionEventTypeThinkingLevelChanged:
		event = &AgentSessionThinkingLevelChangedEvent{}
	case AgentSessionEventTypeAutoRetryStart:
		event = &AgentSessionAutoRetryStartEvent{}
	case AgentSessionEventTypeAutoRetryEnd:
		event = &AgentSessionAutoRetryEndEvent{}
	default:
		var retry bool
		if kind == AgentSessionEventTypeAgentEnd {
			_ = json.Unmarshal(fields["willRetry"], &retry)
			delete(fields, "willRetry")
			raw, _ = json.Marshal(fields)
		}
		base, err := agent.UnmarshalAgentEvent(raw)
		if err != nil {
			return nil, err
		}
		session, err := bridgeAgentSessionEvent(base)
		if err != nil {
			return nil, err
		}
		if ended, ok := session.(AgentSessionAgentEndEvent); ok {
			ended.WillRetry = retry
			session = ended
		}
		result, ok := session.(JSONAgentSessionEvent)
		if !ok {
			return nil, fmt.Errorf("unsupported RPC event %q", kind)
		}
		return result, nil
	}
	if err := json.Unmarshal(raw, event); err != nil {
		return nil, err
	}
	return event, nil
}
