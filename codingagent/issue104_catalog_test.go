package codingagent_test

import (
	"strings"
	"testing"

	"github.com/nankedr/pig/internal/catalog"
)

const issue104ThemeCatalogID = "contract:codingagent/themes"

func issue104ThemeCatalogEntry() catalog.Entry {
	return catalog.Entry{SchemaVersion: catalog.SchemaVersion, ID: issue104ThemeCatalogID, Upstream: catalog.Upstream{Module: "coding-agent", Repository: "https://github.com/badlogic/pi-mono", Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/modes/interactive/theme/theme.ts"}, Mapping: catalog.Mapping{Module: "codingagent", Target: issue32GoPackage + ".DefaultResourceLoader.GetThemes", Kind: "contract"}, Status: catalog.StatusPartial, Milestone: "M5", Classification: "public-api", Partial: &catalog.Partial{Supported: []string{
		"Issue #104: regular local theme JSON validation, variable chains/cycles, required colors and optional thinkingMax/scrollbarThumb fallbacks; embedded locked dark/light assets",
		"Public SDK foreground/background ANSI in truecolor and 256color, styles, thinking borders, resolved CSS and explicit export colors match locked Pi fixtures",
		"Global/trusted project/settings/explicit resource paths, noThemes with explicit paths, source information, auto hidden/ignore-file filtering and first-name conflict diagnostics; owned reload snapshots and first-load trust",
		"CLI real subprocess, AgentSession and file SDK export use validated local theme colors with Pi builtin-name precedence; Chromium checks Pi HTML CSS, offline rendering and XSS defenses",
	}, Unsupported: []string{
		"Package manifests/registration, npm/git, dependencies and lifecycle remain deferred under #99; opaque ThemesOverride and ExtendResources remain M7 Capability Stubs",
		"Interactive UI, theme dialogs, terminal auto-detection, global theme registry/watchers, general TUI adapters and direct Theme constructor are not frozen; full minimatch extglob and six-platform runtime parity remain unverified",
		"Unknown color keys are ignored and CSS hex is strictly six hexadecimal digits; malformed theme error text is diagnostic rather than byte-for-byte TypeBox parity",
	}}, Deviation: &catalog.Deviation{ADR: "docs/adr/0010-trust-and-host-security.md", Reason: "Project settings and themes are read only after trust; explicit paths are independently authorized. Special files are skipped. Strict color validation also covers export colors to prevent CSS/HTML injection."}}
}

func issue104ThemeEvidence(t *testing.T) []issue32ModuleEvidenceDescriptor {
	t.Helper()
	out := []issue32ModuleEvidenceDescriptor{}
	for _, e := range []struct{ kind, path, run string }{
		{"oracle", "parity/oracle/fixtures/themes.json", "node --experimental-strip-types parity/oracle/themes.mjs <locked-pi-checkout> --check"},
		{"go-test", "codingagent/issue104_themes_test.go", "go test ./codingagent -run '^TestThemes(ColorsSDKParity|ResourceSDKParity|SelectionAndHTML)$' -count=1"},
		{"go-test", "codingagent/issue104_trust_test.go", "go test ./codingagent -run '^TestThemes(TrustReloadAndOwnership|RejectCSSInjection|SessionExport)$' -count=1"},
		{"go-test", "cmd/pig/issue104_themes_test.go", "go test ./cmd/pig -run '^TestPigThemes(HTMLParity|ProjectTrustAndDiagnostics)$' -count=1"},
		{"go-test", "codingagent/issue104_surface_test.go", "go test ./codingagent -run '^TestIssue104ThemeAPISnapshot$' -count=1"},
		{"manual", "parity/export-html/check.mjs", "make m4-html-browser"},
		{"manual", "examples/themes/main.go", "go run ./examples/themes"},
	} {
		out = append(out, issue32ModuleEvidenceDescriptor{InputPath: e.path, Evidence: catalog.Evidence{Kind: e.kind, Ref: e.path, Baseline: issue32BaselineCommit, CaseID: "issue104-" + e.path, ExecutionMethod: e.run, Expected: "locked Pi theme colors, discovery, sources and HTML CSS", Actual: "PASS; public SDK and real CLI plus Chromium theme/XSS verification", Platform: "darwin/linux", CatalogID: issue104ThemeCatalogID}})
	}
	return out
}

func issue104PromoteRuntimeEntry(e *catalog.Entry) bool {
	_, member, _ := strings.Cut(e.ID, "#")
	matches := strings.HasSuffix(e.ID, "theme.ts#Theme") && strings.HasPrefix(e.ID, "symbol:")
	for _, name := range []string{"DefaultResourceLoader.getThemes", "DefaultResourceLoaderOptions.additionalThemePaths", "DefaultResourceLoaderOptions.noThemes", "Theme.name", "Theme.sourcePath", "Theme.sourceInfo", "Theme.fg", "Theme.bg", "Theme.bold", "Theme.italic", "Theme.underline", "Theme.inverse", "Theme.strikethrough", "Theme.getFgAnsi", "Theme.getBgAnsi", "Theme.getColorMode", "Theme.getThinkingBorderColor", "Theme.getBashModeBorderColor"} {
		matches = matches || member == name
	}
	if !matches {
		return false
	}
	e.Status = catalog.StatusPartial
	e.Partial = issue104ThemeCatalogEntry().Partial
	e.Evidence = []catalog.Evidence{{Kind: "go-test", Ref: "codingagent/issue104_themes_test.go", Baseline: issue32BaselineCommit, CaseID: e.ID, InputHash: issue104ThemeFixtureHash, ExecutionMethod: "go test ./codingagent -run '^TestThemes(ColorsSDKParity|ResourceSDKParity)$' -count=1", Expected: "locked Pi theme colors, metadata and diagnostics", Actual: "PASS; public SDK theme queries and styles match Pi", Platform: "darwin/linux", CatalogID: e.ID}}
	e.Notes = "Issue #104 local Theme behavior; exact scope and evidence: " + issue104ThemeCatalogID + "."
	return true
}

const issue104ThemeFixtureHash = "sha256:278d6fc07b2589041dd630a266f5b777d094854104903ffd234a4cf00a78f6dc"
