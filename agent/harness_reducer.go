package agent

import (
	"fmt"
	"sort"

	"github.com/nankedr/pig/ai"
)

type RecordLogCorruptionReason string

const (
	RecordLogMultipleOpenOperations   RecordLogCorruptionReason = "multiple_open_operations"
	RecordLogUnknownOperation         RecordLogCorruptionReason = "unknown_operation"
	RecordLogRecordAfterFinish        RecordLogCorruptionReason = "record_after_finish"
	RecordLogNonConsecutiveAttempt    RecordLogCorruptionReason = "non_consecutive_attempt"
	RecordLogInvalidCompactionReason  RecordLogCorruptionReason = "invalid_compaction_reason"
	RecordLogQueueAfterAbort          RecordLogCorruptionReason = "queue_after_abort"
	RecordLogInvalidQueueCancellation RecordLogCorruptionReason = "invalid_queue_cancellation"
	RecordLogToolCallMismatch         RecordLogCorruptionReason = "tool_call_mismatch"
	RecordLogDuplicateToolInvocation  RecordLogCorruptionReason = "duplicate_tool_invocation"
	RecordLogInvalidDeferredHandle    RecordLogCorruptionReason = "invalid_deferred_handle"
)

type RecordLogCorruption struct {
	Reason  RecordLogCorruptionReason
	Message string
}

func (e *RecordLogCorruption) Error() string { return e.Message }

type RecordLogSlice struct {
	Lane           string
	OpenOperations []OperationStartedRecord
	Records        []LaneRecord
	Entries        []Entry
}

func corruptRecordLog(reason RecordLogCorruptionReason, format string, args ...any) error {
	return &RecordLogCorruption{Reason: reason, Message: fmt.Sprintf(format, args...)}
}

func reducerRecordRunID(record LaneRecord) (string, bool) {
	switch record := record.(type) {
	case AbortRequestedRecord:
		return record.RunID, true
	case OperationFinishedRecord:
		return record.RunID, true
	case StepAttemptRecord:
		return record.RunID, true
	case ToolStartedRecord:
		return record.RunID, true
	case QueueEnqueuedRecord:
		return record.RunID, record.RunIDSet
	case QueueCancelledRecord:
		return record.RunID, record.RunIDSet
	case WriteDeferredRecord:
		return record.RunID, true
	case UsageRecord:
		return record.RunID, record.RunIDSet
	default:
		return "", false
	}
}

