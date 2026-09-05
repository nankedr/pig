package codingagent_test

import (
	"encoding/json"
	"github.com/nankedr/pig/internal/catalog"
	"os"
	"path/filepath"
	"testing"
)

func issue72SessionEvidence(t *testing.T, catalogID string) []issue32ModuleEvidenceDescriptor {
	if catalogID != issue32V3JSONLCatalogID && catalogID != issue32MigrationCatalogID {
		return nil
	}
	data, err := os.ReadFile(filepath.Join(issue32RepoRoot(t), "parity/oracle/fixtures/session-interop.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		InputHash       string `json:"input_hash"`
		ObservationHash string `json:"observation_hash"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	descriptors := []issue32ModuleEvidenceDescriptor{
		{InputPath: "parity/oracle/fixtures/session-interop.json", PinnedInputHash: fixture.InputHash, Evidence: catalog.Evidence{Kind: "oracle", Ref: "parity/oracle/fixtures/session-interop.json", Baseline: "936aff00918de1187f085f123c2812d8f2d67745", CaseID: "go-sdk/codingagent/session-interop", ExecutionMethod: "node --experimental-strip-types parity/oracle/session-interop.mjs <locked-pi-checkout> --check", Expected: "Historical open/migrate/save/reopen, all v3 entry records, open messages, header validation and context match " + fixture.ObservationHash, Actual: "PASS; " + fixture.ObservationHash, Platform: "any", CatalogID: catalogID}},
		{InputPath: "parity/oracle/fixtures/session-interop.json", PinnedInputHash: fixture.InputHash, Evidence: catalog.Evidence{Kind: "go-test", Ref: "internal/parity/session_interop_test.go#TestSessionInteropParity", Baseline: "936aff00918de1187f085f123c2812d8f2d67745", CaseID: "go-sdk/codingagent/session-interop", ExecutionMethod: "go test ./internal/parity -run '^TestSessionInteropParity$' -count=1", Expected: "Historical open/migrate/save/reopen, all v3 entry records, open messages, header validation and context match " + fixture.ObservationHash, Actual: "PASS; " + fixture.ObservationHash, Platform: "any", CatalogID: catalogID}},
		{InputPath: "examples/session-interop/main.go", Evidence: catalog.Evidence{Kind: "oracle", Ref: "parity/oracle/fixtures/session-interop-pig-writer.json", Baseline: "936aff00918de1187f085f123c2812d8f2d67745", CaseID: "issue72-pig-writer-pi-reader", ExecutionMethod: "node --experimental-strip-types parity/oracle/session-interop.mjs <locked-pi-checkout> --check", Expected: "Pig production writer output is consumed by the fixed Pi formal reader with declared nondeterministic fields normalized", Actual: "PASS; Pi reader restored the Pig SDK conversation and retained historical and open fields", Platform: "any", CatalogID: catalogID}},
		{InputPath: "codingagent/issue72_session_restore_test.go", Evidence: catalog.Evidence{Kind: "go-test", Ref: "codingagent/issue72_session_restore_test.go#TestHistoricalSummariesReachResumedProvider", Baseline: "936aff00918de1187f085f123c2812d8f2d67745", CaseID: "issue72-historical-provider-context", ExecutionMethod: "go test ./codingagent -run '^Test(HistoricalSummariesReachResumedProvider|SessionWriterPreservesTypedMessagesForPi)$' -count=1", Expected: "Historical compaction/branch/custom summaries reach the resumed Provider and the reply persists", Actual: "PASS; v1/v2/v3 continued through CreateAgentSession, Prompt and explicit reopen", Platform: "any", CatalogID: catalogID}},
		{InputPath: "codingagent/issue72_session_limits_test.go", Evidence: catalog.Evidence{Kind: "go-test", Ref: "codingagent/issue72_session_limits_test.go#TestOpenSessionLargerThanNodeStringLimitWithManyEntries", Baseline: "936aff00918de1187f085f123c2812d8f2d67745", CaseID: "issue72-session-read-limits", ExecutionMethod: "go test ./codingagent -run '^TestOpen(HistoricalSession|SessionLarger)' -count=1", Expected: "Explicit reads support >512 MiB sparse files, >1 MiB headers/prefixes, UTF-8 long lines, no final newline and 10001 entries", Actual: "PASS; complete history restored and legacy long-line continuation persisted", Platform: "any", CatalogID: catalogID}},
		{InputPath: "cmd/pig/issue72_process_test.go", Evidence: catalog.Evidence{Kind: "go-test", Ref: "cmd/pig/issue72_process_test.go#TestPigProcessesContinueHistoricalSessions", Baseline: "936aff00918de1187f085f123c2812d8f2d67745", CaseID: "issue72-cli-historical-session", ExecutionMethod: "go test ./cmd/pig -run '^TestPigProcessesContinueHistoricalSessions$' -count=1", Expected: "Real CLI restores v1/v2/v3 history into the Provider request and saves a readable v3 continuation", Actual: "PASS; history, parent chains and unknown header metadata survive explicit CLI continuation", Platform: "any", CatalogID: catalogID}},
	}
	if catalogID == issue32MigrationCatalogID {
		return descriptors[:4]
	}
	return descriptors
}
