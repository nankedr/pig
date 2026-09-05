package capability_test

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/nankedr/pig/internal/catalog"
)

func TestIssue33CatalogRecordsTheProductEntryContract(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	entries, err := catalog.LoadCatalog(filepath.Join(root, "parity", "catalog.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]struct {
		status       string
		evidence     string
		deviationADR string
		phrases      []string
	}{
		"cmd-pig":                        {status: catalog.StatusPartial, evidence: "codingagent/main_test.go#TestMainDispatchesStaticMetadataToProcessStdout", phrases: []string{"public product entry", "MainOptions", "Capability Stubs"}},
		"cmd-pig-ai":                     {status: catalog.StatusPartial, evidence: "internal/capability/command_test.go#TestPigAIStaticHelpCommands", phrases: []string{"static help", "--auth-path"}},
		"contract:auth/pig-ai/login-cli": {status: catalog.StatusPartial, evidence: "internal/pigaicli/cli_test.go", deviationADR: "ADR-0008", phrases: []string{"login", "list", "cwd auth.json"}},
		"contract:cli/pig/args":          {status: catalog.StatusPartial, evidence: "codingagent/cli_golden_test.go#TestIssue33StaticCLISnapshots", phrases: []string{"command", "mode", "Extension Surface"}},
		"contract:cli/pig/exit-codes":    {status: catalog.StatusPartial, evidence: "internal/capability/command_test.go#TestCommandStubsHaveNoSideEffects", phrases: []string{"exit 0", "exit 1"}},
		"contract:cli/pig/experimental":  {status: catalog.StatusPartial, evidence: "codingagent/experimental_cli_test.go#TestExperimentalCLIPinnedParsingEdges", phrases: []string{"dormant experimental", "pig/server/client", "does not start"}},
		"deferred-extension-runtime":     {status: catalog.StatusDeferred, evidence: "codingagent/extensions_abi_final_review_test.go#TestDiscoverAndLoadExtensionsPreservesOnlyTheDeferredEntry", phrases: []string{"unknown long flags", "no runtime ABI"}},
		"deferred-pig-server":            {status: catalog.StatusDeferred, phrases: []string{"does not provide a Pig Server", "test host"}},
	}
	for _, entry := range entries {
		for _, evidence := range entry.Evidence {
			assertIssue33EvidenceRefPath(t, root, evidence.Ref)
		}
		expected, ok := want[entry.ID]
		if !ok {
			continue
		}
		if entry.Status != expected.status {
			t.Errorf("%s status = %q, want %q", entry.ID, entry.Status, expected.status)
		}
		if expected.evidence != "" {
			_, ok := catalogEvidenceWithRef(entry.Evidence, expected.evidence)
			if !ok {
				t.Errorf("%s lacks evidence %q: %+v", entry.ID, expected.evidence, entry.Evidence)
			}
			for _, evidence := range entry.Evidence {
				assertCompleteIssue33Evidence(t, root, entry.ID, evidence)
			}
		}
		if expected.deviationADR != "" {
			if entry.Deviation == nil || entry.Deviation.ADR != expected.deviationADR || strings.TrimSpace(entry.Deviation.Reason) == "" {
				t.Errorf("%s deviation = %+v, want %s with a reason", entry.ID, entry.Deviation, expected.deviationADR)
			}
		}
		for _, phrase := range expected.phrases {
			if !strings.Contains(entry.Notes, phrase) {
				t.Errorf("%s notes do not contain %q: %q", entry.ID, phrase, entry.Notes)
			}
		}
		delete(want, entry.ID)
	}
	for id := range want {
		t.Errorf("missing Issue #33 catalog row %s", id)
	}
}

func catalogEvidenceWithRef(evidence []catalog.Evidence, ref string) (catalog.Evidence, bool) {
	for _, item := range evidence {
		if item.Ref == ref {
			return item, true
		}
	}
	return catalog.Evidence{}, false
}

func assertCompleteIssue33Evidence(t *testing.T, root, entryID string, evidence catalog.Evidence) {
	t.Helper()
	if evidence.Baseline != "936aff00918de1187f085f123c2812d8f2d67745" {
		t.Errorf("%s evidence baseline = %q", entryID, evidence.Baseline)
	}
	if evidence.CaseID == "" || evidence.ExecutionMethod == "" || evidence.Expected == "" ||
		evidence.Actual == "" || evidence.Platform == "" {
		t.Errorf("%s has incomplete evidence metadata: %+v", entryID, evidence)
	}
	if evidence.CatalogID != entryID {
		t.Errorf("%s evidence catalog_id = %q", entryID, evidence.CatalogID)
	}
	const prefix = "sha256:"
	digest := strings.TrimPrefix(evidence.InputHash, prefix)
	if !strings.HasPrefix(evidence.InputHash, prefix) || len(digest) != 64 {
		t.Errorf("%s evidence input_hash = %q", entryID, evidence.InputHash)
		return
	}
	if _, err := hex.DecodeString(digest); err != nil {
		t.Errorf("%s evidence input_hash = %q: %v", entryID, evidence.InputHash, err)
	}
	inputPaths := issue33EvidenceInputPaths(evidence.CaseID)
	if len(inputPaths) == 0 {
		t.Errorf("%s evidence case %q has no declared canonical inputs", entryID, evidence.CaseID)
		return
	}
	actual := issue33CanonicalInputHash(t, root, inputPaths)
	if evidence.InputHash != actual {
		t.Errorf("%s evidence input_hash = %q, want %q for canonical inputs %v", entryID, evidence.InputHash, actual, inputPaths)
	}
}

func assertIssue33EvidenceRefPath(t *testing.T, root, ref string) {
	t.Helper()
	path := ref
	if index := strings.IndexByte(path, '#'); index >= 0 {
		path = path[:index]
	}
	if path == "" {
		t.Errorf("evidence ref %q has empty file path", ref)
		return
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	if filepath.IsAbs(path) || strings.HasPrefix(clean, "../") || clean == ".." || clean != path {
		t.Errorf("evidence ref %q path %q must be a normalized repository-relative path", ref, path)
		return
	}
	file, err := os.Open(filepath.Join(root, filepath.FromSlash(path)))
	if err != nil {
		t.Errorf("evidence ref %q path %q is not readable: %v", ref, path, err)
		return
	}
	if err := file.Close(); err != nil {
		t.Errorf("evidence ref %q path %q close: %v", ref, path, err)
	}
}

func issue33EvidenceInputPaths(caseID string) []string {
	switch caseID {
	case "issue73-cli-session-navigation":
		return []string{"cmd/pig/issue73_process_test.go"}
	case "issue33-pig-process-entry", "issue33-static-success-exits":
		return []string{
			"internal/capability/command_test.go",
			"codingagent/testdata/pig_help.golden.txt",
			"codingagent/testdata/pig_install_help.golden.txt",
			"codingagent/testdata/pig_remove_help.golden.txt",
			"codingagent/testdata/pig_update_help.golden.txt",
			"codingagent/testdata/pig_list_help.golden.txt",
			"codingagent/testdata/pig_config_help.golden.txt",
			"codingagent/testdata/pig_auth_help.golden.txt",
		}
	case "issue33-pig-static-help-snapshots", "issue33-pig-static-cli-surface":
		return []string{
			"codingagent/cli_golden_test.go",
			"codingagent/testdata/pig_help.golden.txt",
			"codingagent/testdata/pig_install_help.golden.txt",
			"codingagent/testdata/pig_remove_help.golden.txt",
			"codingagent/testdata/pig_update_help.golden.txt",
			"codingagent/testdata/pig_list_help.golden.txt",
			"codingagent/testdata/pig_config_help.golden.txt",
			"codingagent/testdata/pig_auth_help.golden.txt",
		}
	case "issue33-pig-ai-process-entry":
		return []string{"internal/capability/command_test.go", "internal/pigaicli/testdata/pig_ai_help.golden.txt"}
	case "issue33-pig-ai-static-help":
		return []string{"internal/pigaicli/cli_golden_test.go", "internal/pigaicli/testdata/pig_ai_help.golden.txt"}
	case "issue33-pig-ai-command-routing":
		return []string{"internal/pigaicli/cli_test.go"}
	case "issue33-pig-ai-routing-snapshot":
		return []string{"internal/pigaicli/cli_golden_test.go", "internal/pigaicli/testdata/pig_ai_routing.golden.txt"}
	case "issue33-pig-ai-auth-isolation", "issue33-current-failure-exits":
		return []string{"internal/capability/command_test.go"}
	case "issue33-pig-cli-routing-snapshot":
		return []string{"codingagent/cli_golden_test.go", "codingagent/testdata/pig_routing.golden.txt"}
	case "issue33-pig-cli-semantics":
		return []string{"codingagent/auth_command_final_review_test.go", "codingagent/cli_test.go"}
	case "issue33-pig-public-main":
		return []string{"codingagent/main_test.go", "codingagent/testdata/pig_help.golden.txt"}
	case "issue33-stdout-write-failure":
		return []string{"cmd/pig/main_test.go"}
	case "issue33-dormant-experimental-cli-edges":
		return []string{"codingagent/experimental_cli_test.go"}
	case "issue33-dormant-experimental-cli-boundary":
		return []string{"codingagent/experimental_cli_test.go"}
	case "issue33-extension-cli-routing":
		return []string{"codingagent/cli_test.go"}
	case "issue33-dormant-experimental-cli-routing-snapshot":
		return []string{"codingagent/cli_golden_test.go", "codingagent/testdata/pig_experimental_routing.golden.txt"}
	case "issue33-extension-runtime-boundary":
		return []string{"codingagent/extensions_abi_final_review_test.go"}
	case "issue56-pig-process-headless-text", "issue56-pig-headless-exit-status":
		return []string{"cmd/pig/headless_process_test.go"}
	case "issue57-pig-session-first-json", "issue57-pig-json-cli-contract", "issue57-pig-json-exit-status", "issue57-headless-json-boundary":
		return []string{"cmd/pig/headless_process_test.go", "cmd/pig/testdata/headless_json_text.golden.jsonl"}
	case "issue71-pig-session-persistence", "issue71-pig-cli-session-args", "issue71-headless-session-persistence":
		return []string{"cmd/pig/issue71_process_test.go"}
	case "issue72-cli-historical-product":
		return []string{"cmd/pig/issue72_process_test.go"}
	case "issue71-headless-session-outcomes":
		return []string{"codingagent/issue71_session_persistence_test.go"}
	case "issue74-global-settings-startup":
		return []string{"cmd/pig/issue74_process_test.go", "parity/oracle/fixtures/settings-startup.json"}
	default:
		return nil
	}
}

func issue33CanonicalInputHash(t *testing.T, root string, paths []string) string {
	t.Helper()
	paths = append([]string(nil), paths...)
	sort.Strings(paths)
	canonical := make([]byte, 0)
	for index, relative := range paths {
		if index > 0 && paths[index-1] == relative {
			t.Fatalf("duplicate canonical input %q", relative)
		}
		contents, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatalf("read canonical input %q: %v", relative, err)
		}
		canonical = strconv.AppendInt(canonical, int64(len(relative)), 10)
		canonical = append(canonical, ':')
		canonical = append(canonical, relative...)
		canonical = append(canonical, '\n')
		canonical = strconv.AppendInt(canonical, int64(len(contents)), 10)
		canonical = append(canonical, ':')
		canonical = append(canonical, contents...)
		canonical = append(canonical, '\n')
	}
	sum := sha256.Sum256(canonical)
	return "sha256:" + hex.EncodeToString(sum[:])
}
