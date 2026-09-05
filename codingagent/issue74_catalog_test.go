package codingagent_test

import (
	"github.com/nankedr/pig/internal/catalog"
	"testing"
)

const issue74SettingsCatalogID = "contract:config/settings"

func issue74SettingsCatalogEntry() catalog.Entry {
	return catalog.Entry{
		SchemaVersion: catalog.SchemaVersion, ID: issue74SettingsCatalogID,
		Upstream: catalog.Upstream{Module: "coding-agent", Repository: "https://github.com/badlogic/pi-mono", Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/core/settings-manager.ts"},
		Mapping:  catalog.Mapping{Module: "codingagent", Target: issue32GoPackage + ".SettingsManager", Kind: "contract"}, Status: catalog.StatusPartial, Milestone: "M3", Classification: "public-api",
		Partial:   &catalog.Partial{Supported: []string{"global file and in-memory SettingsManager defaults, deep overrides, migrations, locked field-level saves, reload, flush, error collection and preservation of sparse/unknown data", "Headless CLI and SDK use saved Provider/model/thinking and Session directory; real processes match the locked Pi startup precedence fixture"}, Unsupported: []string{"all project settings access and project writes remain unavailable, including explicit projectTrusted=true", "future resource/package/proxy activation remains an explicit Capability Stub; other future settings are data only", "built-in Provider support remains the existing DeepSeek runtime; broader Provider adapters and model catalogs stay deferred"}},
		Deviation: &catalog.Deviation{ADR: "docs/adr/0010-trust-and-host-security.md", Reason: "This global-only slice does not inspect project settings, even to discover trust or Session paths."},
		Notes:     "Issue #74 implements global settings for Headless sessions. Go setters complete writes synchronously under a directory lock; Flush waits for concurrent setters and DrainErrors reports scoped failures. Overrides reset on save/reload. Pig reads only Pig-owned global state and never starts analytics, package installation, extensions, or proxy services from stored data.",
	}
}
func issue74SettingsEvidence(t *testing.T) []issue32ModuleEvidenceDescriptor {
	t.Helper()
	var descriptors []issue32ModuleEvidenceDescriptor
	for _, item := range []struct{ name, ref, run string }{{"global-settings", "internal/parity/global_settings_test.go#TestGlobalSettingsParity", "go test ./internal/parity -run '^TestGlobalSettingsParity$' -count=1"}, {"settings-startup", "cmd/pig/issue74_process_test.go#TestPigGlobalSettingsStartupParity", "go test ./cmd/pig -run '^TestPigGlobalSettingsStartupParity$' -count=1"}} {
		path := "parity/oracle/fixtures/" + item.name + ".json"
		for _, kind := range []string{catalog.MatrixEvidenceOracle, catalog.MatrixEvidenceGoTest} {
			ref, run := item.ref, item.run
			if kind == catalog.MatrixEvidenceOracle {
				ref = path
				run = "node --experimental-strip-types parity/oracle/" + item.name + ".mjs <locked-pi-checkout> --check"
			}
			descriptors = append(descriptors, issue32ModuleEvidenceDescriptor{InputPath: path, Evidence: catalog.Evidence{Kind: kind, Ref: ref, Baseline: issue32BaselineCommit, CaseID: "issue74-" + item.name, ExecutionMethod: run, Expected: "global settings and Headless startup observations match the fixed Pi baseline without semantic normalization", Actual: "PASS; the committed deterministic fixture reproduces and the public Pig boundary matches", Platform: "any", CatalogID: issue74SettingsCatalogID}})
		}
	}
	descriptors = append(descriptors, issue32ModuleEvidenceDescriptor{InputPath: "codingagent/issue74_settings_test.go", Evidence: catalog.Evidence{Kind: "go-test", Ref: "codingagent/issue74_settings_test.go#TestSettingsConcurrentProcessesPreserveUnrelatedFields", Baseline: issue32BaselineCommit, CaseID: "issue74-settings-concurrency-errors", ExecutionMethod: "go test -race ./codingagent -run '^TestSettings' -count=1", Expected: "cross-process writers retain unrelated fields; corrupt data, lock contention, write errors, sparse values and ownership remain observable", Actual: "PASS; public SettingsManager tests preserve data and expose errors without project I/O", Platform: "any", CatalogID: issue74SettingsCatalogID}})
	return descriptors
}
