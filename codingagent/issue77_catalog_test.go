package codingagent_test

import (
	"github.com/nankedr/pig/internal/catalog"
	"testing"
)

const issue77RuntimeCatalogID = "contract:model-runtime/basic"

func issue77RuntimeCatalogEntry() catalog.Entry {
	return catalog.Entry{SchemaVersion: catalog.SchemaVersion, ID: issue77RuntimeCatalogID, Upstream: catalog.Upstream{Module: "coding-agent", Repository: "https://github.com/badlogic/pi-mono", Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/core/model-runtime.ts"}, Mapping: catalog.Mapping{Module: "codingagent", Target: issue32GoPackage + ".NewModelRuntime", Kind: "contract"}, Status: catalog.StatusPartial, Milestone: "M3", Classification: "public-api", Partial: &catalog.Partial{Supported: []string{"embedded immutable 1220-model Catalog Snapshot queries, implemented API-key availability and secret-free source snapshots via ModelRuntime and ModelRegistry", "CLI model/thinking resolution, model scope, saved settings and Session restoration with deterministic fallback, coherent Session services and shared SDK/Headless requests", "existing DeepSeek text/read continuation, terminal error/redaction and cancellation; list-models and unified Offline parsing; no implicit catalog/control-plane network"}, Unsupported: []string{"catalog generation, cache, overlay, network refresh and native/extension provider registration remain deferred to M10/M7", "non-DeepSeek Provider auth/execution, OAuth and ambient auth remain their existing M10/M11 Capability Stubs; basic glob scope excludes minimatch extglob/brace expansion"}}, Notes: "Issue #77. API visibility and authentication availability do not imply an implemented Adapter. Create-time snapshot has no background refresh. Explicit inference is unaffected by Offline. CLI missing-provider custom model IDs follow fixed Pi; Pig retains canonical credential ownership and error redaction. SDK ResourceLoader extensions remain deferred."}
}

func issue77RuntimeEvidence(t *testing.T) []issue32ModuleEvidenceDescriptor {
	t.Helper()
	var result []issue32ModuleEvidenceDescriptor
	for _, e := range []struct{ kind, path, id, run string }{
		{"oracle", "parity/oracle/fixtures/model-runtime.json", "issue77-runtime-oracle", "node --experimental-strip-types parity/oracle/model-runtime.mjs <locked-pi-checkout> --check"},
		{"go-test", "internal/parity/model_runtime_test.go", "issue77-runtime-parity", "go test ./internal/parity -run '^TestModelRuntimeParity$' -count=1"},
		{"go-test", "codingagent/issue77_runtime_test.go", "issue77-runtime-sdk", "go test -race ./codingagent -run '^TestModelRuntime77' -count=1"},
		{"go-test", "cmd/pig/issue77_process_test.go", "issue77-runtime-cli", "go test ./cmd/pig -run '^TestPigModelRuntime77$' -count=1"},
	} {
		result = append(result, issue32ModuleEvidenceDescriptor{InputPath: e.path, Evidence: catalog.Evidence{Kind: e.kind, Ref: e.path, Baseline: issue32BaselineCommit, CaseID: e.id, ExecutionMethod: e.run, Expected: "fixed Pi model resolution and scope, shared SDK/CLI startup and inference, embedded snapshot and no implicit control-plane network", Actual: "PASS; fixed Pi fixture, SDK and process tests preserve model/thinking selection, read continuation, error/cancellation and credential redaction", Platform: "darwin/linux", CatalogID: issue77RuntimeCatalogID}})
	}
	return result
}
