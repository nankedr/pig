package codingagent_test

import (
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/catalog"
)

const issue85MessagesCatalogID = "contract:codingagent/session-messages"

func issue85MessagesCatalogEntry() catalog.Entry {
	return catalog.Entry{
		SchemaVersion: catalog.SchemaVersion, ID: issue85MessagesCatalogID,
		Upstream: catalog.Upstream{Module: "coding-agent", Repository: "https://github.com/badlogic/pi-mono", Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/core/agent-session.ts"},
		Mapping:  catalog.Mapping{Module: "codingagent", Target: issue32GoPackage + ".AgentSession", Kind: "contract"}, Status: catalog.StatusPartial, Milestone: "M4", Classification: "public-api",
		Partial: &catalog.Partial{Supported: []string{"text SendUserMessage normalization and idle execution, running DeliverAs and Prompt StreamingBehavior routing to Legacy Agent", "Steer/FollowUp admission, FIFO one/all modes with saved settings, owned queue queries, pending count and ClearQueue", "queue_update before user message_start, message_end persistence and reopen without duplicate consumed history; cancellation retention and concurrent final-turn admission"}, Unsupported: []string{"images, template/skill expansion, extension input/commands/runtime, PreflightResult and Source remain exact capability boundaries", "manual-compaction queues, new enqueue during Provider retry backoff, interactive UI; text RPC delivery is covered by contract:rpc/session-control; automatic-compaction queues are covered by contract:codingagent/compaction; queued but unconsumed messages are in-memory only",
			"starting a new run while queue notifications are in flight is rejected under the synchronous Go callback boundary; active-run enqueue remains supported", "ClearQueue retains its published error-only Go signature; queues manipulated directly through Session.Agent are outside Session display tracking"}},
		Deviation: &catalog.Deviation{ADR: "docs/adr/0018-legacy-agent-queue-admission.md", Reason: "Session reuses Legacy Agent idle/cancel/settlement rejection; generated monotonic message timestamps distinguish repeated and empty text when tracking consumption."},
		Notes:     "Issue #85 verifies the public CreateAgentSession boundary against locked Pi fixtures. Common fixture compares all four queue mode combinations, text blocks, clear, missing delivery rejection, queue/message event order, model inputs and persisted history. Separate Pi admission fixture records the accepted queues that Pig rejects under ADR-0018; no normalization hides that difference. Baseline declaration rows remain structural mappings; this contract owns behavioral status.",
	}
}
func issue85MessagesEvidence(t *testing.T) []issue32ModuleEvidenceDescriptor {
	t.Helper()
	out := []issue32ModuleEvidenceDescriptor{}
	for _, e := range []struct{ kind, path, ref, id, run string }{
		{"oracle", "parity/oracle/fixtures/session-messages.json", "parity/oracle/fixtures/session-messages.json", "go-sdk/codingagent/session-messages", "node --experimental-strip-types parity/oracle/session-messages.mjs <locked-pi-checkout> --check"},
		{"oracle", "parity/oracle/fixtures/session-messages-deviation.json", "parity/oracle/fixtures/session-messages-deviation.json", "go-sdk/codingagent/session-messages-deviation", "node --experimental-strip-types parity/oracle/session-messages.mjs <locked-pi-checkout> --check"},
		{"go-test", "codingagent/issue85_messages_test.go", "codingagent/issue85_messages_test.go#TestSessionMessagesParity", "issue85-session-messages-sdk", "go test -race ./codingagent -run '^TestSessionMessages' -count=20 -shuffle=on"},
		{"go-test", "codingagent/testdata/issue85_surface_golden.txt", "codingagent/issue85_catalog_test.go#TestSessionMessagesAPISnapshot", "issue85-session-messages-api", "go test ./codingagent -run '^TestSessionMessagesAPISnapshot$' -count=1"},
	} {
		out = append(out, issue32ModuleEvidenceDescriptor{InputPath: e.path, Evidence: catalog.Evidence{Kind: e.kind, Ref: e.ref, Baseline: issue32BaselineCommit, CaseID: e.id, ExecutionMethod: e.run, Expected: "public SDK text queues match Pi common fixture; ADR-0018 admission, duplicate/empty text, cancellation, concurrent settling and persistence remain consistent", Actual: "PASS; public SDK parity and race tests, explicit Pi admission deviation and API snapshot", Platform: "any", CatalogID: issue85MessagesCatalogID}})
	}
	return out
}

var updateIssue85Surface = flag.Bool("update-issue85-surface", false, "regenerate issue #85 API snapshot")

func TestSessionMessagesAPISnapshot(t *testing.T) {
	typ := reflect.TypeOf((*codingagent.AgentSession)(nil))
	parts := []string{}
	for _, name := range []string{"SendUserMessage", "Prompt", "Steer", "FollowUp", "SetSteeringMode", "SetFollowUpMode", "SteeringMode", "FollowUpMode", "GetSteeringMessages", "GetFollowUpMessages", "PendingMessageCount", "ClearQueue", "Abort", "WaitForIdle"} {
		m, ok := typ.MethodByName(name)
		if !ok {
			t.Fatal(name)
		}
		parts = append(parts, name+" "+m.Type.String())
	}
	for _, v := range []any{codingagent.SendUserMessageOptions{}, codingagent.PromptOptions{}, codingagent.AgentSessionQueueUpdateEvent{}} {
		typ := reflect.TypeOf(v)
		parts = append(parts, issue71TypeSnapshot(typ.Name(), typ))
	}
	got := strings.Join(parts, "\n\n") + "\n"
	path := filepath.Join("testdata", "issue85_surface_golden.txt")
	if *updateIssue85Surface {
		if err := os.WriteFile(path, []byte(got), 0644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatal("issue #85 API drift")
	}
}
