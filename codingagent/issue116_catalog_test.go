package codingagent_test

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/catalog"
	"github.com/nankedr/pig/tui"
)

var updateIssue116 = flag.Bool("update-issue116", false, "refresh interactive queues evidence and API snapshot")

func partial116() *catalog.Partial {
	return &catalog.Partial{Supported: []string{
		"Issue #116: pinned Pi real CLI PTY fixtures for steering/follow-up priority, dequeue/edit/clear, abort preserving Assistant output, next submission, retry success and cancelled waiting; active Alt+Enter preserves builtin-looking text",
		"Public AgentSession atomic TakeQueuedMessages, queue callback reentrancy and one/all consumption races; TextInput turn identity isolates delayed interrupts and submissions",
		"Real CLI Bash partial output and v3 Session error persistence, active exit paths and race-instrumented child process cleanup",
	}, Unsupported: []string{
		"New queue admission during retry waiting follows ADR-0018 rejection; compaction UI and every Tool cancellation branch are not claimed fully equivalent",
		"Full Pi spinner/pixel layout, extension runtime M7, package ecosystem #99, full authentication M11, images M12 and six-platform runtime acceptance M13 remain partial/deferred",
	}}
}
func TestInteractiveQueuesCatalog116(t *testing.T) {
	root := issue32RepoRoot(t)
	entry := catalog.Entry{SchemaVersion: catalog.SchemaVersion, ID: "contract:codingagent/interactive-queues", Upstream: catalog.Upstream{Module: "coding-agent", Repository: "https://github.com/badlogic/pi-mono", Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/modes/interactive/interactive-mode.ts"}, Mapping: catalog.Mapping{Module: "codingagent", Target: issue32GoPackage + ".InteractiveMode", Kind: "contract"}, Status: catalog.StatusPartial, Milestone: "M6", Classification: "public-api", Partial: partial116(), Deviation: &catalog.Deviation{ADR: "docs/adr/0018-legacy-agent-queue-admission.md", Reason: "Pig keeps explicit idle/settling/retry admission; unaccepted settling inputs wait for worker completion or return to editor on cancellation."}, Notes: "Issue #116; docs/learning/m6-interactive-queues.md. Reuses legacy AgentSession and v3 Session, with no transport retry changes."}
	for _, item := range []struct{ kind, path, run string }{
		{"oracle", "parity/oracle/fixtures/queues-commands.json", "node parity/oracle/queues.mjs <locked-pi-checkout> --check"},
		{"oracle", "parity/oracle/fixtures/queues.json", "node parity/oracle/queues.mjs <locked-pi-checkout> --check"},
		{"oracle", "parity/oracle/fixtures/queues-restore.json", "node parity/oracle/queues.mjs <locked-pi-checkout> --check"},
		{"oracle", "parity/oracle/fixtures/queues-abort.json", "node parity/oracle/queues.mjs <locked-pi-checkout> --check"},
		{"oracle", "parity/oracle/fixtures/queues-retry.json", "node parity/oracle/queues.mjs <locked-pi-checkout> --check"},
		{"oracle", "parity/oracle/fixtures/queues-retry-success.json", "node parity/oracle/queues.mjs <locked-pi-checkout> --check"},
		{"go-test", "codingagent/issue116_interactive_test.go", "go test -race ./codingagent -run TestInteractiveSDKSettlementCancel116 -count=3 -shuffle=on"},
		{"go-test", "codingagent/issue116_queues_test.go", "go test -race ./codingagent -run 116 -count=3 -shuffle=on"},
		{"go-test", "cmd/pig/issue116_queues_test.go", "PIG_TEST_RACE=1 go test -race ./cmd/pig -run 116 -count=3 -shuffle=on"},
		{"go-test", "codingagent/testdata/issue116_surface_golden.txt", "go test ./codingagent -run TestInteractiveQueuesAPISnapshot116 -count=1"},
		{"manual", "examples/interactive-queues/main.go", "go run ./examples/interactive-queues"},
	} {
		data, err := os.ReadFile(filepath.Join(root, item.path))
		if err != nil {
			t.Fatal(err)
		}
		entry.Evidence = append(entry.Evidence, catalog.Evidence{Kind: item.kind, Ref: item.path, Baseline: issue32BaselineCommit, CaseID: "issue116-" + item.path, InputHash: fmt.Sprintf("sha256:%x", sha256.Sum256(data)), ExecutionMethod: item.run, Expected: "queue priority, restore, cancel, retry and settled lifecycle through public SDK and CLI", Actual: "PASS; explicit partial scope", Platform: "any (SDK), darwin/linux (PTY)", CatalogID: entry.ID})
	}
	path := filepath.Join(root, "parity/catalog.jsonl")
	entries, err := catalog.LoadCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for i, e := range entries {
		if e.ID == entry.ID {
			found = true
			if *updateIssue116 {
				entries[i] = entry
			} else if !reflect.DeepEqual(e, entry) {
				t.Fatal("interactive queue evidence drift")
			}
		}
	}
	if *updateIssue116 {
		if !found {
			entries = append(entries, entry)
		}
		data, err := catalog.EncodeEntries(entries)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, data, 0644); err != nil {
			t.Fatal(err)
		}
	} else if !found {
		t.Fatal("missing interactive queue contract")
	}
}

func TestInteractiveQueuesAPISnapshot116(t *testing.T) {
	var b strings.Builder
	for _, item := range []struct {
		value any
		names []string
	}{
		{(*agent.Agent)(nil), []string{"TakeQueuedMessages"}},
		{(*codingagent.AgentSession)(nil), []string{"TakeQueuedMessages", "Steer", "FollowUp", "Abort", "AbortRetry", "WaitForIdle"}},
		{(*tui.TextUI)(nil), []string{"PrependEditor", "SetTurn", "ReadInput"}},
	} {
		typ := reflect.TypeOf(item.value)
		for _, name := range item.names {
			m, ok := typ.MethodByName(name)
			if !ok {
				t.Fatal(name)
			}
			fmt.Fprintln(&b, m.Type)
		}
	}
	fmt.Fprintln(&b, issue71TypeSnapshot("TextInput", reflect.TypeOf(tui.TextInput{})))
	fmt.Fprintln(&b, agent.ErrAgentIdle, agent.ErrAgentSettling)
	path := "testdata/issue116_surface_golden.txt"
	if *updateIssue116 {
		if err := os.WriteFile(path, []byte(b.String()), 0644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(want) != b.String() {
		t.Fatal("interactive queues API drift")
	}
}
