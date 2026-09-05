package codingagent_test

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/nankedr/pig/internal/catalog"
	"github.com/nankedr/pig/internal/surface"
)

var updateIssue32Catalog = flag.Bool("update-issue32-catalog", false, "regenerate the issue #32 Parity Catalog rows")

const (
	issue32BaselineCommit      = "936aff00918de1187f085f123c2812d8f2d67745"
	issue32SurfaceHash         = "sha256:878278ceb284bc0b2489a5dee57b63622c75827e1c3d1b833af32d980192852f"
	issue32GoPackage           = "github.com/nankedr/pig/codingagent"
	issue32TUIPackage          = "github.com/nankedr/pig/tui"
	issue32ModuleCatalogID     = "module-codingagent"
	issue32AgentSessionID      = "contract:codingagent/agent-session"
	issue32ReadToolCatalogID   = "contract:codingagent/read-tool"
	issue32CompactionCatalogID = "contract:codingagent/compaction"
	issue32TranscriptCatalogID = "contract:codingagent/transcript-projection"
	issue32V3JSONLCatalogID    = "contract:session/v3-jsonl"
	issue32MigrationCatalogID  = "contract:session/migration"
	issue32MemberTestRef       = "codingagent/issue32_surface_test.go#TestIssue32ScaffoldedMemberTargetsResolve"
	issue32MemberTestRun       = "go test ./codingagent -run '^(TestIssue32MemberMappingsMatchLockedCodingAgentSurface|TestIssue32ScaffoldedMemberTargetsResolve|TestIssue32InheritedTUIMemberProjectionsResolve)$' -count=1"
	issue32StaticMemberTestRef = "codingagent/issue32_surface_test.go#TestIssue32ScaffoldedStaticMemberTargetsResolve"
	issue32StaticMemberTestRun = "go test ./codingagent -run '^(TestIssue32MemberMappingsMatchLockedCodingAgentSurface|TestIssue32ScaffoldedStaticMemberTargetsResolve)$' -count=1"
)

// These Harness-origin declarations are intentionally shared with the
// production Coding Agent path. Every other root agent selector mapped from
// the pinned Harness catalog remains forbidden there.
var issue32AllowedHarnessAgentSelectors = map[string]struct{}{
	"BashExecutionMessage":           {},
	"BranchSummaryMessage":           {},
	"CalculateContextTokens":         {},
	"CompactionSettings":             {},
	"CompactionSummaryMessage":       {},
	"ConvertToLLM":                   {},
	"CreateBranchSummaryMessage":     {},
	"CreateCompactionSummaryMessage": {},
	"CreateCustomMessage":            {},
	"CustomMessage":                  {},
	"DefaultMaxBytes":                {},
	"DefaultMaxLines":                {},
	"DefaultCompactionSettings":      {},
	"EstimateTokens":                 {},
	"FormatSize":                     {},
	"ShouldCompact":                  {},
	"TruncateHead":                   {},
	"TruncateLine":                   {},
	"TruncateTail":                   {},
	"TruncationOptions":              {},
	"TruncationResult":               {},
}

func TestIssue32MappingsMatchLockedCodingAgentSurface(t *testing.T) {
	symbols := issue32Symbols(t)
	if len(symbols) != 376 {
		t.Fatalf("locked Coding Agent surface count = %d, want 376", len(symbols))
	}
	want := make(map[string]surface.Symbol, len(symbols))
	for _, symbol := range symbols {
		want[symbol.ID] = symbol
	}
	entries := issue32CatalogSymbolEntries(t)
	if len(entries) != len(want) {
		t.Fatalf("catalog issue #32 symbol row count = %d, want %d", len(entries), len(want))
	}
	for _, entry := range entries {
		symbol, ok := want[entry.ID]
		if !ok {
			t.Errorf("catalog mapping is outside locked Coding Agent surface: %s", entry.ID)
			continue
		}
		if entry.Upstream.Reference != symbol.Upstream.Reference {
			t.Errorf("%s upstream reference = %q, want %q", entry.ID, entry.Upstream.Reference, symbol.Upstream.Reference)
		}
		if entry.Mapping.Module != "codingagent" || entry.Mapping.Kind != "symbol" || entry.Mapping.Target != issue32SymbolTarget(symbol) {
			t.Errorf("%s has wrong mapping: %+v", entry.ID, entry.Mapping)
		}
		switch entry.Status {
		case catalog.StatusScaffolded, catalog.StatusPartial, catalog.StatusImplemented, catalog.StatusVerified:
		default:
			t.Errorf("%s has inactive Capability Status %q", entry.ID, entry.Status)
		}
		delete(want, entry.ID)
	}
	for id := range want {
		t.Errorf("locked Coding Agent symbol has no Catalog mapping: %s", id)
	}
}

func TestIssue32ExportSubpathsUseCanonicalGoPackage(t *testing.T) {
	counts := map[string]int{}
	members := map[string]int{}
	staticMembers := map[string]int{}
	for _, symbol := range issue32Symbols(t) {
		if target := issue32SymbolTarget(symbol); !strings.HasPrefix(target, issue32GoPackage+".") {
			t.Errorf("%s does not use canonical Go Coding Agent package: %s", symbol.ID, target)
		}
		for _, subpath := range symbol.ExportSubpaths {
			counts[subpath]++
			members[subpath] += len(symbol.Members)
			staticMembers[subpath] += len(symbol.StaticMembers)
		}
	}
	if want := (map[string]int{".": 365, "./client": 11}); !reflect.DeepEqual(counts, want) {
		t.Fatalf("Coding Agent export-subpath symbol counts = %v, want %v", counts, want)
	}
	if want := (map[string]int{".": 2092, "./client": 80}); !reflect.DeepEqual(members, want) {
		t.Fatalf("Coding Agent export-subpath instance/type member counts = %v, want %v", members, want)
	}
	if want := (map[string]int{".": 12, "./client": 2}); !reflect.DeepEqual(staticMembers, want) {
		t.Fatalf("Coding Agent export-subpath static member counts = %v, want %v", staticMembers, want)
	}
}

func TestIssue32ProductionV3AndHarnessV4StaySeparate(t *testing.T) {
	seenAgentSession := false
	for _, symbol := range issue32Symbols(t) {
		target := issue32SymbolTarget(symbol)
		if strings.Contains(target, "AgentHarness") || strings.Contains(target, "SessionV4") || strings.Contains(target, "HarnessSession") {
			t.Errorf("production Coding Agent symbol crosses into Harness v4: %s -> %s", symbol.ID, target)
		}
		if symbol.Name == "AgentSession" {
			seenAgentSession = true
			if target != issue32GoPackage+".AgentSession" {
				t.Errorf("AgentSession target = %s, want production v3 Coding Agent carrier", target)
			}
		}
	}
	if !seenAgentSession {
		t.Fatal("locked Coding Agent surface has no production AgentSession")
	}

	root := issue32RepoRoot(t)
	forbidden := issue32ForbiddenAgentSelectors(t)

	if err := issue32CheckProductionAgentSelectors(root, forbidden, func(path string, line int, qualifier, selector string) {
		t.Errorf("%s:%d selects Harness-v4-only agent declaration %s; production codingagent must remain on legacy Agent + v3 Session", path, line, issue32AgentSelectorName(qualifier, selector))
	}); err != nil {
		t.Fatal(err)
	}

	t.Run("scanner distinguishes aliases, shared selectors, and local shadows", func(t *testing.T) {
		fixtureRoot := t.TempDir()
		dir := filepath.Join(fixtureRoot, "codingagent")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		source := `package codingagent

import coreagent "github.com/nankedr/pig/agent"

var legacy *coreagent.Agent
var shared coreagent.CompactionSettings
var harness *coreagent.AgentHarness
var session *coreagent.Session

func locallyShadowed(coreagent struct{ Session int }) {
	_ = coreagent.Session
}
`
		if err := os.WriteFile(filepath.Join(dir, "aliases.go"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		var got []string
		if err := issue32CheckProductionAgentSelectors(fixtureRoot, forbidden, func(path string, line int, qualifier, selector string) {
			got = append(got, fmt.Sprintf("%s:%d:%s", path, line, issue32AgentSelectorName(qualifier, selector)))
		}); err != nil {
			t.Fatal(err)
		}
		if want := []string{"codingagent/aliases.go:7:coreagent.AgentHarness", "codingagent/aliases.go:8:coreagent.Session"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("reported selectors = %v, want %v", got, want)
		}
	})

	t.Run("scanner rejects dot imports", func(t *testing.T) {
		fixtureRoot := t.TempDir()
		dir := filepath.Join(fixtureRoot, "codingagent")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		source := `package codingagent

import . "github.com/nankedr/pig/agent"
`
		if err := os.WriteFile(filepath.Join(dir, "dot_import.go"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		var got []string
		if err := issue32CheckProductionAgentSelectors(fixtureRoot, forbidden, func(path string, line int, qualifier, selector string) {
			got = append(got, fmt.Sprintf("%s:%d:%s", path, line, issue32AgentSelectorName(qualifier, selector)))
		}); err != nil {
			t.Fatal(err)
		}
		if want := []string{"codingagent/dot_import.go:3:.<dot-import>"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("reported selectors = %v, want %v", got, want)
		}
	})
}

func issue32AgentSelectorName(qualifier, selector string) string {
	if qualifier == "." {
		return qualifier + selector
	}
	return qualifier + "." + selector
}

func issue32ForbiddenAgentSelectors(t *testing.T) map[string]struct{} {
	t.Helper()
	const (
		harnessReferencePrefix = "packages/agent/src/harness/"
		rootTargetPrefix       = "github.com/nankedr/pig/agent."
		wantSourceRows         = 265
	)

	selectors := map[string]struct{}{}
	sourceRows := 0
	for _, entry := range issue32AllCatalogEntries(t) {
		if entry.Mapping.Module != "agent" || entry.Mapping.Kind != "symbol" || !strings.HasPrefix(entry.Upstream.Reference, harnessReferencePrefix) {
			continue
		}
		sourceRows++
		if entry.Upstream.Module != "agent" {
			t.Errorf("Harness symbol row %s upstream module = %q, want agent", entry.ID, entry.Upstream.Module)
		}
		if entry.Mapping.Target == "" {
			t.Errorf("Harness symbol row %s has an empty Go target", entry.ID)
			continue
		}
		switch entry.Status {
		case catalog.StatusScaffolded, catalog.StatusPartial, catalog.StatusImplemented, catalog.StatusVerified:
		default:
			t.Errorf("Harness symbol row %s has inactive Capability Status %q", entry.ID, entry.Status)
		}
		if !strings.HasPrefix(entry.Mapping.Target, rootTargetPrefix) {
			continue
		}
		selector := strings.TrimPrefix(entry.Mapping.Target, rootTargetPrefix)
		if selector == "" || strings.Contains(selector, ".") {
			t.Errorf("Harness symbol row %s has invalid root agent selector target %q", entry.ID, entry.Mapping.Target)
			continue
		}
		if _, duplicate := selectors[selector]; duplicate {
			t.Errorf("Harness symbol rows map duplicate root agent selector %q", selector)
			continue
		}
		selectors[selector] = struct{}{}
	}
	if sourceRows != wantSourceRows {
		t.Fatalf("locked Harness catalog source row count = %d, want %d", sourceRows, wantSourceRows)
	}
	for selector := range issue32AllowedHarnessAgentSelectors {
		if _, ok := selectors[selector]; !ok {
			t.Errorf("allowed production selector %q is not backed by a pinned Harness symbol row", selector)
			continue
		}
		delete(selectors, selector)
	}
	return selectors
}

func issue32CheckProductionAgentSelectors(root string, forbidden map[string]struct{}, report func(path string, line int, qualifier, selector string)) error {
	packageDir := filepath.Join(root, "codingagent")
	return filepath.WalkDir(packageDir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}

		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		agentQualifiers := map[string]struct{}{}
		for _, spec := range file.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return fmt.Errorf("unquote %s import %s: %w", path, spec.Path.Value, err)
			}
			if importPath != "github.com/nankedr/pig/agent" {
				continue
			}
			qualifier := "agent"
			if spec.Name != nil {
				qualifier = spec.Name.Name
			}
			if qualifier == "_" {
				continue
			}
			if qualifier == "." {
				position := fset.Position(spec.Pos())
				relative, relErr := filepath.Rel(root, position.Filename)
				if relErr != nil {
					return relErr
				}
				report(filepath.ToSlash(relative), position.Line, qualifier, "<dot-import>")
				continue
			}
			agentQualifiers[qualifier] = struct{}{}
		}

		ast.Inspect(file, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			qualifier, ok := selector.X.(*ast.Ident)
			if !ok || qualifier.Obj != nil {
				return true
			}
			if _, ok := agentQualifiers[qualifier.Name]; !ok {
				return true
			}
			if _, forbidden := forbidden[selector.Sel.Name]; !forbidden {
				return true
			}
			position := fset.Position(selector.Pos())
			relative, err := filepath.Rel(root, position.Filename)
			if err != nil {
				relative = position.Filename
			}
			report(filepath.ToSlash(relative), position.Line, qualifier.Name, selector.Sel.Name)
			return true
		})
		return nil
	})
}

func TestIssue32CodingAgentDependencyPolicy(t *testing.T) {
	root := issue32RepoRoot(t)
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, filepath.Join(root, "codingagent"), func(info os.FileInfo) bool { return !strings.HasSuffix(info.Name(), "_test.go") }, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{
		"github.com/nankedr/pig/ai": true, "github.com/nankedr/pig/agent": true,
		"github.com/nankedr/pig/tui": true, "github.com/nankedr/pig/protocol": true,
		"github.com/nankedr/pig/client": true, "github.com/nankedr/pig/internal/capability": true,
	}
	for _, pkg := range pkgs {
		for name, file := range pkg.Files {
			for _, spec := range file.Imports {
				path, err := strconv.Unquote(spec.Path.Value)
				if err != nil {
					t.Fatal(err)
				}
				if strings.HasPrefix(path, "github.com/nankedr/pig/") && !allowed[path] {
					t.Errorf("%s imports disallowed Pig package %q; codingagent may depend only on established lower layers", name, path)
				}
			}
		}
	}
}

func TestIssue32BroaderContractHasSurfaceAndSideEffectEvidence(t *testing.T) {
	entries := issue32AllCatalogEntries(t)
	entry, ok := issue32EntryByID(entries, issue32ModuleCatalogID)
	if !ok {
		t.Fatal("missing module-codingagent")
	}
	{
		if entry.Mapping.Module != "codingagent" || entry.Mapping.Kind != "package" || entry.Mapping.Target != issue32GoPackage {
			t.Fatalf("Coding Agent module mapping = %+v, want canonical Go package", entry.Mapping)
		}
		if entry.Status != catalog.StatusPartial {
			t.Fatalf("Coding Agent module status = %q, want partial after issue #32 scaffold", entry.Status)
		}
		wantEvidence := issue32ModuleEvidence(t)
		if !reflect.DeepEqual(entry.Evidence, wantEvidence) {
			t.Errorf("module-codingagent evidence = %+v, want claim-specific replay records %+v", entry.Evidence, wantEvidence)
		}
		if entry.Partial == nil || len(entry.Partial.Supported) == 0 || len(entry.Partial.Unsupported) == 0 {
			t.Errorf("module-codingagent must describe supported and unsupported branches: %+v", entry.Partial)
		}
		for _, phrase := range []string{"376", "2,172", "14 static", "38 constructors", "legacy", "v3", "Harness v4", ".pig", "credential", "side effects"} {
			if !strings.Contains(entry.Notes, phrase) {
				t.Errorf("module-codingagent notes do not mention %q: %q", phrase, entry.Notes)
			}
		}
	}
	for _, want := range issue32BehaviorOwnerEntries(t) {
		got, ok := issue32EntryByID(entries, want.ID)
		if !ok {
			t.Errorf("missing behavior owner %s", want.ID)
			continue
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("behavior owner %s mismatch\n got: %+v\nwant: %+v", want.ID, got, want)
		}
	}
}

