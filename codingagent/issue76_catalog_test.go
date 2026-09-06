package codingagent_test

import (
	"github.com/nankedr/pig/internal/catalog"
	"testing"
)

const issue76CredentialCatalogID = "contract:config/auth-json"

func issue76CredentialCatalogEntry() catalog.Entry {
	return catalog.Entry{
		SchemaVersion: catalog.SchemaVersion, ID: issue76CredentialCatalogID,
		Upstream: catalog.Upstream{Module: "coding-agent", Repository: "https://github.com/badlogic/pi-mono", Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/core/auth-storage.ts"},
		Mapping:  catalog.Mapping{Module: "codingagent", Target: issue32GoPackage + ".NewAuthStorage", Kind: "contract"}, Status: catalog.StatusPartial, Milestone: "M3", Classification: "public-api",
		Partial:   &catalog.Partial{Supported: []string{"canonical file CredentialStore read/modify/list/delete, raw ReadStoredCredential, explicit AuthPath and Headless request restoration across processes", "0700 created parents, 0600 writes, cancellable cross-process flock and locked direct overwrite on darwin/linux; raw extra credential fields retain precision", "literal/env/template/escape resolution and post-trust shell commands with 10 second timeout, discarded stderr and process-lifetime success/failure cache", "CLI key overrides owned store entries; failed entries never fall back to environment; request credential redaction in errors and sessions"}, Unsupported: []string{"OAuth/ambient login, ModelRuntime orchestration, and pig-ai credential commands remain M11 Capability Stubs", "read-only storage facade and non-darwin/linux file locking remain deferred"}},
		Deviation: &catalog.Deviation{ADR: "docs/adr/0021-canonical-file-credentials.md", Reason: "ADR-0008 canonical Pig paths replace Pi cwd auth fallback. Owned unresolved entries and corrupt stores fail closed instead of Pi undefined/stale fallback; kernel file locks replace proper-lockfile within Pig-only state. Known request secrets are redacted from Provider error text."},
		Notes:     "Issue #76. NewAuthStorage is inert until an operation; raw read and metadata list never execute references. Explicit store reads authorize host execution; Headless constructs its store after the trust decision. Auth files are never copied from project or Pi state. Command stderr and credential values are excluded from default logs and Telemetry.",
	}
}

func issue76CredentialEvidence(t *testing.T) []issue32ModuleEvidenceDescriptor {
	t.Helper()
	var out []issue32ModuleEvidenceDescriptor
	for _, e := range []struct{ kind, path, id, run string }{
		{"oracle", "parity/oracle/fixtures/credentials.json", "issue76-credential-oracle", "node --experimental-strip-types parity/oracle/credentials.mjs <locked-pi-checkout> --check"},
		{"go-test", "internal/parity/credentials_test.go", "issue76-credential-parity", "go test ./internal/parity -run '^TestCredentialStorageParity$' -count=1"},
		{"go-test", "codingagent/issue76_credentials_test.go", "issue76-credential-storage", "go test -race ./codingagent -run '^TestCredential76' -count=1"},
		{"go-test", "codingagent/issue76_headless_test.go", "issue76-credential-sdk", "go test ./codingagent -run '^TestCredential76(Explicit|Models)' -count=1"},
		{"go-test", "cmd/pig/issue76_process_test.go", "issue76-credential-process", "go test ./cmd/pig -run '^TestPigCanonicalCredentials$' -count=1"},
		{"go-test", "internal/pigaicli/issue76_path_test.go", "issue76-credential-path", "go test ./internal/pigaicli -run '^TestCredential76' -count=1"},
	} {
		out = append(out, issue32ModuleEvidenceDescriptor{InputPath: e.path, Evidence: catalog.Evidence{Kind: e.kind, Ref: e.path, Baseline: issue32BaselineCommit, CaseID: e.id, ExecutionMethod: e.run, Expected: "canonical credentials survive process boundaries; references, ownership, concurrency, trust ordering and secret redaction follow the slice contract", Actual: "PASS; deterministic Pi fixture plus public SDK and real Headless process tests cover credentials and explicit remaining stubs", Platform: "darwin/linux", CatalogID: issue76CredentialCatalogID}})
	}
	return out
}