func ValidateRecordLog(input RecordLogSlice) error {
	if len(input.OpenOperations) > 1 {
		return corruptRecordLog(RecordLogMultipleOpenOperations, "lane %s has at least two open operations", input.Lane)
	}

	entries := make(map[string]Entry, len(input.Entries))
	for _, entry := range input.Entries {
		base := entry.entryBase()
		entries[base.ID] = entry
		if message, ok := entry.(MessageEntry); ok {
			assistant, ok := message.Message.(ai.AssistantMessage)
			if ok && assistant.StopReason == ai.StopReasonDeferred && !assistant.Deferred.IsSet() {
				return corruptRecordLog(RecordLogInvalidDeferredHandle, "deferred assistant entry %s does not carry a handle", base.ID)
			}
		}
	}

	records := append([]LaneRecord(nil), input.Records...)
	sort.SliceStable(records, func(i, j int) bool { return records[i].recordBase().Seq < records[j].recordBase().Seq })
	starts := make(map[string]OperationStartedRecord)
	finishedAt := make(map[string]int64)
	abortedAt := make(map[string]int64)
	attempts := make(map[string]StepAttemptRecord)
	queues := make(map[string]QueueEnqueuedRecord)
	toolInvocations := make(map[string]struct{})

	for _, record := range records {
		base := record.recordBase()
		if started, ok := record.(OperationStartedRecord); ok {
			starts[started.ID] = started
			continue
		}

		runID, hasRunID := reducerRecordRunID(record)
		if hasRunID {
			if _, ok := starts[runID]; !ok {
				return corruptRecordLog(RecordLogUnknownOperation, "record %s references unknown operation %s", base.ID, runID)
			}
			if finish, ok := finishedAt[runID]; ok && base.Seq > finish {
				return corruptRecordLog(RecordLogRecordAfterFinish, "record %s follows the finish of operation %s", base.ID, runID)
			}
		}

		switch record := record.(type) {
		case OperationFinishedRecord:
			finishedAt[record.RunID] = record.Seq
		case AbortRequestedRecord:
			abortedAt[record.RunID] = record.Seq
		case StepAttemptRecord:
			if err := validateStepAttempt(record, attempts[record.RunID], entries); err != nil {
				return err
			}
			attempts[record.RunID] = record
		case ToolStartedRecord:
			invocation := fmt.Sprintf("%s\x00%d", record.AssistantEntryID, record.ToolIndex)
			if _, exists := toolInvocations[invocation]; exists {
				return corruptRecordLog(RecordLogDuplicateToolInvocation, "tool invocation %s:%d is duplicated", record.AssistantEntryID, record.ToolIndex)
			}
			toolInvocations[invocation] = struct{}{}
			if !matchesToolCall(record, entries[record.AssistantEntryID]) {
				return corruptRecordLog(RecordLogToolCallMismatch, "tool start %s does not match its assistant tool-call ordinal", record.ID)
			}
		case QueueEnqueuedRecord:
			if record.Queue != QueueNameNextRun {
				if abort, ok := abortedAt[record.RunID]; ok && record.Seq > abort {
					return corruptRecordLog(RecordLogQueueAfterAbort, "%s item was enqueued after abort", record.Queue)
				}
			}
			queues[record.Target.entryBase().ID] = record
		case QueueCancelledRecord:
			enqueue, ok := queues[record.EntryID]
			if !ok || enqueue.Seq >= record.Seq || enqueue.RunID != record.RunID {
				return corruptRecordLog(RecordLogInvalidQueueCancellation, "queue cancellation %s has no pending matching enqueue", record.ID)
			}
			if _, exists := entries[record.EntryID]; exists {
				return corruptRecordLog(RecordLogInvalidQueueCancellation, "queue cancellation %s targets an existing entry", record.ID)
			}
		}
	}
	return nil
}

func validateStepAttempt(record, previous StepAttemptRecord, entries map[string]Entry) error {
	reason := record.CompactionReason
	if record.Step == StepKindCompaction {
		if reason == nil || (*reason != CompactionReasonManual && *reason != CompactionReasonThreshold && *reason != CompactionReasonOverflow) {
			return corruptRecordLog(RecordLogInvalidCompactionReason, "compaction attempt %s has no valid compaction reason", record.ID)
		}
	} else if reason != nil {
		return corruptRecordLog(RecordLogInvalidCompactionReason, "%s attempt %s has a compaction reason", record.Step, record.ID)
	}

	continues := previous.ID != "" && previous.Step == record.Step
	if entry, ok := entries[previous.ResultEntryID]; ok && entry.entryBase().Seq < record.Seq {
		continues = false
	}
	expected := 1
	if continues {
		expected = previous.Attempt + 1
	}
	if record.Attempt != expected {
		return corruptRecordLog(RecordLogNonConsecutiveAttempt, "%s attempt %s is %d; expected %d", record.Step, record.ID, record.Attempt, expected)
	}
	return nil
}

func matchesToolCall(record ToolStartedRecord, entry Entry) bool {
	messageEntry, ok := entry.(MessageEntry)
	if !ok {
		return false
	}
	assistant, ok := messageEntry.Message.(ai.AssistantMessage)
	if !ok {
		return false
	}
	index := 0
	for _, content := range assistant.Content {
		call, ok := content.(ai.ToolCall)
		if !ok {
			continue
		}
		if index == record.ToolIndex {
			return call.ID == record.ToolCallID && call.Name == record.ToolName
		}
		index++
	}
	return false
}