func TestIssue32CatalogPromotionWritesCompleteClaimSpecificEvidence(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "parity")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	sentinel := func(id, marker string) catalog.Entry {
		return catalog.Entry{
			SchemaVersion:  catalog.SchemaVersion,
			ID:             id,
			Upstream:       catalog.Upstream{Module: marker, Repository: "https://example.invalid/" + marker, Commit: marker + "-commit", Reference: marker + "/reference"},
			Mapping:        catalog.Mapping{Module: marker, Target: "example.invalid/" + marker, Kind: "package"},
			Status:         catalog.StatusPartial,
			Milestone:      "M0",
			Classification: "public-api",
			Evidence:       []catalog.Evidence{{Kind: "go-test", Ref: marker + "_test.go#TestSentinel", Baseline: marker + "-commit", CaseID: marker + "-case", InputHash: "sha256:" + strings.Repeat("0", 64), ExecutionMethod: "go test ./" + marker, Expected: marker + " expected", Actual: marker + " actual", Platform: "any", CatalogID: id}},
			Partial:        &catalog.Partial{Supported: []string{marker + " supported"}, Unsupported: []string{marker + " unsupported"}},
			Notes:          marker + " sentinel must remain unchanged",
		}
	}
	before := sentinel("aaa-unrelated-before", "before")
	after := sentinel("zzz-unrelated-after", "after")
	staleModule := catalog.Entry{
		SchemaVersion:  catalog.SchemaVersion,
		ID:             issue32ModuleCatalogID,
		Upstream:       catalog.Upstream{Module: "coding-agent", Repository: "https://github.com/badlogic/pi-mono", Commit: issue32BaselineCommit, Reference: "coding-agent"},
		Mapping:        catalog.Mapping{Module: "codingagent", Target: issue32GoPackage, Kind: "package"},
		Status:         catalog.StatusScaffolded,
		Milestone:      "M3",
		Classification: "public-api",
		Evidence:       []catalog.Evidence{{Kind: "go-test", Ref: "stale_test.go", Baseline: issue32BaselineCommit}},
		Partial:        &catalog.Partial{Supported: []string{"stale supported"}, Unsupported: []string{"stale unsupported"}},
		Notes:          "stale module row",
	}
	staleOwned := catalog.Entry{
		SchemaVersion:  catalog.SchemaVersion,
		ID:             "symbol:codingagent/stale.ts#Stale",
		Upstream:       catalog.Upstream{Module: "coding-agent", Repository: "https://example.invalid/stale", Commit: "stale-commit", Reference: "stale/reference"},
		Mapping:        catalog.Mapping{Module: "codingagent", Target: issue32GoPackage + ".Stale", Kind: "symbol"},
		Status:         catalog.StatusInventoried,
		Milestone:      "M3",
		Classification: "public-api",
		Notes:          "stale generated issue #32 row",
	}
	replacement := staleOwned
	replacement.Upstream.Commit = issue32BaselineCommit
	replacement.Mapping.Target = issue32GoPackage + ".Replacement"
	replacement.Status = catalog.StatusScaffolded
	replacement.Notes = "replacement generated issue #32 row"

	staleBehaviorOwners := issue32BehaviorOwnerEntries(t)
	for index := range staleBehaviorOwners {
		staleBehaviorOwners[index].Status = catalog.StatusInventoried
		staleBehaviorOwners[index].Evidence = []catalog.Evidence{{Kind: "go-test", Ref: "stale_test.go", Baseline: issue32BaselineCommit}}
		staleBehaviorOwners[index].Partial = nil
		staleBehaviorOwners[index].Notes = "stale behavior owner"
	}
	seed := []catalog.Entry{before, staleModule, staleOwned, after}
	seed = append(seed, staleBehaviorOwners...)
	data, err := catalog.EncodeEntries(seed)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "catalog.jsonl"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	issue32WriteCatalog(t, root, []catalog.Entry{replacement})
	entries, err := catalog.LoadCatalog(filepath.Join(dir, "catalog.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 4 + len(staleBehaviorOwners) {
		t.Fatalf("written catalog entry count = %d, want %d", len(entries), 4 + len(staleBehaviorOwners))
	}
	byID := make(map[string]catalog.Entry, len(entries))
	for _, entry := range entries {
		if _, duplicate := byID[entry.ID]; duplicate {
			t.Fatalf("written catalog contains duplicate ID %q", entry.ID)
		}
		byID[entry.ID] = entry
	}
	for _, unchanged := range []catalog.Entry{before, after} {
		if got, ok := byID[unchanged.ID]; !ok {
			t.Errorf("unrelated catalog entry %q was dropped", unchanged.ID)
		} else if !reflect.DeepEqual(got, unchanged) {
			t.Errorf("unrelated catalog entry %q changed\n got: %+v\nwant: %+v", unchanged.ID, got, unchanged)
		}
	}
	if got := byID[replacement.ID]; !reflect.DeepEqual(got, replacement) {
		t.Errorf("owned issue #32 row was not replaced\n got: %+v\nwant: %+v", got, replacement)
	}

	entry, ok := byID[issue32ModuleCatalogID]
	if !ok {
		t.Fatal("written catalog lacks module-codingagent")
	}
	wantModule := staleModule
	wantModule.Status = catalog.StatusPartial
	wantModule.Evidence = issue32ModuleEvidence(t)
	wantModule.Partial = issue32ModulePartial()
	wantModule.Notes = issue32ModuleNotes()
	if !reflect.DeepEqual(entry, wantModule) {
		t.Errorf("module-codingagent was not replaced field-for-field\n got: %+v\nwant: %+v", entry, wantModule)
	}

	for _, want := range issue32BehaviorOwnerEntries(t) {
		got, ok := byID[want.ID]
		if !ok {
			t.Errorf("written catalog lacks behavior owner %s", want.ID)
			continue
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("behavior owner %s was not replaced field-for-field\n got: %+v\nwant: %+v", want.ID, got, want)
		}
	}

	descriptors := issue32ModuleEvidenceDescriptors(t)
	wantEvidence := issue32ModuleEvidence(t)
	wantByCase := make(map[string]issue32ModuleEvidenceDescriptor, len(descriptors))
	for _, descriptor := range descriptors {
		caseID := descriptor.Evidence.CaseID
		if _, duplicate := wantByCase[caseID]; duplicate {
			t.Fatalf("module evidence descriptor has duplicate case ID %q", caseID)
		}
		wantByCase[caseID] = descriptor
	}
	if !reflect.DeepEqual(entry.Evidence, wantEvidence) {
		t.Errorf("written module evidence = %+v, want %+v", entry.Evidence, wantEvidence)
	}
	if len(entry.Evidence) != len(descriptors) {
		t.Fatalf("written module evidence count = %d, want %d", len(entry.Evidence), len(wantEvidence))
	}
	seen := map[string]bool{}
	for _, evidence := range entry.Evidence {
		if evidence.Kind == "" || evidence.Ref == "" || evidence.Baseline == "" || evidence.CaseID == "" ||
			evidence.InputHash == "" || evidence.ExecutionMethod == "" || evidence.Expected == "" ||
			evidence.Actual == "" || evidence.Platform == "" || evidence.CatalogID == "" {
			t.Errorf("evidence %q has an incomplete replay record: %+v", evidence.CaseID, evidence)
		}
		descriptor, ok := wantByCase[evidence.CaseID]
		if !ok {
			t.Errorf("unexpected evidence case ID %q", evidence.CaseID)
			continue
		} else if evidence != descriptor.Evidence {
			t.Errorf("evidence %q = %+v, want %+v", evidence.CaseID, evidence, descriptor.Evidence)
		}
		if seen[evidence.CaseID] {
			t.Errorf("duplicate evidence case ID %q", evidence.CaseID)
		}
		seen[evidence.CaseID] = true
		if evidence.Baseline != issue32BaselineCommit {
			t.Errorf("evidence %q baseline = %q, want %q", evidence.CaseID, evidence.Baseline, issue32BaselineCommit)
		}
		if evidence.CatalogID != issue32ModuleCatalogID {
			t.Errorf("evidence %q catalog ID = %q, want %q", evidence.CaseID, evidence.CatalogID, issue32ModuleCatalogID)
		}
		if descriptor.InputPath == "" {
			t.Errorf("evidence %q has no declared input path", evidence.CaseID)
			continue
		}
		wantHash := issue32FileHash(t, descriptor.InputPath)
		if descriptor.PinnedInputHash != "" {
			wantHash = descriptor.PinnedInputHash
		}
		if evidence.InputHash != wantHash {
			t.Errorf("evidence %q input hash = %q, want declared input %s hash %q", evidence.CaseID, evidence.InputHash, descriptor.InputPath, wantHash)
		}
	}
	for caseID := range wantByCase {
		if !seen[caseID] {
			t.Errorf("module evidence descriptor %q was not written", caseID)
		}
	}

	for _, owner := range issue32BehaviorOwnerEntries(t) {
		if owner.Status == catalog.StatusPartial && (owner.Partial == nil || len(owner.Partial.Supported) == 0 || len(owner.Partial.Unsupported) == 0) {
			t.Errorf("partial behavior owner %s lacks supported/unsupported accounting: %+v", owner.ID, owner.Partial)
		}
		if owner.Status == catalog.StatusImplemented && owner.Partial != nil {
			t.Errorf("implemented behavior owner %s retains a partial block: %+v", owner.ID, owner.Partial)
		}
		issue32ValidateEvidenceDescriptors(t, owner.ID, issue32BehaviorEvidenceDescriptors(t, owner.ID), owner.Evidence)
	}
}

func TestIssue32MemberMappingsMatchLockedCodingAgentSurface(t *testing.T) {
	root := issue32RepoRoot(t)
	surfacePath := filepath.Join(root, "parity", "surface", "symbols.jsonl")
	data, err := os.ReadFile(surfacePath)
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("sha256:%x", sha256.Sum256(data)); got != issue32SurfaceHash {
		t.Fatalf("locked surface hash = %s, want %s", got, issue32SurfaceHash)
	}
	expected, err := issue32ExpectedCatalogEntries(issue32Symbols(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(expected) != 2600 {
		t.Fatalf("issue #32 expected catalog rows = %d, want 2600 (376 symbols plus 2,172 instance/type members plus 14 static members plus 38 constructors)", len(expected))
	}
	if got := issue32CountKind(expected, "symbol"); got != 376 {
		t.Fatalf("issue #32 symbol rows = %d, want 376", got)
	}
	if got := issue32CountIDPrefix(expected, "member:codingagent/"); got != 2172 {
		t.Fatalf("issue #32 instance/type member rows = %d, want 2172", got)
	}
	if got := issue32CountIDPrefix(expected, "static-member:codingagent/"); got != 14 {
		t.Fatalf("issue #32 static member rows = %d, want 14", got)
	}
	if got := issue32CountIDPrefix(expected, "constructor:codingagent/"); got != 38 {
		t.Fatalf("issue #32 constructor rows = %d, want 38", got)
	}
	wantByMilestone := map[string]int{
		"M1":  249,
		"M3":  595,
		"M4":  218,
		"M5":  193,
		"M6":  492,
		"M7":  702,
		"M9":  93,
		"M11": 29,
		"M12": 23,
		"M13": 6,
	}
	gotByMilestone := make(map[string]int, len(wantByMilestone))
	gotByStatus := map[string]int{}
	for _, entry := range expected {
		gotByMilestone[entry.Milestone]++
		gotByStatus[entry.Status]++
	}
	if !reflect.DeepEqual(gotByMilestone, wantByMilestone) {
		t.Fatalf("issue #32 milestone row counts = %v, want %v", gotByMilestone, wantByMilestone)
	}
	if want := (map[string]int{catalog.StatusScaffolded: 1851, catalog.StatusInventoried: 744, catalog.StatusPartial: 2, catalog.StatusImplemented: 3}); !reflect.DeepEqual(gotByStatus, want) {
		t.Fatalf("issue #32 status row counts = %v, want %v", gotByStatus, want)
	}
	if *updateIssue32Catalog {
		issue32WriteCatalog(t, root, expected)
		return
	}
	got := issue32CatalogEntries(t)
	if len(got) != len(expected) {
		t.Fatalf("catalog issue #32 rows = %d, want %d", len(got), len(expected))
	}
	for i := range expected {
		if !reflect.DeepEqual(got[i], expected[i]) {
			t.Errorf("catalog row %s differs\n got: %+v\nwant: %+v", expected[i].ID, got[i], expected[i])
		}
	}
}

func TestIssue32ScaffoldedSymbolTargetsResolve(t *testing.T) {
	index, err := issue32LoadDecls(filepath.Join(issue32RepoRoot(t), "codingagent"))
	if err != nil {
		t.Fatal(err)
	}
	var missing []string
	for _, symbol := range issue32Symbols(t) {
		target := issue32SymbolTarget(symbol)
		name, err := issue32TargetName(target)
		if err != nil {
			t.Errorf("%s: %v", symbol.ID, err)
			continue
		}
		if index[name] == nil {
			missing = append(missing, fmt.Sprintf("%s -> %s", symbol.ID, target))
		}
	}
	if len(missing) != 0 {
		t.Fatalf("unresolved scaffolded symbol targets: %s", strings.Join(missing, ", "))
	}
}

func TestIssue32ScaffoldedMemberTargetsResolve(t *testing.T) {
	index, err := issue32LoadDecls(filepath.Join(issue32RepoRoot(t), "codingagent"))
	if err != nil {
		t.Fatal(err)
	}
	missingTypes, missingMembers := map[string]bool{}, map[string]bool{}
	entries, err := issue32ExpectedCatalogEntries(issue32Symbols(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Mapping.Kind != "contract" || entry.Status != catalog.StatusScaffolded {
			continue
		}
		if strings.HasPrefix(entry.ID, "static-member:") || strings.HasPrefix(entry.ID, "constructor:") {
			continue
		}
		typeName, member, err := issue32MemberTargetParts(entry.Mapping.Target)
		if err != nil {
			t.Errorf("%s: %v", entry.ID, err)
			continue
		}
		decl := index[typeName]
		if decl == nil {
			missingTypes[typeName] = true
			continue
		}
		if _, ok := decl.members[member]; !ok {
			missingMembers[typeName+"."+member] = true
		}
	}
	if len(missingTypes) != 0 || len(missingMembers) != 0 {
		t.Fatalf("unresolved scaffold targets: %d missing Go types, %d missing Go members; types=%v members=%v", len(missingTypes), len(missingMembers), issue32SortedKeys(missingTypes), issue32SortedKeys(missingMembers))
	}
}

func TestIssue32InheritedTUIMemberProjectionsResolve(t *testing.T) {
	index, err := issue32LoadDecls(filepath.Join(issue32RepoRoot(t), "tui"))
	if err != nil {
		t.Fatal(err)
	}
	entries, err := issue32ExpectedCatalogEntries(issue32Symbols(t))
	if err != nil {
		t.Fatal(err)
	}
	wantByCarrier := map[string]int{
		"Box":                18,
		"Container":          107,
		"Editor":             20,
		"KeybindingsManager": 7,
	}
	gotByCarrier := map[string]int{}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Mapping.Target, issue32TUIPackage+".") {
			continue
		}
		if entry.Mapping.Module != "codingagent" || entry.Mapping.Kind != "contract" || entry.Status != catalog.StatusInventoried {
			t.Errorf("%s has invalid inherited dependency projection: %+v", entry.ID, entry)
		}
		typeName, member, ok := strings.Cut(strings.TrimPrefix(entry.Mapping.Target, issue32TUIPackage+"."), ".")
		if !ok || typeName == "" || member == "" || strings.Contains(member, ".") {
			t.Errorf("%s has malformed TUI target %q", entry.ID, entry.Mapping.Target)
			continue
		}
		decl := index[typeName]
		if decl == nil {
			t.Errorf("%s maps to missing TUI carrier %s", entry.ID, typeName)
			continue
		}
		if _, ok := decl.members[member]; !ok {
			t.Errorf("%s maps to missing TUI member %s.%s", entry.ID, typeName, member)
		}
		gotByCarrier[typeName]++
	}
	if !reflect.DeepEqual(gotByCarrier, wantByCarrier) {
		t.Fatalf("inherited TUI projection counts = %v, want %v", gotByCarrier, wantByCarrier)
	}
}

func TestIssue32ScaffoldedStaticMemberTargetsResolve(t *testing.T) {
	index, err := issue32LoadDecls(filepath.Join(issue32RepoRoot(t), "codingagent"))
	if err != nil {
		t.Fatal(err)
	}
	missing, wrongKind := map[string]bool{}, map[string]bool{}
	resolved := 0
	entries, err := issue32ExpectedCatalogEntries(issue32Symbols(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.ID, "static-member:") || entry.Status != catalog.StatusScaffolded {
			continue
		}
		resolved++
		name, err := issue32TargetName(entry.Mapping.Target)
		if err != nil {
			t.Errorf("%s: %v", entry.ID, err)
			continue
		}
		decl := index[name]
		if decl == nil {
			missing[name] = true
			continue
		}
		if !decl.packageFunction {
			wrongKind[name] = true
		}
	}
	if resolved != 14 {
		t.Errorf("resolved Coding Agent static-member rows = %d, want 14", resolved)
	}
	if len(missing) != 0 || len(wrongKind) != 0 {
		t.Fatalf("unresolved scaffolded static targets: missing declarations=%v non-package-functions=%v", issue32SortedKeys(missing), issue32SortedKeys(wrongKind))
	}
}

type issue32GoDecl struct {
	constants       []string
	values          []string
	members         map[string]struct{}
	embeds          []string
	aliasTarget     string
	packageFunction bool
}

func issue32LoadDecls(dir string) (map[string]*issue32GoDecl, error) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(info os.FileInfo) bool { return !strings.HasSuffix(info.Name(), "_test.go") }, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	index := map[string]*issue32GoDecl{}
	declFor := func(name string) *issue32GoDecl {
		d := index[name]
		if d == nil {
			d = &issue32GoDecl{members: map[string]struct{}{}}
			index[name] = d
		}
		return d
	}
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, raw := range file.Decls {
				switch decl := raw.(type) {
				case *ast.GenDecl:
					for _, spec := range decl.Specs {
						for _, name := range issue32SpecNames(spec) {
							_, isType := spec.(*ast.TypeSpec)
							if !ast.IsExported(name) && !isType {
								continue
							}
							d := declFor(name)
							if ts, ok := spec.(*ast.TypeSpec); ok {
								if ts.Assign.IsValid() {
									d.aliasTarget = issue32QualifiedTypeName(ts.Type)
								}
								members, embeds := issue32TypeMembers(ts.Type)
								for _, member := range members {
									d.members[member] = struct{}{}
								}
								d.embeds = append(d.embeds, embeds...)
							}
						}
						if value, ok := spec.(*ast.ValueSpec); ok && value.Type != nil {
							if typeName := issue32ReceiverName(value.Type); typeName != "" {
								typed := declFor(typeName)
								typed.constants = append(typed.constants, value.Names[0].Name)
								for _, expression := range value.Values {
									literal, ok := expression.(*ast.BasicLit)
									if !ok || literal.Kind != token.STRING {
										continue
									}
									decoded, err := strconv.Unquote(literal.Value)
									if err != nil {
										return nil, err
									}
									typed.values = append(typed.values, decoded)
								}
							}
						}
					}
				case *ast.FuncDecl:
					if decl.Recv == nil {
						if ast.IsExported(decl.Name.Name) {
							d := declFor(decl.Name.Name)
							d.packageFunction = true
						}
						continue
					}
					receiver := issue32ReceiverName(decl.Recv.List[0].Type)
					if receiver != "" && ast.IsExported(decl.Name.Name) {
						d := declFor(receiver)
						d.members[decl.Name.Name] = struct{}{}
					}
				}
			}
		}
	}
	for _, d := range index {
		sort.Strings(d.constants)
		sort.Strings(d.values)
	}
	for typeName, alias := range issue32AliasMembers {
		d := index[typeName]
		if d == nil || d.aliasTarget != alias.target {
			continue
		}
		for _, member := range alias.members {
			d.members[member] = struct{}{}
		}
	}
	for changed := true; changed; {
		changed = false
		for _, d := range index {
			for _, embed := range d.embeds {
				if e := index[embed]; e != nil {
					for member := range e.members {
						if _, ok := d.members[member]; !ok {
							d.members[member] = struct{}{}
							changed = true
						}
					}
				}
			}
		}
	}
	return index, nil
}

