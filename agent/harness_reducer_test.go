package agent_test

import (
	"errors"
	"testing"

	"github.com/nankedr/pig/agent"
)

func TestValidateRecordLogRejectsNonConsecutiveAttempt(t *testing.T) {
	started := agent.OperationStartedRecord{
		RecordBase: agent.RecordBase{ID: "run", Seq: 1, Lane: "main"},
		Intent:     agent.RunOperationIntent{},
	}
	attempt := agent.StepAttemptRecord{
		RecordBase:    agent.RecordBase{ID: "attempt", Seq: 2, Lane: "main"},
		RunID:         "run",
		Step:          agent.StepKindAssistant,
		Attempt:       2,
		ResultEntryID: "assistant",
	}

	err := agent.ValidateRecordLog(agent.RecordLogSlice{
		Lane:           "main",
		OpenOperations: []agent.OperationStartedRecord{started},
		Records:        []agent.LaneRecord{started, attempt},
	})
	var corruption *agent.RecordLogCorruption
	if !errors.As(err, &corruption) || corruption.Reason != agent.RecordLogNonConsecutiveAttempt {
		t.Fatalf("ValidateRecordLog() = %v", err)
	}
}

func TestValidateRecordLogRejectsMultipleOpenOperations(t *testing.T) {
	err := agent.ValidateRecordLog(agent.RecordLogSlice{
		Lane:           "main",
		OpenOperations: make([]agent.OperationStartedRecord, 2),
	})
	var corruption *agent.RecordLogCorruption
	if !errors.As(err, &corruption) || corruption.Reason != agent.RecordLogMultipleOpenOperations {
		t.Fatalf("ValidateRecordLog() = %v", err)
	}
}