func TestIssue32FiniteUnionValuesAreDeclared(t *testing.T) {
	index, err := issue32LoadDecls(filepath.Join(issue32RepoRoot(t), "codingagent"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"AppKeybinding",
		"CredentialSynchronizationOperation",
		"DefaultProjectTrust",
		"FullscreenExitOutput",
		"InputSource",
		"ProjectTrustEventDecision",
		"RemoteSessionOperation",
		"SlashCommandSource",
		"ThemeColor",
		"WidgetPlacement",
	} {
		decl := index[name]
		if decl == nil {
			t.Errorf("missing finite-union declaration %s", name)
			continue
		}
		if len(decl.constants) == 0 {
			t.Errorf("finite-union declaration %s has no typed constants", name)
		}
	}
}

func TestIssue32FiniteUnionValuesMatchPinnedBaseline(t *testing.T) {
	index, err := issue32LoadDecls(filepath.Join(issue32RepoRoot(t), "codingagent"))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"AppKeybinding": {
			"app.clear", "app.clipboard.pasteImage", "app.editor.external", "app.exit",
			"app.interrupt", "app.message.copy", "app.message.dequeue", "app.message.followUp",
			"app.model.cycleBackward", "app.model.cycleForward", "app.model.select",
			"app.models.clearAll", "app.models.enableAll", "app.models.reorderDown",
			"app.models.reorderUp", "app.models.save", "app.models.toggleProvider",
			"app.session.delete", "app.session.deleteNoninvasive", "app.session.fork",
			"app.session.new", "app.session.rename", "app.session.resume", "app.session.toggleNamedFilter",
			"app.session.togglePath", "app.session.toggleSort", "app.session.tree", "app.suspend",
			"app.thinking.cycle", "app.thinking.toggle", "app.tools.expand",
			"app.tree.editLabel", "app.tree.filter.all", "app.tree.filter.cycleBackward",
			"app.tree.filter.cycleForward", "app.tree.filter.default", "app.tree.filter.labeledOnly",
			"app.tree.filter.noTools", "app.tree.filter.userOnly", "app.tree.foldOrUp",
			"app.tree.toggleLabelTimestamp", "app.tree.unfoldOrDown",
		},
		"CredentialSynchronizationOperation": {"login", "logout", "removeRuntimeApiKey", "setRuntimeApiKey"},
		"DefaultProjectTrust":                {"always", "ask", "never"},
		"FullscreenExitOutput":               {"resume-hint", "transcript"},
		"InputSource":                        {"extension", "interactive", "rpc"},
		"ProjectTrustEventDecision":          {"no", "undecided", "yes"},
		"RemoteSessionOperation":             {"abort", "create", "open", "reconnect", "setModel", "setThinking", "submit"},
		"SlashCommandSource":                 {"extension", "prompt", "skill"},
		"ThemeColor": {
			"accent", "bashMode", "border", "borderAccent", "borderMuted", "customMessageLabel",
			"customMessageText", "dim", "error", "mdCode", "mdCodeBlock", "mdCodeBlockBorder",
			"mdHeading", "mdHr", "mdLink", "mdLinkUrl", "mdListBullet", "mdQuote",
			"mdQuoteBorder", "muted", "success", "syntaxComment", "syntaxFunction",
			"syntaxKeyword", "syntaxNumber", "syntaxOperator", "syntaxPunctuation", "syntaxString",
			"syntaxType", "syntaxVariable", "text", "thinkingHigh", "thinkingLow",
			"thinkingMax", "thinkingMedium", "thinkingMinimal", "thinkingOff", "thinkingText",
			"thinkingXhigh", "toolDiffAdded", "toolDiffContext", "toolDiffRemoved", "toolOutput",
			"toolTitle", "userMessageText", "warning",
		},
		"WidgetPlacement": {"aboveEditor", "belowEditor"},
	}
	for name, expected := range want {
		decl := index[name]
		if decl == nil {
			t.Errorf("missing finite-union declaration %s", name)
			continue
		}
		sort.Strings(expected)
		if !reflect.DeepEqual(decl.values, expected) {
			t.Errorf("%s values = %q, want %q", name, decl.values, expected)
		}
	}
}

func issue32SpecNames(spec ast.Spec) []string {
	switch s := spec.(type) {
	case *ast.TypeSpec:
		return []string{s.Name.Name}
	case *ast.ValueSpec:
		out := make([]string, len(s.Names))
		for i := range s.Names {
			out[i] = s.Names[i].Name
		}
		return out
	}
	return nil
}
func issue32TypeMembers(expr ast.Expr) (members, embeds []string) {
	switch e := expr.(type) {
	case *ast.StructType:
		for _, f := range e.Fields.List {
			if len(f.Names) == 0 {
				if n := issue32ReceiverName(f.Type); n != "" {
					embeds = append(embeds, n)
				}
			} else {
				for _, n := range f.Names {
					if ast.IsExported(n.Name) {
						members = append(members, n.Name)
					}
				}
			}
		}
	case *ast.InterfaceType:
		for _, f := range e.Methods.List {
			if len(f.Names) == 0 {
				if n := issue32ReceiverName(f.Type); n != "" {
					embeds = append(embeds, n)
				}
			} else {
				for _, n := range f.Names {
					if ast.IsExported(n.Name) {
						members = append(members, n.Name)
					}
				}
			}
		}
	default:
		if n := issue32ReceiverName(expr); n != "" {
			embeds = append(embeds, n)
		}
	}
	return
}
func issue32ReceiverName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.StarExpr:
		return issue32ReceiverName(e.X)
	case *ast.IndexExpr:
		return issue32ReceiverName(e.X)
	case *ast.IndexListExpr:
		return issue32ReceiverName(e.X)
	}
	return ""
}
func issue32QualifiedTypeName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		qualifier, ok := e.X.(*ast.Ident)
		if ok {
			return qualifier.Name + "." + e.Sel.Name
		}
	}
	return ""
}

const issue32ReferencePrefix = "packages/coding-agent/src/"

// issue32MilestoneOverrides records declarations whose first runnable slice
// differs from the primary capability of their source file. Children inherit
// the resolved parent milestone; mapped Go targets never influence ownership.
var issue32MilestoneOverrides = map[string]string{
	"config.ts#getDocsPath":                                   "M13",
	"config.ts#getExamplesPath":                               "M13",
	"config.ts#getPackageDir":                                 "M13",
	"config.ts#getReadmePath":                                 "M13",
	"core/agent-session.ts#SessionStats":                      "M4",
	"core/agent-session.ts#parseSkillBlock":                   "M5",
	"core/extensions/loader.ts#createExtensionRuntime":        "M7",
	"core/extensions/loader.ts#discoverAndLoadExtensions":     "M5",
	"core/session-manager.ts#BranchSummaryEntry":              "M4",
	"core/session-manager.ts#SessionTreeNode":                 "M4",
	"core/settings-manager.ts#CompactionSettings":             "M4",
	"core/settings-manager.ts#ImageSettings":                  "M12",
	"core/settings-manager.ts#PackageSource":                  "M5",
	"core/settings-manager.ts#RetrySettings":                  "M4",
	"core/settings-manager.ts#TuiMode":                        "M6",
	"core/trust-manager.ts#hasTrustRequiringProjectResources": "M5",
}

// issue32MilestonesByFile is deliberately closed over the pinned surface. A
// newly extracted file must be reviewed instead of silently inheriting M3.
var issue32MilestonesByFile = map[string]string{
	"cli/args.ts":                                            "M1",
	"client/remote-session.ts":                               "M9",
	"client/transcript.ts":                                   "M9",
	"config.ts":                                              "M3",
	"core/agent-session-runtime.ts":                          "M1",
	"core/agent-session-services.ts":                         "M1",
	"core/agent-session.ts":                                  "M1",
	"core/auth-storage.ts":                                   "M3",
	"core/compaction/branch-summarization.ts":                "M4",
	"core/compaction/compaction.ts":                          "M4",
	"core/compaction/utils.ts":                               "M4",
	"core/diagnostics.ts":                                    "M5",
	"core/event-bus.ts":                                      "M7",
	"core/exec.ts":                                           "M7",
	"core/extensions/runner.ts":                              "M7",
	"core/extensions/types.ts":                               "M7",
	"core/extensions/wrapper.ts":                             "M7",
	"core/footer-data-provider.ts":                           "M6",
	"core/keybindings.ts":                                    "M6",
	"core/messages.ts":                                       "M1",
	"core/model-registry.ts":                                 "M3",
	"core/model-resolver.ts":                                 "M3",
	"core/model-runtime.ts":                                  "M3",
	"core/package-manager.ts":                                "M5",
	"core/prompt-templates.ts":                               "M5",
	"core/resource-loader.ts":                                "M5",
	"core/sdk.ts":                                            "M1",
	"core/session-manager.ts":                                "M3",
	"core/settings-manager.ts":                               "M3",
	"core/skills.ts":                                         "M5",
	"core/slash-commands.ts":                                 "M5",
	"core/source-info.ts":                                    "M5",
	"core/system-prompt.ts":                                  "M5",
	"core/tools/bash.ts":                                     "M3",
	"core/tools/edit-diff.ts":                                "M3",
	"core/tools/edit.ts":                                     "M3",
	"core/tools/file-mutation-queue.ts":                      "M3",
	"core/tools/find.ts":                                     "M4",
	"core/tools/grep.ts":                                     "M4",
	"core/tools/index.ts":                                    "M4",
	"core/tools/ls.ts":                                       "M4",
	"core/tools/read.ts":                                     "M1",
	"core/tools/truncate.ts":                                 "M3",
	"core/tools/write.ts":                                    "M3",
	"core/trust-manager.ts":                                  "M3",
	"main.ts":                                                "M1",
	"modes/interactive/components/armin.ts":                  "M6",
	"modes/interactive/components/assistant-message.ts":      "M6",
	"modes/interactive/components/bash-execution.ts":         "M6",
	"modes/interactive/components/bordered-loader.ts":        "M6",
	"modes/interactive/components/branch-summary-message.ts": "M6",
	"modes/interactive/components/compaction-summary-message.ts": "M6",
	"modes/interactive/components/custom-editor.ts":              "M6",
	"modes/interactive/components/custom-message.ts":             "M6",
	"modes/interactive/components/diff.ts":                       "M6",
	"modes/interactive/components/dynamic-border.ts":             "M6",
	"modes/interactive/components/extension-editor.ts":           "M6",
	"modes/interactive/components/extension-input.ts":            "M6",
	"modes/interactive/components/extension-selector.ts":         "M6",
	"modes/interactive/components/footer.ts":                     "M6",
	"modes/interactive/components/keybinding-hints.ts":           "M6",
	"modes/interactive/components/login-dialog.ts":               "M11",
	"modes/interactive/components/model-selector.ts":             "M6",
	"modes/interactive/components/oauth-selector.ts":             "M11",
	"modes/interactive/components/session-selector.ts":           "M6",
	"modes/interactive/components/settings-selector.ts":          "M6",
	"modes/interactive/components/show-images-selector.ts":       "M12",
	"modes/interactive/components/skill-invocation-message.ts":   "M6",
	"modes/interactive/components/theme-selector.ts":             "M6",
	"modes/interactive/components/thinking-selector.ts":          "M6",
	"modes/interactive/components/tool-execution.ts":             "M6",
	"modes/interactive/components/tree-selector.ts":              "M6",
	"modes/interactive/components/user-message-selector.ts":      "M6",
	"modes/interactive/components/user-message.ts":               "M6",
	"modes/interactive/components/visual-truncate.ts":            "M6",
	"modes/interactive/interactive-mode.ts":                      "M6",
	"modes/interactive/theme/theme.ts":                           "M6",
	"modes/json-event.ts":                                        "M1",
	"modes/print-mode.ts":                                        "M1",
	"modes/rpc/rpc-client.ts":                                    "M4",
	"modes/rpc/rpc-mode.ts":                                      "M4",
	"modes/rpc/rpc-types.ts":                                     "M4",
	"utils/clipboard.ts":                                         "M13",
	"utils/frontmatter.ts":                                       "M5",
	"utils/image-convert.ts":                                     "M12",
	"utils/image-resize-core.ts":                                 "M12",
	"utils/image-resize.ts":                                      "M12",
	"utils/shell.ts":                                             "M13",
}

func issue32Milestone(reference string) (string, error) {
	local, ok := strings.CutPrefix(reference, issue32ReferencePrefix)
	if !ok {
		return "", fmt.Errorf("reference %q is outside the pinned Coding Agent source tree", reference)
	}
	path, symbol, ok := strings.Cut(local, "#")
	if !ok || path == "" || symbol == "" || strings.Contains(symbol, "#") {
		return "", fmt.Errorf("reference %q is not a Coding Agent symbol reference", reference)
	}
	if milestone, ok := issue32MilestoneOverrides[local]; ok {
		return milestone, nil
	}
	if milestone, ok := issue32MilestonesByFile[path]; ok {
		return milestone, nil
	}
	return "", fmt.Errorf("source file %q has no reviewed roadmap milestone", path)
}

func TestIssue32Milestone(t *testing.T) {
	tests := []struct {
		name      string
		reference string
		want      string
		wantErr   string
	}{
		{
			name:      "exact declaration override wins over file default",
			reference: issue32ReferencePrefix + "core/settings-manager.ts#CompactionSettings",
			want:      "M4",
		},
		{
			name:      "reviewed file supplies fallback",
			reference: issue32ReferencePrefix + "core/settings-manager.ts#SettingsManager",
			want:      "M3",
		},
		{
			name:      "unknown file is rejected",
			reference: issue32ReferencePrefix + "core/not-reviewed.ts#Thing",
			wantErr:   `source file "core/not-reviewed.ts" has no reviewed roadmap milestone`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := issue32Milestone(tt.reference)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("issue32Milestone(%q) = %q, want error %q", tt.reference, got, tt.wantErr)
				}
				if err.Error() != tt.wantErr {
					t.Fatalf("issue32Milestone(%q) error = %q, want %q", tt.reference, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("issue32Milestone(%q): %v", tt.reference, err)
			}
			if got != tt.want {
				t.Fatalf("issue32Milestone(%q) = %q, want %q", tt.reference, got, tt.want)
			}
		})
	}
}

func TestIssue32ExpectedCatalogEntriesInheritParentMilestone(t *testing.T) {
	newSymbol := func(path, name, kind string, members, staticMembers []string) surface.Symbol {
		return surface.Symbol{
			SchemaVersion: surface.SchemaVersion,
			ID:            "symbol:codingagent/src/" + path + "#" + name,
			Module:        "codingagent",
			Name:          name,
			Kind:          kind,
			Upstream: surface.Upstream{
				Module:     "coding-agent",
				Repository: "https://github.com/badlogic/pi-mono",
				Commit:     issue32BaselineCommit,
				Reference:  issue32ReferencePrefix + path + "#" + name,
			},
			ExportSubpaths: []string{"."},
			Members:        members,
			StaticMembers:  staticMembers,
		}
	}

	entries, err := issue32ExpectedCatalogEntries([]surface.Symbol{
		newSymbol("core/agent-session.ts", "SessionStats", "interface", []string{"tokens"}, nil),
		newSymbol("core/extensions/types.ts", "SessionBeforeCompactEvent", "interface", []string{"signal"}, nil),
		newSymbol("core/keybindings.ts", "KeybindingsManager", "class", nil, []string{"create"}),
		newSymbol("core/session-manager.ts", "SessionManager", "class", []string{"_persist"}, nil),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 8 {
		t.Fatalf("generated catalog rows = %d, want 8", len(entries))
	}

	byID := make(map[string]catalog.Entry, len(entries))
	for _, entry := range entries {
		byID[entry.ID] = entry
	}
	tests := []struct {
		name          string
		parentID      string
		childID       string
		wantMilestone string
		wantTarget    string
	}{
		{
			name:          "member inherits exact parent override",
			parentID:      "symbol:codingagent/src/core/agent-session.ts#SessionStats",
			childID:       "member:codingagent/src/core/agent-session.ts#SessionStats.tokens",
			wantMilestone: "M4",
			wantTarget:    issue32GoPackage + ".SessionStats.Tokens",
		},
		{
			name:          "projected member target does not change ownership",
			parentID:      "symbol:codingagent/src/core/extensions/types.ts#SessionBeforeCompactEvent",
			childID:       "member:codingagent/src/core/extensions/types.ts#SessionBeforeCompactEvent.signal",
			wantMilestone: "M7",
			wantTarget:    issue32GoPackage + ".AgentSession.Compact",
		},
		{
			name:          "static package function target does not change ownership",
			parentID:      "symbol:codingagent/src/core/keybindings.ts#KeybindingsManager",
			childID:       "static-member:codingagent/src/core/keybindings.ts#KeybindingsManager.create",
			wantMilestone: "M6",
			wantTarget:    issue32GoPackage + ".NewKeybindingsManager",
		},
		{
			name:          "intended private persistence helper stays on its owning carrier",
			parentID:      "symbol:codingagent/src/core/session-manager.ts#SessionManager",
			childID:       "member:codingagent/src/core/session-manager.ts#SessionManager._persist",
			wantMilestone: "M3",
			wantTarget:    issue32GoPackage + ".SessionManager",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parent, ok := byID[tt.parentID]
			if !ok {
				t.Fatalf("missing parent row %q", tt.parentID)
			}
			child, ok := byID[tt.childID]
			if !ok {
				t.Fatalf("missing child row %q", tt.childID)
			}
			if parent.Milestone != tt.wantMilestone {
				t.Fatalf("parent milestone = %q, want %q", parent.Milestone, tt.wantMilestone)
			}
			if child.Milestone != parent.Milestone {
				t.Errorf("child milestone = %q, want inherited parent milestone %q", child.Milestone, parent.Milestone)
			}
			if child.Mapping.Target != tt.wantTarget {
				t.Errorf("child mapping target = %q, want %q", child.Mapping.Target, tt.wantTarget)
			}
		})
	}

	persist := byID["member:codingagent/src/core/session-manager.ts#SessionManager._persist"]
	if persist.Status != catalog.StatusInventoried || persist.Classification != "private-impl" {
		t.Errorf("SessionManager._persist disposition = status %q, classification %q; want inventoried private-impl", persist.Status, persist.Classification)
	}
	if len(persist.Evidence) != 0 {
		t.Errorf("SessionManager._persist has behavioral evidence despite deferred persistence: %+v", persist.Evidence)
	}
	for _, phrase := range []string{"intended-private", "deferred", "no artificial Go field"} {
		if !strings.Contains(persist.Notes, phrase) {
			t.Errorf("SessionManager._persist notes do not mention %q: %q", phrase, persist.Notes)
		}
	}
}

func issue32ExpectedCatalogEntries(symbols []surface.Symbol) ([]catalog.Entry, error) {
	var entries []catalog.Entry
	for _, symbol := range symbols {
		milestone, err := issue32Milestone(symbol.Upstream.Reference)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", symbol.ID, err)
		}
		target := issue32SymbolTarget(symbol)
		behaviorOwner := issue32BehaviorOwnerForReference(symbol.Upstream.Reference)
		symbolEntry := catalog.Entry{SchemaVersion: catalog.SchemaVersion, ID: symbol.ID, Upstream: catalog.Upstream(symbol.Upstream), Mapping: catalog.Mapping{Module: "codingagent", Target: target, Kind: "symbol"}, Status: catalog.StatusScaffolded, Milestone: milestone, Classification: "public-api", Notes: fmt.Sprintf("Issue #32 concrete Coding Agent SDK symbol mapping authority. Both root and ./client exports map to the canonical Go codingagent package; behavioral status remains on %s.", behaviorOwner)}
		issue32PromoteRuntimeEntry(&symbolEntry)
		entries = append(entries, symbolEntry)
		for _, member := range symbol.Members {
			id := "member:" + strings.TrimPrefix(symbol.ID, "symbol:") + "." + member
			memberTarget, reason := issue32MemberTarget(symbol, target, member)
			memberKey := symbol.Name + "." + member
			memberReference := symbol.Upstream.Reference + "." + member
			e := catalog.Entry{SchemaVersion: catalog.SchemaVersion, ID: id, Upstream: catalog.Upstream{Module: "coding-agent", Repository: symbol.Upstream.Repository, Commit: symbol.Upstream.Commit, Reference: memberReference}, Mapping: catalog.Mapping{Module: "codingagent", Target: memberTarget, Kind: "contract"}, Status: catalog.StatusScaffolded, Milestone: milestone, Classification: "public-api", Notes: fmt.Sprintf("Issue #32 concrete %s member mapping authority. Behavioral status remains on %s.", symbol.Name, behaviorOwner)}
			if reason != "" {
				e.Status = catalog.StatusInventoried
				if projection, ok := issue32MemberProjectionFor(symbol.Name, member); ok {
					if projection.classification != "" {
						e.Classification = projection.classification
					}
					e.Notes = fmt.Sprintf("Issue #32 records %s as a deliberate Go projection because %s. It maps to %s; no artificial Go field is required.", memberKey, projection.reason, memberTarget)
				} else {
					e.Notes = fmt.Sprintf("Issue #32 records %s.%s as %s. It maps to its Go carrier; no artificial Go member is required.", symbol.Name, member, reason)
				}
			} else {
				e.Evidence = []catalog.Evidence{{Kind: "go-test", Ref: issue32MemberTestRef, Baseline: issue32BaselineCommit, CaseID: id, InputHash: issue32SurfaceHash, ExecutionMethod: issue32MemberTestRun, Expected: fmt.Sprintf("the pinned Pi %s.%s member maps to the declared scaffolded Go contract target", symbol.Name, member), Actual: fmt.Sprintf("PASS; %s resolved to %s", id, memberTarget), Platform: "any", CatalogID: id}}
			}
			issue32PromoteRuntimeEntry(&e)
			entries = append(entries, e)
		}
		for _, member := range symbol.StaticMembers {
			id := "static-member:" + strings.TrimPrefix(symbol.ID, "symbol:") + "." + member
			staticTarget, err := issue32StaticMemberTarget(symbol, member)
			if err != nil {
				return nil, err
			}
			entries = append(entries, catalog.Entry{
				SchemaVersion:  catalog.SchemaVersion,
				ID:             id,
				Upstream:       catalog.Upstream{Module: "coding-agent", Repository: symbol.Upstream.Repository, Commit: symbol.Upstream.Commit, Reference: symbol.Upstream.Reference + "." + member},
				Mapping:        catalog.Mapping{Module: "codingagent", Target: staticTarget, Kind: "contract"},
				Status:         catalog.StatusScaffolded,
				Milestone:      milestone,
				Classification: "public-api",
				Evidence: []catalog.Evidence{{
					Kind:            "go-test",
					Ref:             issue32StaticMemberTestRef,
					Baseline:        issue32BaselineCommit,
					CaseID:          id,
					InputHash:       issue32SurfaceHash,
					ExecutionMethod: issue32StaticMemberTestRun,
					Expected:        fmt.Sprintf("the pinned Pi %s.%s static member maps to the declared scaffolded Go package function", symbol.Name, member),
					Actual:          fmt.Sprintf("PASS; %s resolved to %s", id, staticTarget),
					Platform:        "any",
					CatalogID:       id,
				}},
				Notes: fmt.Sprintf("Issue #32 concrete %s static-member mapping authority. Behavioral status remains on %s.", symbol.Name, behaviorOwner),
			})
		}
		if symbol.Constructible {
			entry, err := issue32ConstructorEntry(symbol, milestone)
			if err != nil {
				return nil, err
			}
			entries = append(entries, entry)
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	return entries, nil
}

func issue32PromoteRuntimeEntry(entry *catalog.Entry) {
	const (
		headlessProcessHash = "sha256:f0c26458070ac2ff1c89393991ab14834564f15dc512f0a073c90a8378fed80f"
		sessionProcessHash  = "sha256:a47c1ca79073e8a07ca4853ecef36d700711bbb5ad473be3b8501af4d7d3bbc1"
		exactResultsHash    = "sha256:ce08293fc33a104302e5a26e4a11f2e2b46e3e57d469ae9ac58ac7fa6f6d5912"
		runtimeTestHash     = "sha256:d4cf309c1b8e8a4f68a3478c6e73ebdeb931842cfe4a47c65aa21c559e0bc6fe"
	)
	evidence := func(ref, caseID, inputHash, run, expected, actual, platform string) []catalog.Evidence {
		return []catalog.Evidence{{
			Kind: "go-test", Ref: ref, Baseline: issue32BaselineCommit, CaseID: caseID, InputHash: inputHash,
			ExecutionMethod: run, Expected: expected, Actual: actual, Platform: platform, CatalogID: entry.ID,
		}}
	}
	switch entry.ID {
	case "symbol:codingagent/src/main.ts#main":
		entry.Status = catalog.StatusPartial
		entry.Evidence = append(append(evidence(
			"cmd/pig/headless_process_test.go#TestPigProcessRunsHeadlessTextWithExplicitDeepSeekInputs",
			"issue56-codingagent-main-headless-text", headlessProcessHash,
			"go test ./cmd/pig -run '^TestPigProcess' -count=1",
			"the real pig process assembles explicit DeepSeek inputs and an in-memory Session while later product modes remain exact Capability Stubs",
			"PASS; explicit and ambient credentials reached a local DeepSeek endpoint, final text used stdout only, stable failures used stderr, and SIGINT exited 130",
			"darwin/linux",
		), evidence(
			"cmd/pig/headless_process_test.go#TestPigProcessStreamsSessionFirstHeadlessJSON",
			"issue57-codingagent-main-headless-json", headlessProcessHash,
			"go test ./cmd/pig -run '^TestPigProcess.*HeadlessJSON|^TestPigProcessTreatsPipedJSONAsPromptNotRPCCommand$' -count=1",
			"the real pig --mode json process emits a v3 Session header followed by ordered projected events while stdin remains prompt input",
			"PASS; JSONL was session-first, every event was parseable and ordered, cumulative partial snapshots were absent, and piped JSON remained literal prompt text",
			"darwin/linux",
		)...), evidence(
			"cmd/pig/issue71_process_test.go#TestPigProcessesPersistAndReopenExplicitSessionPath",
			"issue71-codingagent-main-session-persistence", sessionProcessHash,
			"go test ./cmd/pig -run '^TestPig(ProcessesPersistAndReopenExplicitSessionPath|NoSessionDoesNotCreatePigState|ExplicitPiSessionDoesNotMigrateAdjacentPiState)$' -count=1",
			"real pig processes create and reopen Pig-owned v3 Sessions while explicit memory remains side-effect-free and explicit Pi files do not migrate adjacent state",
			"PASS; a second process received prior history, appended without duplication, --no-session wrote no state, and explicit Pi state remained untouched",
			"darwin/linux",
		)...)
		entry.Partial = &catalog.Partial{
			Supported:   []string{"Headless text and one-way session-first JSON dispatch accept explicit Provider, exact model, API key or DEEPSEEK_API_KEY, prompt arguments or stdin, default Pig-owned v3 persistence, explicit-path reopen, and explicit memory"},
			Unsupported: []string{"interactive, RPC, continue/recent lookup, fork, resource, extension, and broader Provider assembly remain exact Capability Stubs"},
		}
		entry.Notes = "Issues #56 and #57 promote the pinned main entrypoint for real Headless text and session-first JSONL. Issue #71 adds Pig-owned v3 persistence and explicit reopen without migrating Pi state; later resource, extension, interactive, RPC, continue, and fork capabilities remain deferred."
	case "symbol:codingagent/src/modes/json-event.ts#JsonAgentSessionEvent":
		entry.Status = catalog.StatusImplemented
		entry.Evidence = evidence(
			"codingagent/exact_results_test.go#TestProjectJSONAgentSessionEventRemovesCumulativeMessageState",
			"issue57-json-agent-session-event-projection", exactResultsHash,
			"go test ./codingagent -run '^TestProjectJSONAgentSessionEventRemovesCumulativeMessageState$' -count=1",
			"message_update projects to the formal JSON union without the in-process cumulative message or nested Assistant partial snapshot",
			"PASS; the projection retained the incremental Assistant event and omitted both cumulative snapshots while non-update events remained unchanged",
			"any",
		)
		entry.Partial = nil
		entry.Notes = "Issue #57 implements the JSON-mode AgentSessionEvent projection, including the pinned omission of cumulative message and nested partial state from message_update records."
	case "symbol:codingagent/src/modes/print-mode.ts#runPrintMode":
		entry.Status = catalog.StatusPartial
		entry.Evidence = append(evidence(
			"cmd/pig/headless_process_test.go#TestPigProcessRunsHeadlessTextWithExplicitDeepSeekInputs",
			"issue56-run-print-mode-headless-text", headlessProcessHash,
			"go test ./cmd/pig -run '^TestPigProcess' -count=1",
			"print mode runs the shared Headless lifecycle, writes only final Assistant text on success, and distinguishes Provider errors and interruption",
			"PASS; the process emitted only final text, retained deterministic failure diagnostics, and propagated SIGINT as exit 130",
			"darwin/linux",
		), evidence(
			"cmd/pig/headless_process_test.go#TestPigProcessStreamsSessionFirstHeadlessJSON",
			"issue57-run-print-mode-headless-json", headlessProcessHash,
			"go test ./cmd/pig -run '^TestPigProcess.*HeadlessJSON|^TestPigProcessTreatsPipedJSONAsPromptNotRPCCommand$' -count=1",
			"JSON print mode writes the in-memory v3 Session header first and then one projected AgentSessionEvent per line for text, Tool, Provider-error, and cancellation flows",
			"PASS; process JSONL matched its normalized golden and event ordering, Provider error stayed in-band with exit 0, cancellation ended parseably with exit 130, and stdin was not interpreted as RPC",
			"darwin/linux",
		)...)
		entry.Partial = &catalog.Partial{
			Supported:   []string{"text and session-first JSONL presentation over the shared in-memory Headless runner, including multiple prompts, Tool continuation, terminal Provider outcomes, cleanup, and cancellation"},
			Unsupported: []string{"image inputs remain a documented unavailable capability"},
		}
		entry.Notes = "Issues #56 and #57 implement the text and one-way JSON branches of runPrintMode over the shared Headless lifecycle; image inputs remain deferred to M12."
	case "symbol:codingagent/src/core/agent-session-runtime.ts#createAgentSessionRuntime":
		entry.Status = catalog.StatusImplemented
		entry.Evidence = evidence(
			"codingagent/session_api_final_review_test.go#TestAgentSessionRuntimeCreatesInitialSessionOnlyWhenRequested",
			"issue56-create-agent-session-runtime", runtimeTestHash,
			"go test ./codingagent -run '^TestAgentSessionRuntimeCreatesInitialSessionOnlyWhenRequested$' -count=1",
			"the runtime factory is invoked once on explicit creation and its Session and services become the runtime's initial owned state",
			"PASS; construction remained inert until requested, then called the factory once and returned its usable Session",
			"any",
		)
		entry.Partial = nil
		entry.Notes = "Issue #56 implements initial AgentSessionRuntime creation through the supplied factory; replacement operations keep their independently cataloged Capability Stubs."
	case "member:codingagent/src/core/agent-session-runtime.ts#AgentSessionRuntime.dispose":
		entry.Status = catalog.StatusImplemented
		entry.Evidence = evidence(
			"codingagent/session_api_final_review_test.go#TestAgentSessionRuntimeCreatesInitialSessionOnlyWhenRequested",
			"issue56-agent-session-runtime-dispose", runtimeTestHash,
			"go test ./codingagent -run '^TestAgentSessionRuntimeCreatesInitialSessionOnlyWhenRequested$' -count=1",
			"disposing the runtime disposes its owned AgentSession and a nil runtime Session is safe",
			"PASS; the created runtime disposed its factory Session successfully",
			"any",
		)
		entry.Partial = nil
		entry.Notes = "Issue #56 implements AgentSessionRuntime disposal as ownership-preserving delegation to the current AgentSession."
	}
}

func issue32BehaviorOwnerForReference(reference string) string {
	switch {
	case strings.HasPrefix(reference, issue32ReferencePrefix+"core/settings-manager.ts#"):
		return issue74SettingsCatalogID
	case strings.HasPrefix(reference, issue32ReferencePrefix+"core/agent-session.ts#"),
		strings.HasPrefix(reference, issue32ReferencePrefix+"core/sdk.ts#createAgentSession"),
		strings.HasPrefix(reference, issue32ReferencePrefix+"core/sdk.ts#CreateAgentSessionOptions"):
		return issue32AgentSessionID
	case reference == issue32ReferencePrefix+"core/tools/read.ts#createReadTool":
		return issue32ReadToolCatalogID
	case strings.HasPrefix(reference, issue32ReferencePrefix+"client/transcript.ts#"):
		return issue32TranscriptCatalogID
	case strings.HasPrefix(reference, issue32ReferencePrefix+"core/compaction/"):
		return issue32CompactionCatalogID
	case reference == issue32ReferencePrefix+"core/session-manager.ts#migrateSessionEntries":
		return issue32MigrationCatalogID
	case strings.HasPrefix(reference, issue32ReferencePrefix+"core/session-manager.ts#"):
		return issue32V3JSONLCatalogID
	default:
		return issue32ModuleCatalogID
	}
}

func issue32StaticMemberTarget(symbol surface.Symbol, member string) (string, error) {
	key := symbol.Name + "." + member
	target, ok := issue32StaticMemberTargetExceptions[key]
	if !ok {
		return "", fmt.Errorf("no Go package-function mapping for Coding Agent static member %s", key)
	}
	return issue32GoPackage + "." + target, nil
}

func issue32SymbolTarget(symbol surface.Symbol) string {
	if mapped, ok := issue32SymbolTargetExceptions[symbol.ID]; ok {
		return issue32GoPackage + "." + mapped
	}
	return issue32GoPackage + "." + issue32GoName(symbol.Name)
}
func issue32MemberTarget(symbol surface.Symbol, parent, member string) (string, string) {
	if reflect.DeepEqual(symbol.Members, issue32StringInheritedMembers) {
		return parent, "a TypeScript default-library inherited String member"
	}
	if reflect.DeepEqual(symbol.Members, issue32NumberInheritedMembers) {
		return parent, "a TypeScript default-library inherited Number member"
	}
	if projection, ok := issue32MemberProjectionFor(symbol.Name, member); ok {
		packagePath := projection.packagePath
		if packagePath == "" {
			packagePath = issue32GoPackage
		}
		return packagePath + "." + projection.target, projection.reason
	}
	if target, ok := issue32MemberTargetExceptions[symbol.Name+"."+member]; ok {
		return issue32GoPackage + "." + target, ""
	}
	return parent + "." + issue32GoName(member), ""
}
func issue32GoName(name string) string {
	if mapped, ok := issue32NameExceptions[name]; ok {
		return mapped
	}
	if name == "" {
		return ""
	}
	if strings.Contains(name, ".") {
		var b strings.Builder
		for _, p := range strings.Split(name, ".") {
			b.WriteString(issue32GoName(p))
		}
		return b.String()
	}
	if strings.Contains(name, "_") {
		var b strings.Builder
		for _, p := range strings.Split(strings.ToLower(name), "_") {
			if p != "" {
				b.WriteString(strings.ToUpper(p[:1]) + p[1:])
			}
		}
		name = b.String()
	}
	name = strings.ToUpper(name[:1]) + name[1:]
	return strings.NewReplacer("Jsonl", "JSONL", "Json", "JSON", "Api", "API", "Rpc", "RPC", "Tui", "TUI", "Url", "URL", "Uri", "URI", "Uuid", "UUID", "Id", "ID", "Cwd", "CWD", "Ai", "AI", "Llm", "LLM", "Oauth", "OAuth", "Http", "HTTP", "Html", "HTML", "Ansi", "ANSI", "Png", "PNG", "Fs", "FS", "Cli", "CLI", "Sdk", "SDK", "Ms", "MS").Replace(name)
}
func issue32TargetName(target string) (string, error) {
	prefix := issue32GoPackage + "."
	if !strings.HasPrefix(target, prefix) {
		return "", fmt.Errorf("unsupported Go target %q", target)
	}
	name := strings.TrimPrefix(target, prefix)
	if name == "" || strings.Contains(name, ".") {
		return "", fmt.Errorf("symbol target %q is not a declaration in canonical codingagent package", target)
	}
	return name, nil
}
func issue32MemberTargetParts(target string) (string, string, error) {
	prefix := issue32GoPackage + "."
	if !strings.HasPrefix(target, prefix) {
		return "", "", fmt.Errorf("unsupported Go target %q", target)
	}
	typ, member, ok := strings.Cut(strings.TrimPrefix(target, prefix), ".")
	if !ok || typ == "" || member == "" || strings.Contains(member, ".") {
		return "", "", fmt.Errorf("member target %q does not name one canonical member", target)
	}
	return typ, member, nil
}

func issue32Symbols(t *testing.T) []surface.Symbol {
	t.Helper()
	symbols, err := surface.LoadSymbols(filepath.Join(issue32RepoRoot(t), "parity", "surface", "symbols.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var out []surface.Symbol
	for _, s := range symbols {
		if s.Module == "codingagent" && s.Upstream.Module == "coding-agent" && strings.HasPrefix(s.Upstream.Reference, "packages/coding-agent/src/") {
			out = append(out, s)
		}
	}
	return out
}
func issue32AllCatalogEntries(t *testing.T) []catalog.Entry {
	t.Helper()
	e, err := catalog.LoadCatalog(filepath.Join(issue32RepoRoot(t), "parity", "catalog.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	return e
}
func issue32EntryByID(entries []catalog.Entry, id string) (catalog.Entry, bool) {
	for _, entry := range entries {
		if entry.ID == id {
			return entry, true
		}
	}
	return catalog.Entry{}, false
}
func issue32CatalogEntries(t *testing.T) []catalog.Entry {
	var out []catalog.Entry
	for _, e := range issue32AllCatalogEntries(t) {
		if issue32CatalogID(e.ID) {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
func issue32CatalogSymbolEntries(t *testing.T) []catalog.Entry {
	var out []catalog.Entry
	for _, e := range issue32AllCatalogEntries(t) {
		if e.Mapping.Module == "codingagent" && e.Mapping.Kind == "symbol" {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
func issue32CatalogID(id string) bool {
	return strings.HasPrefix(id, "symbol:codingagent/") || strings.HasPrefix(id, "member:codingagent/") || strings.HasPrefix(id, "static-member:codingagent/") || strings.HasPrefix(id, "constructor:codingagent/")
}
func issue32CountKind(entries []catalog.Entry, kind string) int {
	n := 0
	for _, e := range entries {
		if e.Mapping.Kind == kind {
			n++
		}
	}
	return n
}
func issue32CountIDPrefix(entries []catalog.Entry, prefix string) int {
	n := 0
	for _, e := range entries {
		if strings.HasPrefix(e.ID, prefix) {
			n++
		}
	}
	return n
}
func issue32SortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
func issue32RepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
}

func issue32FileHash(t *testing.T, relative string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(issue32RepoRoot(t), filepath.FromSlash(relative)))
	if err != nil {
		t.Fatalf("read %s: %v", relative, err)
	}
	return fmt.Sprintf("sha256:%x", sha256.Sum256(data))
}

type issue32ModuleEvidenceDescriptor struct {
	Evidence        catalog.Evidence
	InputPath       string
	PinnedInputHash string
}

func issue32BehaviorOwnerEntries(t *testing.T) []catalog.Entry {
	t.Helper()
	repository := "https://github.com/badlogic/pi-mono"
	entries := []catalog.Entry{
		{
			SchemaVersion: catalog.SchemaVersion, ID: issue32AgentSessionID,
			Upstream: catalog.Upstream{Module: "coding-agent", Repository: repository, Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/core/sdk.ts#createAgentSession"},
			Mapping:  catalog.Mapping{Module: "codingagent", Target: issue32GoPackage + ".CreateAgentSession", Kind: "contract"},
			Status:   catalog.StatusPartial, Milestone: "M1", Classification: "public-api",
			Partial: &catalog.Partial{
				Supported: []string{
					"callers can inject a model, narrow Provider or StreamFunction, and executable Agent Tools into a pure in-memory AgentSession",
					"text and read Tool continuation, AgentSession event bridging, in-memory transcript append, cancellation, settlement, disposal, header identity, and nil SessionFile are implemented",
					"callers can inject a persisted v3 SessionManager, reopen its history in a new runtime, and continue without rewriting prior entries",
				},
				Unsupported: []string{
					"ambient model, credential, settings, trust, resource, and package assembly remain explicit Capability Stubs",
					"queues, extension ToolDefinition execution, branches, compaction, RPC, and other later-milestone AgentSession operations remain explicit Capability Stubs",
				},
			},
			Notes: "Issue #55 delivers the narrow M1 Go SDK AgentSession slice and issue #71 adds explicit M3 v3 persistence injection and reopen. Session event listeners are ordered barriers, agent_settled follows transcript updates, and in-memory creation plus execution perform no ambient disk writes.",
		},
		{
			SchemaVersion: catalog.SchemaVersion, ID: issue32ReadToolCatalogID,
			Upstream: catalog.Upstream{Module: "coding-agent", Repository: repository, Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/core/tools/read.ts#createReadTool"},
			Mapping:  catalog.Mapping{Module: "codingagent", Target: issue32GoPackage + ".CreateReadTool", Kind: "contract"},
			Status:   catalog.StatusPartial, Milestone: "M1", Classification: "public-api",
			Partial: &catalog.Partial{
				Supported: []string{
					"focused Go tests verify pinned-source path normalization, host-user permissions, number-valued offset and limit pagination, truncation, failure ToolResults, and cancellation",
					"a locked Pi source/dist Oracle verifies the short real-file Agent ToolCall-to-ToolResult-to-Faux continuation slice",
				},
				Unsupported: []string{
					"image payload execution remains an explicit M12 Capability Stub",
					"CreateReadToolDefinition remains the explicit M7 Extension ABI Capability Stub",
				},
			},
			Notes: "Issue #54 implements the public M1 text read Tool through real host files and verifies its Agent continuation against locked Pi source and built dist. Image execution remains deferred to M12, and the extension-facing read Tool definition remains deferred to M7.",
		},
		{
			SchemaVersion: catalog.SchemaVersion, ID: issue32CompactionCatalogID,
			Upstream: catalog.Upstream{Module: "coding-agent", Repository: repository, Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/core/compaction"},
			Mapping:  catalog.Mapping{Module: "codingagent", Target: issue32GoPackage, Kind: "contract"},
			Status:   catalog.StatusPartial, Milestone: "M4", Classification: "public-api",
			Partial: &catalog.Partial{
				Supported: []string{
					"shared compaction settings, context-token calculation, threshold policy, last usable assistant usage, turn-start and cut-point selection are implemented",
					"branch divergence collection, branch-entry preparation, context projection, token estimation, conversation serialization, and UTF-16 tool-result truncation are implemented",
				},
				Unsupported: []string{
					"GenerateSummary, GenerateSummaryWithUsage, and GenerateBranchSummary remain explicit Capability Stubs and do not invoke a model",
					"end-to-end Compact, automatic triggering, persistence, cancellation, retry, event lifecycle, and settings mutation remain explicit Capability Stubs",
				},
			},
			Notes: "Behavior owner for the M4 production Coding Agent compaction pipeline. Deterministic policy, selection, projection, and serialization helpers are live while model-backed summarization and end-to-end compaction remain explicitly unsupported.",
		},
		{
			SchemaVersion: catalog.SchemaVersion, ID: issue32TranscriptCatalogID,
			Upstream: catalog.Upstream{Module: "coding-agent", Repository: repository, Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/client/transcript.ts"},
			Mapping:  catalog.Mapping{Module: "codingagent", Target: issue32GoPackage, Kind: "contract"},
			Status:   catalog.StatusImplemented, Milestone: "M9", Classification: "public-api",
			Notes: "Behavior owner for the M9 Coding Agent client transcript projection. Authoritative snapshots, progress overlays, assistant deltas, partial tool-call buffering, stable ordering, queued-steering deduplication, stale-snapshot handling, and defensive ownership are implemented; fixed-baseline differential verification remains outstanding.",
		},
		{
			SchemaVersion: catalog.SchemaVersion, ID: issue32V3JSONLCatalogID,
			Upstream: catalog.Upstream{Module: "coding-agent", Repository: repository, Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/core/session-manager.ts#CURRENT_SESSION_VERSION"},
			Mapping:  catalog.Mapping{Module: "codingagent", Target: issue32GoPackage, Kind: "contract"},
			Status:   catalog.StatusPartial, Milestone: "M3", Classification: "public-api",
			Partial: &catalog.Partial{
				Supported: []string{
					"newline-delimited session records preserve every syntactically valid raw JSON value while projecting recognized v3 headers, message discriminators, and session entries",
					"branch lookup, latest-compaction context reconstruction, model and thinking-level recovery, defensive snapshots, and side-effect-free in-memory v3 sessions with owned message append are implemented",
					"filesystem-backed create and explicit-path open persist v3 headers, model and thinking changes, messages, ToolResults, failures, cancellation outcomes, and parent chains with Pi-compatible first-write timing and append-only reopen",
				},
				Unsupported: []string{
					"continue/recent lookup, fork, session-file reassignment, and branch mutation remain explicit Capability Stubs",
					"tree and child traversal plus end-to-end continue and fork persistence semantics remain explicit Capability Stubs",
				},
			},
			Notes: "Behavior owner for production Coding Agent v3 JSONL parsing, append-only persistence, explicit reopen, and in-memory projection. Issue #71 verifies the highest public CLI and Go SDK boundaries against the fixed Pi baseline; continue, fork, tree, and later mutations remain unclaimed.",
		},
		{
			SchemaVersion: catalog.SchemaVersion, ID: issue32MigrationCatalogID,
			Upstream: catalog.Upstream{Module: "coding-agent", Repository: repository, Commit: issue32BaselineCommit, Reference: "packages/coding-agent/src/core/session-manager.ts#migrateSessionEntries"},
			Mapping:  catalog.Mapping{Module: "codingagent", Target: issue32GoPackage, Kind: "contract"},
			Status:   catalog.StatusImplemented, Milestone: "M3", Classification: "public-api",
			Notes: "Behavior owner for production Coding Agent session data migration. In-memory v1 records upgrade to v3 with generated IDs, parent chains, and compaction first-kept references; v2 hookMessage records become custom-role messages without rewriting an existing tree. This does not perform filesystem, credential, trust, layout, or CLI migration.",
		},
	}
	entries = append(entries, issue74SettingsCatalogEntry())
	for index := range entries {
		entries[index].Evidence = issue32EvidenceFromDescriptors(issue32BehaviorEvidenceDescriptors(t, entries[index].ID))
	}
	return entries
}

func issue32BehaviorEvidenceDescriptors(t *testing.T, catalogID string) []issue32ModuleEvidenceDescriptor {
	t.Helper()
	var descriptors []issue32ModuleEvidenceDescriptor
	switch catalogID {
	case issue74SettingsCatalogID:
		descriptors = issue74SettingsEvidence(t)
	case issue32AgentSessionID:
		descriptors = []issue32ModuleEvidenceDescriptor{
			{InputPath: "codingagent/agent_session_runtime_test.go", Evidence: catalog.Evidence{
				Kind: "go-test", Ref: "codingagent/agent_session_runtime_test.go#TestCreateAgentSessionRunsTextRoundAndSettlesAfterTranscriptUpdate", Baseline: issue32BaselineCommit,
				CaseID:          "issue55-codingagent-in-memory-agent-session",
				ExecutionMethod: "go test -race ./codingagent -run '^(TestCreateAgentSessionRunsTextRoundAndSettlesAfterTranscriptUpdate|TestCreateAgentSessionRunsReadContinuationWithInjectedProviderAndTool|TestAgentSessionAbortRequestsCancellationAndWaitForIdleObservesSettledTranscript|TestAgentSessionAbortIsSafeFromSynchronousListeners|TestAgentSessionDisposeCancelsWithoutDroppingActiveTranscript|TestAgentSessionUnsupportedPromptOptionsAreExactInertStubs|TestCreateAgentSessionNoToolsModesMatchPublicSelectionContract|TestSessionManagerAppendMessageBuildsInMemoryOwnedChain|TestSDKInMemorySessionInjectionFields|TestAgentSessionExactOperationStubsAreInert)$' -count=1",
				Expected:        "the public Go SDK constructs a side-effect-free in-memory session from injected model, Provider or stream, and Tool dependencies; bridges ordered events; publishes agent_settled after transcript updates; and settles abort and disposal without activating later capabilities",
				Actual:          "PASS; text and real read continuation completed through CreateAgentSession, transcript and identity remained in memory, cancellation settled deterministically under the race detector, and later operations retained structured stubs",
				Platform:        "any", CatalogID: catalogID,
			}},
			{InputPath: "codingagent/issue71_session_persistence_test.go", Evidence: catalog.Evidence{
				Kind: catalog.MatrixEvidenceGoTest, Ref: "codingagent/issue71_session_persistence_test.go#TestCreateAgentSessionReopensPersistedHistoryWithoutDuplicatingIt", Baseline: issue32BaselineCommit,
				CaseID: "issue71-codingagent-persisted-agent-session", ExecutionMethod: "go test ./codingagent -run '^(TestCreateAgentSessionReopensPersistedHistoryWithoutDuplicatingIt|TestPersistedAgentSessionKeepsProviderFailureAndReportsStorageFailure)$' -count=1",
				Expected: "a new public SDK runtime reopens persisted history without duplicate entries and preserves Provider failure, cancellation, cleanup, and observable storage-failure outcomes",
				Actual:   "PASS; the second runtime sent prior history to the Provider, appended one new round, retained partial terminal outcomes, and returned persistence errors without claiming a stored file", Platform: "any", CatalogID: catalogID,
			}},
		}
	case issue32ReadToolCatalogID:
		const (
			inputHash       = "sha256:f4f006f254b90ce868c7f6440fca8c5cef7595f18a1e404fc80200abd4d5117d"
			observationHash = "sha256:473491ffaf0329fcfb81ca79f5342daae8cc0b6961181377ded2c9602118ba23"
		)
		descriptors = []issue32ModuleEvidenceDescriptor{
			{
				InputPath: "parity/oracle/fixtures/codingagent-read-continuation.json", PinnedInputHash: inputHash,
				Evidence: catalog.Evidence{
					Kind: catalog.MatrixEvidenceOracle, Ref: "parity/oracle/fixtures/codingagent-read-continuation.json", Baseline: issue32BaselineCommit,
					CaseID:          "go-sdk/codingagent/read-tool-continuation",
					ExecutionMethod: "node --experimental-strip-types parity/oracle/agent-tool-continuation.mjs --read <locked-pi-checkout>; source modules with built-dist differential",
					Expected:        "locked Pi createReadTool reads the real sentinel and runAgentLoop passes its ToolResult into the deterministic continuation observation " + observationHash,
					Actual:          "PASS; locked source and built dist produced identical read continuation observation " + observationHash + " without network side effects",
					Platform:        "any", CatalogID: catalogID,
				},
			},
			{
				InputPath: "parity/oracle/fixtures/codingagent-read-continuation.json", PinnedInputHash: inputHash,
				Evidence: catalog.Evidence{
					Kind: catalog.MatrixEvidenceGoTest, Ref: "internal/parity/codingagent_read_continuation_test.go#TestCodingAgentReadContinuationParity", Baseline: issue32BaselineCommit,
					CaseID:          "go-sdk/codingagent/read-tool-continuation",
					ExecutionMethod: "go test ./internal/parity -run '^TestCodingAgentReadContinuationParity$' -count=1",
					Expected:        "Pig public CreateReadTool events, ToolResult, Provider continuation context, transcript and real sentinel file match Oracle observation " + observationHash,
					Actual:          "PASS; Pig read continuation observation " + observationHash + " matched the locked Pi fixture with no normalizations",
					Platform:        "any", CatalogID: catalogID,
				},
			},
			{
				InputPath: "codingagent/read_tool_test.go",
				Evidence: catalog.Evidence{
					Kind: "go-test", Ref: "codingagent/read_tool_test.go#TestReadToolResolvesHostPathsWithoutWorkspaceContainment", Baseline: issue32BaselineCommit,
					CaseID:          "issue54-codingagent-read-text-behavior",
					ExecutionMethod: "go test ./codingagent -run '^TestReadTool' -count=1",
					Expected:        "focused Go tests cover pinned-source path normalization, host path permissions, numeric pagination, deterministic truncation, compatible failure and cancellation ToolResults, and explicit image deferral",
					Actual:          "PASS; focused Go assertions covered path normalization and fallbacks, relative/absolute/parent/symlink access, numeric offset/limit boundaries, line/byte/long-line truncation details, failures, real cancellation, image deferral and Faux continuation",
					Platform:        "any", CatalogID: catalogID,
				},
			},
		}
	case issue32CompactionCatalogID:
		descriptors = []issue32ModuleEvidenceDescriptor{{
			InputPath: "codingagent/compaction_review_test.go",
			Evidence: catalog.Evidence{
				Kind: "go-test", Ref: "codingagent/compaction_review_test.go#TestCompactionPolicyUsesTheSharedAgentPrimitive", Baseline: issue32BaselineCommit,
				CaseID:          "issue32-codingagent-compaction-deterministic-core",
				ExecutionMethod: "go test ./codingagent -run '^(TestCompactionPolicyUsesTheSharedAgentPrimitive|TestFindCutPoint|TestCollectEntriesForBranchSummary|TestPrepareBranchEntries|TestSerializeConversationTruncatesToolResultsByUTF16CodeUnits|TestBuildSessionContextUsesLatestV3CompactionAndFullPathSettings|TestAgentSessionExactOperationStubsAreInert|TestRPCClientCompactResult|TestSessionManagerMutationStubsReturnNoIDsAndHaveNoSideEffects)$' -count=1",
				Expected:        "deterministic compaction policy, cut-point, branch collection, message preparation, context reconstruction, and UTF-16 serialization behavior work while model-backed, session, RPC, and persistence operations remain explicit inert Capability Stubs",
				Actual:          "PASS; deterministic compaction helpers produced the expected projections and all covered end-to-end or persistence operations returned structured ErrNotImplemented without mutation", Platform: "any", CatalogID: catalogID,
			},
		}}
	case issue32TranscriptCatalogID:
		descriptors = []issue32ModuleEvidenceDescriptor{{
			InputPath: "codingagent/transcript_projection_review_test.go",
			Evidence: catalog.Evidence{
				Kind: "go-test", Ref: "codingagent/transcript_projection_review_test.go#TestTranscriptProjectionReducer", Baseline: issue32BaselineCommit,
				CaseID: "issue32-codingagent-transcript-projection", ExecutionMethod: "go test ./codingagent -run '^(TestTranscriptProjectionReducer|TestTranscriptStateOwnsJSONCompositeAliases)$' -count=1",
				Expected: "snapshots and progress reduce immutably into stable-ID transcript order; assistant deltas and partial tool JSON are accumulated; accepted snapshots clear transient state while stale same-session snapshots do not",
				Actual:   "PASS; ordering, overlays, queued steering, text/thinking/tool deltas, buffer cleanup, revision handling, and defensive ownership matched the transcript projection contract", Platform: "any", CatalogID: catalogID,
			},
		}}
	case issue32V3JSONLCatalogID:
		descriptors = []issue32ModuleEvidenceDescriptor{
			{InputPath: "codingagent/session_parse_final_review_test.go", Evidence: catalog.Evidence{Kind: "go-test", Ref: "codingagent/session_parse_final_review_test.go#TestParseSessionEntriesPreservesEverySyntacticallyValidRecord", Baseline: issue32BaselineCommit, CaseID: "issue32-codingagent-v3-jsonl-forward-compatible-read", ExecutionMethod: "go test ./codingagent -run '^(TestParseSessionEntriesPreservesEverySyntacticallyValidRecord|TestParseSessionEntriesNormalizesMissingAndNullBuiltInMessageContent|TestSessionManagerGetBranchDistinguishesOmittedAndInvalidLeafIDs|TestSessionManagerTreeOperationsAreExplicitCapabilityStubs)$' -count=1", Expected: "the v3 read path retains syntactically valid raw JSON, projects known records, distinguishes branch selection states, and reports unimplemented tree operations explicitly", Actual: "PASS; valid records remained lossless, known message content normalized, branch reads selected the intended path, and tree operations returned structured ErrNotImplemented", Platform: "any", CatalogID: catalogID}},
			{InputPath: "codingagent/session_test.go", Evidence: catalog.Evidence{Kind: "go-test", Ref: "codingagent/session_test.go#TestBuildSessionContextUsesLatestV3CompactionAndFullPathSettings", Baseline: issue32BaselineCommit, CaseID: "issue32-codingagent-v3-jsonl-context-projection", ExecutionMethod: "go test ./codingagent -run '^(TestParseSessionEntriesDecodesV3MessageDiscriminators|TestBuildSessionContextUsesLatestV3CompactionAndFullPathSettings|TestSessionCarriersDefensivelyCopyNestedMutableValues)$' -count=1", Expected: "recognized v3 messages decode into typed carriers and context projection uses the latest compaction while retaining full-path model and thinking settings with defensive ownership", Actual: "PASS; v3 messages decoded, latest-compaction context and settings were reconstructed, and mutable carrier values did not alias inputs or getters", Platform: "any", CatalogID: catalogID}},
			{InputPath: "codingagent/session_contract_review_test.go", Evidence: catalog.Evidence{Kind: "go-test", Ref: "codingagent/session_contract_review_test.go#TestNewInMemorySessionManagerBuildsCompleteUniqueV3Headers", Baseline: issue32BaselineCommit, CaseID: "issue32-codingagent-v3-jsonl-in-memory-boundary", ExecutionMethod: "go test ./codingagent -run '^(TestNewInMemorySessionManagerBuildsCompleteUniqueV3Headers|TestSessionManagerAppendMessageBuildsInMemoryOwnedChain|TestSessionManagerMutationStubsReturnNoIDsAndHaveNoSideEffects)$' -count=1", Expected: "in-memory managers create complete distinct v3 headers and owned message chains without persistence, while later mutation methods return structured ErrNotImplemented without side effects", Actual: "PASS; in-memory headers and message parent chains were complete and defensively owned while all covered later mutation operations remained inert Capability Stubs", Platform: "any", CatalogID: catalogID}},
			{InputPath: "parity/oracle/fixtures/session-persistence.json", Evidence: catalog.Evidence{Kind: catalog.MatrixEvidenceOracle, Ref: "parity/oracle/fixtures/session-persistence.json", Baseline: issue32BaselineCommit, CaseID: "go-sdk/codingagent/session-persistence", ExecutionMethod: "node --experimental-strip-types parity/oracle/session-persistence.mjs <locked-pi-checkout>", Expected: "the locked Pi SessionManager establishes first-write timing, v3 metadata and message parent chains, explicit reopen append, memory mode, and explicit empty/invalid path behavior", Actual: "PASS; locked Pi produced observation sha256:79e7d83bedc0009fd792a136aa2389b2be3cef00456f4a834e0b35dac2412a55", Platform: "any", CatalogID: catalogID}},
			{InputPath: "parity/oracle/fixtures/session-persistence.json", Evidence: catalog.Evidence{Kind: catalog.MatrixEvidenceGoTest, Ref: "internal/parity/session_persistence_test.go#TestSessionPersistenceParity", Baseline: issue32BaselineCommit, CaseID: "go-sdk/codingagent/session-persistence", ExecutionMethod: "go test ./internal/parity -run '^TestSessionPersistenceParity$' -count=1", Expected: "Pig's public SessionManager observation matches the fixed Pi fixture without semantic normalization", Actual: "PASS; Pig matched observation sha256:79e7d83bedc0009fd792a136aa2389b2be3cef00456f4a834e0b35dac2412a55", Platform: "any", CatalogID: catalogID}},
		}
	case issue32MigrationCatalogID:
		descriptors = []issue32ModuleEvidenceDescriptor{{
			InputPath: "codingagent/session_test.go",
			Evidence:  catalog.Evidence{Kind: "go-test", Ref: "codingagent/session_test.go#TestMigrateSessionEntriesMigratesV1ToV3InPlace", Baseline: issue32BaselineCommit, CaseID: "issue32-codingagent-session-migration-v1-v2-v3", ExecutionMethod: "go test ./codingagent -run '^(TestMigrateSessionEntriesMigratesV1ToV3InPlace|TestMigrateSessionEntriesRenamesV2HookMessageRole)$' -count=1", Expected: "v1 entries acquire a v3 header, generated IDs, parent links, and compaction first-kept IDs; v2 hookMessage becomes custom without altering existing IDs or ancestry", Actual: "PASS; both migration paths produced the expected v3 data and preserved the existing v2 tree", Platform: "any", CatalogID: catalogID},
		}}
	default:
		t.Fatalf("unknown behavior owner %q", catalogID)
	}
	for index := range descriptors {
		inputHash := issue32FileHash(t, descriptors[index].InputPath)
		if descriptors[index].PinnedInputHash != "" {
			inputHash = descriptors[index].PinnedInputHash
		}
		descriptors[index].Evidence.InputHash = inputHash
	}
	return descriptors
}

func issue32EvidenceFromDescriptors(descriptors []issue32ModuleEvidenceDescriptor) []catalog.Evidence {
	evidence := make([]catalog.Evidence, len(descriptors))
	for index := range descriptors {
		evidence[index] = descriptors[index].Evidence
	}
	return evidence
}

func issue32ValidateEvidenceDescriptors(t *testing.T, catalogID string, descriptors []issue32ModuleEvidenceDescriptor, evidence []catalog.Evidence) {
	t.Helper()
	if len(evidence) != len(descriptors) {
		t.Errorf("%s evidence count = %d, want %d", catalogID, len(evidence), len(descriptors))
	}
	want := make(map[string]issue32ModuleEvidenceDescriptor, len(descriptors))
	for _, descriptor := range descriptors {
		want[descriptor.Evidence.Kind+"\x00"+descriptor.Evidence.CaseID] = descriptor
	}
	seen := map[string]bool{}
	for _, item := range evidence {
		if item.Kind == "" || item.Ref == "" || item.Baseline == "" || item.CaseID == "" || item.InputHash == "" || item.ExecutionMethod == "" || item.Expected == "" || item.Actual == "" || item.Platform == "" || item.CatalogID == "" {
			t.Errorf("%s evidence %q has an incomplete replay record: %+v", catalogID, item.CaseID, item)
		}
		key := item.Kind + "\x00" + item.CaseID
		descriptor, ok := want[key]
		if !ok {
			t.Errorf("%s has unexpected evidence case %q", catalogID, item.CaseID)
			continue
		}
		if item != descriptor.Evidence {
			t.Errorf("%s evidence %q = %+v, want %+v", catalogID, item.CaseID, item, descriptor.Evidence)
		}
		if seen[key] {
			t.Errorf("%s has duplicate evidence kind/case %q/%q", catalogID, item.Kind, item.CaseID)
		}
		seen[key] = true
		if item.Baseline != issue32BaselineCommit || item.CatalogID != catalogID {
			t.Errorf("%s evidence %q binding = baseline %q catalog %q", catalogID, item.CaseID, item.Baseline, item.CatalogID)
		}
		wantHash := issue32FileHash(t, descriptor.InputPath)
		if descriptor.PinnedInputHash != "" {
			wantHash = descriptor.PinnedInputHash
		}
		if item.InputHash != wantHash {
			t.Errorf("%s evidence %q input hash = %q, want %q", catalogID, item.CaseID, item.InputHash, wantHash)
		}
	}
}

func issue32ModuleEvidenceDescriptors(t *testing.T) []issue32ModuleEvidenceDescriptor {
	t.Helper()
	descriptors := []issue32ModuleEvidenceDescriptor{
		{
			InputPath:       "parity/surface/symbols.jsonl",
			PinnedInputHash: issue32SurfaceHash,
			Evidence: catalog.Evidence{
				Kind:            "go-test",
				Ref:             "codingagent/issue32_surface_test.go#TestIssue32MemberMappingsMatchLockedCodingAgentSurface",
				Baseline:        issue32BaselineCommit,
				CaseID:          "issue32-codingagent-locked-surface",
				ExecutionMethod: "go test ./codingagent -run '^(TestIssue32MappingsMatchLockedCodingAgentSurface|TestIssue32ExportSubpathsUseCanonicalGoPackage|TestIssue32MemberMappingsMatchLockedCodingAgentSurface|TestIssue32ScaffoldedMemberTargetsResolve|TestIssue32InheritedTUIMemberProjectionsResolve|TestIssue32ScaffoldedStaticMemberTargetsResolve)$' -count=1",
				Expected:        "all 376 pinned Coding Agent symbols, 2,172 extracted instance/type members, and 14 extracted static members across root and ./client map to resolvable codingagent declarations or explicit inherited lower-layer TUI carriers",
				Actual:          "PASS; all 376 pinned symbols, 2,172 extracted instance/type members, and 14 extracted static members mapped to resolvable codingagent declarations or explicit inherited lower-layer TUI carriers",
				Platform:        "darwin/linux",
				CatalogID:       issue32ModuleCatalogID,
			},
		},
		{
			InputPath: "codingagent/static_factories_review_test.go",
			Evidence: catalog.Evidence{
				Kind:            "go-test",
				Ref:             "codingagent/static_factories_review_test.go#TestStaticFactoryProjectionsAreInertCapabilityStubs",
				Baseline:        issue32BaselineCommit,
				CaseID:          "issue32-codingagent-static-factory-projections",
				ExecutionMethod: "go test ./codingagent -run '^(TestStaticFactoryProjectionsAreInertCapabilityStubs|TestNewInMemorySessionManagerRemainsFunctional|TestStaticFactorySupportingCarriersPreserveAbsence)$' -count=1",
				Expected:        "all 14 static-member package-function projections have exact signatures; 11 deferred factories return zero values and structured ErrNotImplemented without dependencies, callbacks, transports, or host I/O, while create, open, and in-memory Session factories remain functional",
				Actual:          "PASS; all 14 static factory signatures matched, 11 deferred factories were inert structured capability stubs, and the three implemented SessionManager factories remained functional",
				Platform:        "any",
				CatalogID:       issue32ModuleCatalogID,
			},
		},
		{
			InputPath: "codingagent/sdk_test.go",
			Evidence: catalog.Evidence{
				Kind:            "go-test",
				Ref:             "codingagent/sdk_test.go#TestSettingsResourcePackageAndTrustStubsDoNotTouchHostState",
				Baseline:        issue32BaselineCommit,
				CaseID:          "issue32-codingagent-capability-stub-side-effects",
				ExecutionMethod: "go test ./codingagent -run '^(TestCreateAgentSessionAssemblyStubsAreSideEffectFree|TestModelRuntimeStubsDoNotReadCredentialsModelsOrNetwork|TestSettingsResourcePackageAndTrustStubsDoNotTouchHostState|TestDefaultReadToolFactoryConstructionIsSideEffectFree|TestRemoteSessionStubsDoNotCreateTransportOrLeaseState|TestAmbientStateCapabilityStubsCannotImportHostIO)$' -count=1",
				Expected:        "the covered assembly, model, settings, resource, package, trust, and remote-session stubs remain inert while constructing the live read Tool performs no host I/O; no path starts transports or mutates sentinel files or client state",
				Actual:          "PASS; focused SDK stub checks remained inert, the read Tool factory constructed metadata without reading a file, and no credential, model-store, filesystem, transport, client-state, or forbidden ambient-state I/O occurred",
				Platform:        "any",
				CatalogID:       issue32ModuleCatalogID,
			},
		},
		{
			InputPath: "codingagent/session_test.go",
			Evidence: catalog.Evidence{
				Kind:            "go-test",
				Ref:             "codingagent/session_test.go#TestAgentSessionComposesLegacyAgentAndV3Session",
				Baseline:        issue32BaselineCommit,
				CaseID:          "issue32-codingagent-production-v3-session",
				ExecutionMethod: "go test ./codingagent -run '^(TestParseSessionEntriesDecodesV3MessageDiscriminators|TestBuildSessionContextUsesLatestV3CompactionAndFullPathSettings|TestMigrateSessionEntriesMigratesV1ToV3InPlace|TestMigrateSessionEntriesRenamesV2HookMessageRole|TestAgentSessionComposesLegacyAgentAndV3Session|TestAgentSessionPromptPropagatesUnconfiguredAgentErrorAndSettles|TestAgentSessionLifecycleAndUnsupportedQueueMethodsHaveNoQueueSideEffects)$' -count=1",
				Expected:        "production AgentSession composes the legacy agent.Agent with the v3 SessionManager, preserves v3 parsing and migration semantics, settles failed prompts, and leaves unsupported queue operations inert",
				Actual:          "PASS; focused production-session tests decoded and migrated v3 entries, retained the legacy Agent and v3 SessionManager, settled failed prompts, and observed no queue mutation from lifecycle or unsupported operations",
				Platform:        "any",
				CatalogID:       issue32ModuleCatalogID,
			},
		},
		{
			InputPath: "codingagent/agent_session_runtime_test.go",
			Evidence: catalog.Evidence{
				Kind:            "go-test",
				Ref:             "codingagent/agent_session_runtime_test.go#TestCreateAgentSessionRunsTextRoundAndSettlesAfterTranscriptUpdate",
				Baseline:        issue32BaselineCommit,
				CaseID:          "issue55-codingagent-in-memory-agent-session",
				ExecutionMethod: "go test -race ./codingagent -run '^(TestCreateAgentSessionRunsTextRoundAndSettlesAfterTranscriptUpdate|TestCreateAgentSessionRunsReadContinuationWithInjectedProviderAndTool|TestAgentSessionAbortRequestsCancellationAndWaitForIdleObservesSettledTranscript|TestAgentSessionAbortIsSafeFromSynchronousListeners|TestAgentSessionDisposeCancelsWithoutDroppingActiveTranscript|TestAgentSessionUnsupportedPromptOptionsAreExactInertStubs|TestCreateAgentSessionNoToolsModesMatchPublicSelectionContract)$' -count=1",
				Expected:        "the public SDK runs text and read continuation with explicit dependencies, ordered lifecycle events, post-transcript settlement, cancellation, and no ambient disk writes",
				Actual:          "PASS; injected sessions completed text and read rounds, settled after transcript updates, aborted deterministically, and left the sentinel filesystem unchanged under the race detector",
				Platform:        "any",
				CatalogID:       issue32ModuleCatalogID,
			},
		},
		{
			InputPath: "cmd/pig/headless_process_test.go",
			Evidence: catalog.Evidence{
				Kind:            "go-test",
				Ref:             "cmd/pig/headless_process_test.go#TestPigProcessRunsHeadlessTextWithExplicitDeepSeekInputs",
				Baseline:        issue32BaselineCommit,
				CaseID:          "issue56-codingagent-headless-text-product-path",
				ExecutionMethod: "go test ./cmd/pig -run '^TestPigProcess' -count=1",
				Expected:        "the real pig process assembles explicit DeepSeek model and credential inputs into a pure in-memory AgentSession, emits final text only, preserves stable failures, and propagates SIGINT",
				Actual:          "PASS; local DeepSeek text used explicit and ambient credentials, stdout contained only the final Assistant text, failures stayed stable, and SIGINT exited 130",
				Platform:        "darwin/linux",
				CatalogID:       issue32ModuleCatalogID,
			},
		},
		{
			InputPath: "cmd/pig/issue71_process_test.go",
			Evidence: catalog.Evidence{
				Kind: catalog.MatrixEvidenceGoTest, Ref: "cmd/pig/issue71_process_test.go#TestPigProcessesPersistAndReopenExplicitSessionPath", Baseline: issue32BaselineCommit,
				CaseID: "issue71-codingagent-session-persistence-product-path", ExecutionMethod: "go test ./cmd/pig -run '^TestPig(ProcessesPersistAndReopenExplicitSessionPath|NoSessionDoesNotCreatePigState)$' -count=1",
				Expected: "two real pig processes persist and reopen one explicit v3 Session while --no-session creates no Pig state",
				Actual:   "PASS; the second process sent the first round to the Provider, appended without duplicate records, and explicit memory mode left Pig state absent", Platform: "darwin/linux", CatalogID: issue32ModuleCatalogID,
			},
		},
		{
			InputPath: "cmd/pig/headless_process_test.go",
			Evidence: catalog.Evidence{
				Kind:            "go-test",
				Ref:             "cmd/pig/headless_process_test.go#TestPigProcessStreamsSessionFirstHeadlessJSON",
				Baseline:        issue32BaselineCommit,
				CaseID:          "issue57-codingagent-headless-json-product-path",
				ExecutionMethod: "go test ./cmd/pig -run '^TestPigProcess.*HeadlessJSON|^TestPigProcessTreatsPipedJSONAsPromptNotRPCCommand$' -count=1",
				Expected:        "the real pig process emits session-first projected JSONL for text, Tool, Provider error, and cancellation while preserving the one-way stdin and RPC boundary",
				Actual:          "PASS; JSONL records were parseable, golden-stable and Session-ordered, Provider error stayed in-band with exit 0, SIGINT ended with exit 130, and piped JSON remained a prompt",
				Platform:        "darwin/linux",
				CatalogID:       issue32ModuleCatalogID,
			},
		},
		{
			InputPath: "codingagent/tools_review_test.go",
			Evidence: catalog.Evidence{
				Kind:            "go-test",
				Ref:             "codingagent/tools_review_test.go#TestWithFileMutationQueueIsCapabilityStub",
				Baseline:        issue32BaselineCommit,
				CaseID:          "issue32-codingagent-file-mutation-queue-stub",
				ExecutionMethod: "go test ./codingagent -run '^TestWithFileMutationQueueIsCapabilityStub$' -count=1",
				Expected:        "WithFileMutationQueue returns the generic zero value and a structured codingagent.WithFileMutationQueue ErrNotImplemented without inspecting the invalid path or invoking the callback",
				Actual:          "PASS; WithFileMutationQueue returned the zero value and structured ErrNotImplemented without inspecting an invalid-NUL path or invoking the callback",
				Platform:        "any",
				CatalogID:       issue32ModuleCatalogID,
			},
		},
		{
			InputPath: "codingagent/extensions_abi_final_review_test.go",
			Evidence: catalog.Evidence{
				Kind:            "go-test",
				Ref:             "codingagent/extensions_abi_final_review_test.go#TestExtensionBoundaryDeclaresNoExecutableFunctionFields",
				Baseline:        issue32BaselineCommit,
				CaseID:          "issue32-codingagent-extension-abi-stubs",
				ExecutionMethod: "go test ./codingagent -run '^(TestExtensionBoundaryDeclaresNoExecutableFunctionFields|TestExtensionExecutableTypesRemainOpaqueUntilM7|TestExtensionOperationSlotsUseOpaqueCarrier|TestKeybindingsManagerEffectiveConfigIsExplicitlyUnavailable)$' -count=1",
				Expected:        "the extension boundary exposes no executable function declarations or fields before M7, executable concepts and operation slots remain opaque carriers, and effective keybinding lookup is explicitly unavailable",
				Actual:          "PASS; extension ABI declarations remained non-executable and opaque, operation slots used the opaque carrier, and effective keybinding lookup returned structured ErrNotImplemented",
				Platform:        "any",
				CatalogID:       issue32ModuleCatalogID,
			},
		},
	}
	for i := range descriptors {
		descriptor := &descriptors[i]
		inputHash := issue32FileHash(t, descriptor.InputPath)
		if descriptor.PinnedInputHash != "" {
			if inputHash != descriptor.PinnedInputHash {
				t.Fatalf("evidence %q declared input %s hash = %q, want pinned %q", descriptor.Evidence.CaseID, descriptor.InputPath, inputHash, descriptor.PinnedInputHash)
			}
			inputHash = descriptor.PinnedInputHash
		}
		descriptor.Evidence.InputHash = inputHash
	}
	return descriptors
}

func issue32ModuleEvidence(t *testing.T) []catalog.Evidence {
	t.Helper()
	descriptors := issue32ModuleEvidenceDescriptors(t)
	evidence := make([]catalog.Evidence, len(descriptors))
	for i := range descriptors {
		evidence[i] = descriptors[i].Evidence
	}
	return evidence
}

func issue32WriteCatalog(t *testing.T, root string, expected []catalog.Entry) {
	t.Helper()
	path := filepath.Join(root, "parity", "catalog.jsonl")
	entries, err := catalog.LoadCatalog(path)
	if err != nil {
		t.Fatal(err)
	}
	kept := entries[:0]
	for _, e := range entries {
		if issue32CatalogID(e.ID) {
			continue
		}
		if e.ID == issue32ModuleCatalogID {
			e.Status = catalog.StatusPartial
			e.Evidence = issue32ModuleEvidence(t)
			e.Partial = issue32ModulePartial()
			e.Notes = issue32ModuleNotes()
		}
		kept = append(kept, e)
	}
	for _, owner := range issue32BehaviorOwnerEntries(t) {
		replaced := false
		for index := range kept {
			if kept[index].ID == owner.ID {
				kept[index] = owner
				replaced = true
				break
			}
		}
		if !replaced {
			kept = append(kept, owner)
		}
	}
	data, err := catalog.EncodeEntries(append(kept, expected...))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func issue32ModulePartial() *catalog.Partial {
	return &catalog.Partial{
		Supported: []string{
			"all fixed-snapshot Coding Agent symbols, instance/type members, static members, and public constructors have compile-usable Go mappings",
			"the public Go SDK runs an injected in-memory AgentSession for text and read Tool continuation with deterministic lifecycle events",
			"the real pig process runs Headless text with explicit DeepSeek model and credentials, final-text stdout, stable errors, and SIGINT exit 130",
			"the real pig process runs one-way session-first Headless JSONL with projected ordered events for text, Tool, Provider error, and cancellation",
			"public SDK and real pig processes create, append, reopen, and continue v3 Sessions while explicit --no-session remains side-effect-free",
			"global SettingsManager and Headless startup consume saved model/thinking and Session paths with locked saves and no project settings access",
			"Capability Stubs perform no ambient state, credential, resource, package, network, event, timer, or goroutine side effects",
		},
		Unsupported: []string{
			"continue/recent lookup, fork, project settings, resources, packages, remaining tools, ambient model/auth assembly, interactive mode, and RPC remain explicit Capability Stubs until their roadmap milestones",
			"surface tests prove API coverage and target resolution, not runtime parity",
		},
	}
}

func issue32ModuleNotes() string {
	return "Issue #32 maps all 376 symbols, 2,172 instance/type members, 14 static members, and 38 constructors across root and ./client into the canonical Go codingagent package. The public text read Tool is live under contract:codingagent/read-tool, issue #55 adds the injected in-memory legacy AgentSession, issues #56/#57 add real Headless text and session-first JSONL, issue #71 adds Pig-owned v3 create/open persistence, and issue #74 adds global settings-driven Headless startup; the remaining deferred operations stay Capability Stubs. The production v3 session path remains separate from Harness v4. Stubs do not read .pig, credential, project setting, resource or package state and produce no side effects."
}

var issue32NameExceptions = map[string]string{
	"RpcClient":        "RPCClient",
	"RpcClientOptions": "RPCClientOptions",
}
var issue32SymbolTargetExceptions = map[string]string{
	"symbol:codingagent/src/config.ts#VERSION":                  "Version",
	"symbol:codingagent/src/utils/clipboard.ts#copyToClipboard": "CopyToClipboard",
}

var issue32StaticMemberTargetExceptions = map[string]string{
	"RemoteSession.create":          "CreateRemoteSession",
	"RemoteSession.open":            "OpenRemoteSession",
	"KeybindingsManager.create":     "NewKeybindingsManager",
	"ModelRuntime.create":           "NewModelRuntime",
	"SessionManager.continueRecent": "ContinueRecentSessionManager",
	"SessionManager.create":         "NewSessionManager",
	"SessionManager.forkFrom":       "ForkSessionManager",
	"SessionManager.inMemory":       "NewInMemorySessionManager",
	"SessionManager.list":           "ListSessions",
	"SessionManager.listAll":        "ListAllSessions",
	"SessionManager.open":           "OpenSessionManager",
	"SettingsManager.create":        "NewSettingsManager",
	"SettingsManager.fromStorage":   "NewSettingsManagerFromStorage",
	"SettingsManager.inMemory":      "NewInMemorySettingsManager",
}

type issue32AliasMemberSet struct {
	target  string
	members []string
}

// These declarations deliberately alias representation-neutral Agent
// contracts. The local AST index cannot load members through imported
// selectors, so pin each exact alias and the fields established by its
// contract.
var issue32AliasMembers = map[string]issue32AliasMemberSet{
	"CompactionSettings": {
		target:  "agent.CompactionSettings",
		members: []string{"Enabled", "KeepRecentTokens", "ReserveTokens"},
	},
	"TruncationOptions": {
		target:  "agent.TruncationOptions",
		members: []string{"MaxBytes", "MaxLines"},
	},
	"TruncationResult": {
		target: "agent.TruncationResult",
		members: []string{
			"Content", "FirstLineExceedsLimit", "LastLinePartial",
			"MaxBytes", "MaxLines", "OutputBytes", "OutputLines",
			"TotalBytes", "TotalLines", "Truncated", "TruncatedBy",
		},
	},
}

type issue32MemberProjection struct {
	packagePath    string
	target         string
	reason         string
	classification string
}

func issue32MemberProjectionFor(symbol, member string) (issue32MemberProjection, bool) {
	if projection, ok := issue32MemberProjectionExceptions[symbol+"."+member]; ok {
		return projection, true
	}
	if overrides, ok := issue32ContainerProjectionSymbols[symbol]; ok {
		if target, inherited := issue32ContainerMembers[member]; inherited && !overrides[member] {
			return issue32TUIProjection("Container."+target, "the inherited Pi TUI Container contract is reused from the lower-layer tui package rather than duplicated in Coding Agent"), true
		}
	}
	if issue32BoxProjectionSymbols[symbol] {
		if target, inherited := issue32BoxMembers[member]; inherited {
			return issue32TUIProjection("Box."+target, "the inherited Pi TUI Box contract is reused from the lower-layer tui package rather than duplicated in Coding Agent"), true
		}
	}
	if symbol == "CustomEditor" {
		if target, inherited := issue32EditorMembers[member]; inherited {
			return issue32TUIProjection("Editor."+target, "the inherited Pi TUI Editor contract is reused from the lower-layer tui package rather than duplicated in Coding Agent"), true
		}
	}
	if symbol == "KeybindingsManager" {
		if target, inherited := issue32KeybindingsManagerMembers[member]; inherited {
			return issue32TUIProjection("KeybindingsManager."+target, "the inherited Pi TUI KeybindingsManager contract is reused from the lower-layer tui package rather than duplicated in Coding Agent"), true
		}
	}
	return issue32MemberProjection{}, false
}

func issue32TUIProjection(target, reason string) issue32MemberProjection {
	return issue32MemberProjection{packagePath: issue32TUIPackage, target: target, reason: reason}
}

var issue32ContainerMembers = map[string]string{
	"addChild":    "AddChild",
	"children":    "Children",
	"clear":       "Clear",
	"invalidate":  "Invalidate",
	"removeChild": "RemoveChild",
	"render":      "Render",
}

// Values name subtype overrides that must stay mapped to Coding Agent. A nil
// set means the subtype inherits all six Container members.
var issue32ContainerProjectionSymbols = map[string]map[string]bool{
	"AssistantMessageComponent":    {"invalidate": true, "render": true},
	"BashExecutionComponent":       {"invalidate": true},
	"BorderedLoader":               nil,
	"CustomMessageComponent":       {"invalidate": true},
	"ExtensionEditorComponent":     nil,
	"ExtensionInputComponent":      nil,
	"ExtensionSelectorComponent":   nil,
	"LoginDialogComponent":         nil,
	"ModelSelectorComponent":       nil,
	"OAuthSelectorComponent":       nil,
	"SessionSelectorComponent":     nil,
	"SettingsSelectorComponent":    nil,
	"ShowImagesSelectorComponent":  nil,
	"ThemeSelectorComponent":       nil,
	"ThinkingSelectorComponent":    nil,
	"ToolExecutionComponent":       {"invalidate": true, "render": true},
	"TreeSelectorComponent":        nil,
	"UserMessageSelectorComponent": nil,
	"UserMessageComponent":         {"render": true},
}

var issue32BoxProjectionSymbols = map[string]bool{
	"BranchSummaryMessageComponent":     true,
	"CompactionSummaryMessageComponent": true,
	"SkillInvocationMessageComponent":   true,
}

var issue32BoxMembers = map[string]string{
	"addChild":    "AddChild",
	"children":    "Children",
	"clear":       "Clear",
	"removeChild": "RemoveChild",
	"render":      "Render",
	"setBgFn":     "SetBGFunc",
}

var issue32EditorMembers = map[string]string{
	"addToHistory":              "AddToHistory",
	"borderColor":               "BorderColor",
	"disableSubmit":             "DisableSubmit",
	"focused":                   "Focused",
	"getAutocompleteMaxVisible": "GetAutocompleteMaxVisible",
	"getCursor":                 "GetCursor",
	"getExpandedText":           "GetExpandedText",
	"getLines":                  "GetLines",
	"getPaddingX":               "GetPaddingX",
	"getText":                   "GetText",
	"insertTextAtCursor":        "InsertTextAtCursor",
	"invalidate":                "Invalidate",
	"isShowingAutocomplete":     "IsShowingAutocomplete",
	"onChange":                  "OnChange",
	"onSubmit":                  "OnSubmit",
	"render":                    "Render",
	"setAutocompleteMaxVisible": "SetAutocompleteMaxVisible",
	"setAutocompleteProvider":   "SetAutocompleteProvider",
	"setPaddingX":               "SetPaddingX",
	"setText":                   "SetText",
}

var issue32KeybindingsManagerMembers = map[string]string{
	"getConflicts":        "GetConflicts",
	"getDefinition":       "GetDefinition",
	"getKeys":             "GetKeys",
	"getResolvedBindings": "GetResolvedBindings",
	"getUserBindings":     "GetUserBindings",
	"matches":             "Matches",
	"setUserBindings":     "SetUserBindings",
}

var issue32MemberProjectionExceptions = map[string]issue32MemberProjection{
	"SessionManager._persist": {
		target:         "SessionManager",
		reason:         "the technically reachable TypeScript method is an intended-private persistence helper whose Go implementation is deferred; SessionManager is its destination and no filesystem behavior is introduced",
		classification: "private-impl",
	},
	"CreateAgentSessionServicesOptions.modelRuntimeSignal": {
		target: "CreateAgentSessionServices",
		reason: "the leading context.Context parameter of CreateAgentSessionServices carries cancellation",
	},
	"CreateModelRuntimeOptions.signal": {
		target: "NewModelRuntime",
		reason: "the leading context.Context parameter of NewModelRuntime carries cancellation",
	},
	"ExecOptions.signal": {
		target: "ExtensionAPI.Exec",
		reason: "cancellation belongs to the opaque blocking Exec operation rather than a duplicated option field before the extension ABI is implemented",
	},
	"ExtensionCommandContext.signal": {
		target: "ExtensionContextActions.GetSignal",
		reason: "the explicit GetSignal action is the cancellation carrier shared by extension contexts",
	},
	"ExtensionContext.signal": {
		target: "ExtensionContextActions.GetSignal",
		reason: "the explicit GetSignal action is the cancellation carrier shared by extension contexts",
	},
	"ExtensionUIDialogOptions.signal": {
		target: "ExtensionUIContext",
		reason: "dialog cancellation belongs to the opaque blocking UI operation carrier before the extension ABI is implemented",
	},
	"ExtensionRunner.bindCommandContext":                  {target: "ExtensionRunner", reason: "ADR-0009 defers extension context binding to the M7 runtime ABI decision"},
	"ExtensionRunner.bindCore":                            {target: "ExtensionRunner", reason: "ADR-0009 defers core context binding to the M7 runtime ABI decision"},
	"ExtensionRunner.createCommandContext":                {target: "ExtensionRunner", reason: "ADR-0009 defers extension context construction to the M7 runtime ABI decision"},
	"ExtensionRunner.createContext":                       {target: "ExtensionRunner", reason: "ADR-0009 defers extension context construction to the M7 runtime ABI decision"},
	"ExtensionRunner.emit":                                {target: "ExtensionRunner", reason: "ADR-0009 defers event dispatch to the M7 runtime ABI decision"},
	"ExtensionRunner.emitBeforeAgentStart":                {target: "ExtensionRunner", reason: "ADR-0009 defers event dispatch to the M7 runtime ABI decision"},
	"ExtensionRunner.emitBeforeProviderHeaders":           {target: "ExtensionRunner", reason: "ADR-0009 defers extension hook dispatch to the M7 runtime ABI decision"},
	"ExtensionRunner.emitBeforeProviderRequest":           {target: "ExtensionRunner", reason: "ADR-0009 defers extension hook dispatch to the M7 runtime ABI decision"},
	"ExtensionRunner.emitContext":                         {target: "ExtensionRunner", reason: "ADR-0009 defers event dispatch to the M7 runtime ABI decision"},
	"ExtensionRunner.emitError":                           {target: "ExtensionRunner", reason: "ADR-0009 defers callback error isolation to the M7 runtime ABI decision"},
	"ExtensionRunner.emitInput":                           {target: "ExtensionRunner", reason: "ADR-0009 defers input callback dispatch to the M7 runtime ABI decision"},
	"ExtensionRunner.emitMessageEnd":                      {target: "ExtensionRunner", reason: "ADR-0009 defers event dispatch to the M7 runtime ABI decision"},
	"ExtensionRunner.emitResourcesDiscover":               {target: "ExtensionRunner", reason: "ADR-0009 defers resource callback dispatch to the M7 runtime ABI decision"},
	"ExtensionRunner.emitToolCall":                        {target: "ExtensionRunner", reason: "ADR-0009 defers tool callback dispatch to the M7 runtime ABI decision"},
	"ExtensionRunner.emitToolResult":                      {target: "ExtensionRunner", reason: "ADR-0009 defers tool callback dispatch to the M7 runtime ABI decision"},
	"ExtensionRunner.emitUserBash":                        {target: "ExtensionRunner", reason: "ADR-0009 defers shell callback dispatch to the M7 runtime ABI decision"},
	"ExtensionRunner.getActiveTools":                      {target: "ExtensionRunner", reason: "ADR-0009 defers runtime state access to the M7 runtime ABI decision"},
	"ExtensionRunner.getAllRegisteredTools":               {target: "ExtensionRunner", reason: "ADR-0009 defers runtime registry access to the M7 runtime ABI decision"},
	"ExtensionRunner.getCommand":                          {target: "ExtensionRunner", reason: "ADR-0009 defers runtime registry access to the M7 runtime ABI decision"},
	"ExtensionRunner.getCommandDiagnostics":               {target: "ExtensionRunner", reason: "ADR-0009 defers runtime diagnostics access to the M7 runtime ABI decision"},
	"ExtensionRunner.getEntryRenderer":                    {target: "ExtensionRunner", reason: "ADR-0009 defers UI callback representation to the M7 runtime ABI decision"},
	"ExtensionRunner.getExtensionPaths":                   {target: "ExtensionRunner", reason: "ADR-0009 defers loaded-runtime state to the M7 runtime ABI decision"},
	"ExtensionRunner.getFlagValues":                       {target: "ExtensionRunner", reason: "ADR-0009 defers runtime state access to the M7 runtime ABI decision"},
	"ExtensionRunner.getFlags":                            {target: "ExtensionRunner", reason: "ADR-0009 defers runtime registry access to the M7 runtime ABI decision"},
	"ExtensionRunner.getMarkdownTransformers":             {target: "ExtensionRunner", reason: "ADR-0009 defers callback representation to the M7 runtime ABI decision"},
	"ExtensionRunner.getMessageRenderer":                  {target: "ExtensionRunner", reason: "ADR-0009 defers UI callback representation to the M7 runtime ABI decision"},
	"ExtensionRunner.getModelRegistry":                    {target: "ExtensionRunner", reason: "ADR-0009 defers runtime registry access to the M7 runtime ABI decision"},
	"ExtensionRunner.getRegisteredCommands":               {target: "ExtensionRunner", reason: "ADR-0009 defers runtime registry access to the M7 runtime ABI decision"},
	"ExtensionRunner.getShortcutDiagnostics":              {target: "ExtensionRunner", reason: "ADR-0009 defers runtime diagnostics access to the M7 runtime ABI decision"},
	"ExtensionRunner.getShortcuts":                        {target: "ExtensionRunner", reason: "ADR-0009 defers runtime registry access to the M7 runtime ABI decision"},
	"ExtensionRunner.getToolDefinition":                   {target: "ExtensionRunner", reason: "ADR-0009 defers tool dispatch representation to the M7 runtime ABI decision"},
	"ExtensionRunner.getUIContext":                        {target: "ExtensionRunner", reason: "ADR-0009 defers UI callback representation to the M7 runtime ABI decision"},
	"ExtensionRunner.hasHandlers":                         {target: "ExtensionRunner", reason: "ADR-0009 defers callback registry state to the M7 runtime ABI decision"},
	"ExtensionRunner.hasUI":                               {target: "ExtensionRunner", reason: "ADR-0009 defers UI runtime state to the M7 runtime ABI decision"},
	"ExtensionRunner.invalidate":                          {target: "ExtensionRunner", reason: "ADR-0009 defers lifecycle semantics to the M7 runtime ABI decision"},
	"ExtensionRunner.onError":                             {target: "ExtensionRunner", reason: "ADR-0009 defers callback and error-isolation semantics to the M7 runtime ABI decision"},
	"ExtensionRunner.setFlagValue":                        {target: "ExtensionRunner", reason: "ADR-0009 defers runtime state mutation to the M7 runtime ABI decision"},
	"ExtensionRunner.setUIContext":                        {target: "ExtensionRunner", reason: "ADR-0009 defers UI callback representation and lifecycle to the M7 runtime ABI decision"},
	"ExtensionRunner.shutdown":                            {target: "ExtensionRunner", reason: "ADR-0009 defers lifecycle semantics to the M7 runtime ABI decision"},
	"ExtensionRuntime.appendEntry":                        {target: "ExtensionRuntime", reason: "ADR-0009 defers the host/runtime binding layout to the M7 runtime ABI decision"},
	"ExtensionRuntime.assertActive":                       {target: "ExtensionRuntime", reason: "ADR-0009 defers lifecycle semantics to the M7 runtime ABI decision"},
	"ExtensionRuntime.flagValues":                         {target: "ExtensionRuntime", reason: "ADR-0009 defers runtime state representation to the M7 runtime ABI decision"},
	"ExtensionRuntime.getActiveTools":                     {target: "ExtensionRuntime", reason: "ADR-0009 defers the host/runtime binding layout to the M7 runtime ABI decision"},
	"ExtensionRuntime.getAllTools":                        {target: "ExtensionRuntime", reason: "ADR-0009 defers the host/runtime binding layout to the M7 runtime ABI decision"},
	"ExtensionRuntime.getCommands":                        {target: "ExtensionRuntime", reason: "ADR-0009 defers the host/runtime binding layout to the M7 runtime ABI decision"},
	"ExtensionRuntime.getSessionName":                     {target: "ExtensionRuntime", reason: "ADR-0009 defers the host/runtime binding layout to the M7 runtime ABI decision"},
	"ExtensionRuntime.getThinkingLevel":                   {target: "ExtensionRuntime", reason: "ADR-0009 defers the host/runtime binding layout to the M7 runtime ABI decision"},
	"ExtensionRuntime.invalidate":                         {target: "ExtensionRuntime", reason: "ADR-0009 defers lifecycle semantics to the M7 runtime ABI decision"},
	"ExtensionRuntime.pendingNativeProviderRegistrations": {target: "ExtensionRuntime", reason: "ADR-0009 defers runtime state representation to the M7 runtime ABI decision"},
	"ExtensionRuntime.pendingProviderRegistrations":       {target: "ExtensionRuntime", reason: "ADR-0009 defers runtime state representation to the M7 runtime ABI decision"},
	"ExtensionRuntime.refreshTools":                       {target: "ExtensionRuntime", reason: "ADR-0009 defers the host/runtime binding layout to the M7 runtime ABI decision"},
	"ExtensionRuntime.registerNativeProvider":             {target: "ExtensionRuntime", reason: "ADR-0009 defers provider dispatch and binding to the M7 runtime ABI decision"},
	"ExtensionRuntime.registerProvider":                   {target: "ExtensionRuntime", reason: "ADR-0009 defers provider dispatch and binding to the M7 runtime ABI decision"},
	"ExtensionRuntime.sendMessage":                        {target: "ExtensionRuntime", reason: "ADR-0009 defers the host/runtime binding layout to the M7 runtime ABI decision"},
	"ExtensionRuntime.sendUserMessage":                    {target: "ExtensionRuntime", reason: "ADR-0009 defers the host/runtime binding layout to the M7 runtime ABI decision"},
	"ExtensionRuntime.setActiveTools":                     {target: "ExtensionRuntime", reason: "ADR-0009 defers runtime state mutation to the M7 runtime ABI decision"},
	"ExtensionRuntime.setLabel":                           {target: "ExtensionRuntime", reason: "ADR-0009 defers runtime state mutation to the M7 runtime ABI decision"},
	"ExtensionRuntime.setModel":                           {target: "ExtensionRuntime", reason: "ADR-0009 defers runtime state mutation to the M7 runtime ABI decision"},
	"ExtensionRuntime.setSessionName":                     {target: "ExtensionRuntime", reason: "ADR-0009 defers runtime state mutation to the M7 runtime ABI decision"},
	"ExtensionRuntime.setThinkingLevel":                   {target: "ExtensionRuntime", reason: "ADR-0009 defers runtime state mutation to the M7 runtime ABI decision"},
	"ExtensionRuntime.trackEventBusSubscription":          {target: "ExtensionRuntime", reason: "ADR-0009 defers callback lifecycle and unsubscribe semantics to the M7 runtime ABI decision"},
	"ExtensionRuntime.unregisterProvider":                 {target: "ExtensionRuntime", reason: "ADR-0009 defers provider lifecycle and binding to the M7 runtime ABI decision"},
	"GenerateBranchSummaryOptions.signal": {
		target: "GenerateBranchSummary",
		reason: "the leading context.Context parameter of GenerateBranchSummary carries cancellation",
	},
	"SessionBeforeCompactEvent.signal": {
		target: "AgentSession.Compact",
		reason: "the context.Context parameter of the blocking compaction operation carries cancellation rather than duplicated event data",
	},
	"SessionBeforeTreeEvent.signal": {
		target: "ExtensionCommandContextActions.NavigateTree",
		reason: "cancellation belongs to the opaque blocking tree-navigation operation rather than duplicated event data before that ABI is implemented",
	},
}

var issue32MemberTargetExceptions = map[string]string{
	// Optional TypeScript interface members use a companion Go interface so
	// implementations of the required operations remain narrow.
	"ReadOperations.detectImageMimeType": "ReadImageOperations.DetectImageMimeType",
	// Keep the Go spelling of the English word Idle instead of treating its
	// leading letters as the identifier initialism ID.
	"AgentSession.isIdle":                        "AgentSession.IsIdle",
	"AgentSession.waitForIdle":                   "AgentSession.WaitForIdle",
	"ExtensionCommandContext.isIdle":             "ExtensionCommandContext.IsIdle",
	"ExtensionCommandContext.waitForIdle":        "ExtensionCommandContext.WaitForIdle",
	"ExtensionCommandContextActions.waitForIdle": "ExtensionCommandContextActions.WaitForIdle",
	"ExtensionContext.isIdle":                    "ExtensionContext.IsIdle",
	"ExtensionContextActions.isIdle":             "ExtensionContextActions.IsIdle",
	"RpcClient.waitForIdle":                      "RPCClient.WaitForIdle",
	// Preserve the common all-caps UI initialism.
	"ExtensionCommandContext.ui": "ExtensionCommandContext.UI",
	"ExtensionContext.ui":        "ExtensionContext.UI",
	"ProjectTrustContext.ui":     "ProjectTrustContext.UI",
	// Preserve the HTTP and millisecond initialisms without rewriting the word
	// Idle as IDle.
	"SettingsCallbacks.onHttpIdleTimeoutMsChange": "SettingsCallbacks.OnHTTPIdleTimeoutMSChange",
	"SettingsConfig.httpIdleTimeoutMs":            "SettingsConfig.HTTPIdleTimeoutMS",
	"SettingsManager.getHttpIdleTimeoutMs":        "SettingsManager.GetHTTPIdleTimeoutMS",
	"SettingsManager.setHttpIdleTimeoutMs":        "SettingsManager.SetHTTPIdleTimeoutMS",
	// YAML frontmatter keys may contain hyphens; Go fields may not.
	"SkillFrontmatter.disable-model-invocation": "SkillFrontmatter.DisableModelInvocation",
}
var issue32NumberInheritedMembers = []string{"toLocaleString", "toString", "valueOf"}
var issue32StringInheritedMembers = []string{"anchor", "at", "big", "blink", "bold", "charAt", "charCodeAt", "codePointAt", "concat", "endsWith", "fixed", "fontcolor", "fontsize", "includes", "indexOf", "italics", "lastIndexOf", "length", "link", "localeCompare", "match", "matchAll", "normalize", "padEnd", "padStart", "repeat", "replace", "replaceAll", "search", "slice", "small", "split", "startsWith", "strike", "sub", "substr", "substring", "sup", "toLocaleLowerCase", "toLocaleUpperCase", "toLowerCase", "toString", "toUpperCase", "trim", "trimEnd", "trimLeft", "trimRight", "trimStart", "valueOf"}
