package agent_test

import (
	"bytes"
	"crypto/sha256"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/nankedr/pig/internal/catalog"
	"github.com/nankedr/pig/internal/surface"
)

var updateIssue30Catalog = flag.Bool("update-issue30-catalog", false, "regenerate the issue #30 Parity Catalog rows")
var updateIssue30Surface = flag.Bool("update-issue30-surface", false, "regenerate the issue #30 Go API snapshot")

const (
	issue30BaselineCommit = "936aff00918de1187f085f123c2812d8f2d67745"
	issue30SurfaceHash    = "sha256:353816fada23e5469f2357d8a5e1b034481b0ada916e8ea129773f934a42c689"
	issue30MemberTestRef  = "agent/issue30_surface_test.go#TestIssue30ScaffoldedMemberTargetsResolve"
	issue30MemberTestRun  = "go test ./agent -run '^(TestIssue30MemberMappingsMatchLockedHarnessSurface|TestIssue30ScaffoldedMemberTargetsResolve)$' -count=1"
)

func TestIssue30MappingsMatchLockedHarnessSurface(t *testing.T) {
	root := issue30RepoRoot(t)
	symbols, err := surface.LoadSymbols(filepath.Join(root, "parity", "surface", "symbols.jsonl"))
	if err != nil {
		t.Fatalf("LoadSymbols: %v", err)
	}
	want := make(map[string]surface.Symbol)
	for _, symbol := range symbols {
		if issue30HarnessReference(symbol.Upstream.Reference) {
			want[symbol.ID] = symbol
		}
	}
	if len(want) != 265 {
		t.Fatalf("locked Harness surface count = %d, want 265", len(want))
	}

	entries := issue30CatalogSymbolEntries(t, root)
	if len(entries) != len(want) {
		t.Fatalf("catalog issue #30 symbol row count = %d, want %d", len(entries), len(want))
	}
	for _, entry := range entries {
		symbol, ok := want[entry.ID]
		if !ok {
			t.Errorf("catalog mapping is outside locked Harness surface: %s", entry.ID)
			continue
		}
		if entry.Upstream.Reference != symbol.Upstream.Reference {
			t.Errorf("%s upstream reference = %q, want %q", entry.ID, entry.Upstream.Reference, symbol.Upstream.Reference)
		}
		if entry.Mapping.Module != "agent" || entry.Mapping.Kind != "symbol" || entry.Mapping.Target == "" {
			t.Errorf("%s has incomplete mapping: %+v", entry.ID, entry.Mapping)
		}
		switch entry.Status {
		case catalog.StatusScaffolded, catalog.StatusPartial, catalog.StatusImplemented, catalog.StatusVerified:
		default:
			t.Errorf("%s has inactive Capability Status %q", entry.ID, entry.Status)
		}
		delete(want, entry.ID)
	}
	for id := range want {
		t.Errorf("locked Harness symbol has no Catalog mapping: %s", id)
	}
}

func TestIssue30ExportSubpathsUseApprovedGoPackages(t *testing.T) {
	root := issue30RepoRoot(t)
	symbols, err := surface.LoadSymbols(filepath.Join(root, "parity", "surface", "symbols.jsonl"))
	if err != nil {
		t.Fatalf("LoadSymbols: %v", err)
	}
	counts := map[string]int{}
	for _, symbol := range symbols {
		if !issue30HarnessReference(symbol.Upstream.Reference) {
			continue
		}
		counts[strings.Join(symbol.ExportSubpaths, ",")]++
		target := issue30SymbolTarget(symbol)
		switch strings.Join(symbol.ExportSubpaths, ",") {
		case ".,./node":
			if !strings.HasPrefix(target, "github.com/nankedr/pig/agent.") {
				t.Errorf("%s shared root/node contract does not use canonical Go agent package: %s", symbol.ID, target)
			}
		case "./node":
			if !strings.HasPrefix(target, "github.com/nankedr/pig/agent/node.") {
				t.Errorf("%s node-only contract does not use Go node package: %s", symbol.ID, target)
			}
		case "./session/testing":
			if !strings.HasPrefix(target, "github.com/nankedr/pig/agent/session/testing.") {
				t.Errorf("%s testing-only contract does not use Go testing package: %s", symbol.ID, target)
			}
		default:
			t.Errorf("%s has an unapproved Harness export-subpath mapping: %v", symbol.ID, symbol.ExportSubpaths)
		}
		if strings.Contains(target, "codingagent") || strings.Contains(strings.ToLower(target), "v3") || strings.Contains(strings.ToLower(target), "bridge") {
			t.Errorf("%s crosses the production v3 boundary: %s", symbol.ID, target)
		}
	}
	want := map[string]int{".,./node": 260, "./node": 1, "./session/testing": 4}
	if !reflect.DeepEqual(counts, want) {
		t.Fatalf("Harness export-subpath counts = %v, want %v", counts, want)
	}
}

func TestIssue30BroaderContractIsScaffolded(t *testing.T) {
	root := issue30RepoRoot(t)
	entries, err := catalog.LoadCatalog(filepath.Join(root, "parity", "catalog.jsonl"))
	if err != nil {
		t.Fatalf("LoadCatalog: %v", err)
	}
	for _, entry := range entries {
		if entry.ID != "contract:session/v4-harness" {
			continue
		}
		if entry.Status != catalog.StatusScaffolded {
			t.Fatalf("v4 Harness contract status = %q, want scaffolded", entry.Status)
		}
		if len(entry.Evidence) == 0 {
			t.Fatal("v4 Harness contract has no scaffold evidence")
		}
		if !strings.Contains(entry.Notes, "production v3") || !strings.Contains(entry.Notes, "unimplemented") {
			t.Fatalf("v4 Harness contract notes do not preserve the dual-path and fixed-snapshot gaps: %q", entry.Notes)
		}
		return
	}
	t.Fatal("missing contract:session/v4-harness")
}

func TestIssue30MemberMappingsMatchLockedHarnessSurface(t *testing.T) {
	root := issue30RepoRoot(t)
	surfacePath := filepath.Join(root, "parity", "surface", "symbols.jsonl")
	surfaceJSONL, err := os.ReadFile(surfacePath)
	if err != nil {
		t.Fatalf("read locked surface: %v", err)
	}
	if got := fmt.Sprintf("sha256:%x", sha256.Sum256(surfaceJSONL)); got != issue30SurfaceHash {
		t.Fatalf("locked surface hash = %s, want %s", got, issue30SurfaceHash)
	}
	symbols, err := surface.LoadSymbols(surfacePath)
	if err != nil {
		t.Fatalf("LoadSymbols: %v", err)
	}
	expected := issue30ExpectedCatalogEntries(symbols)
	if len(expected) != 1667 {
		t.Fatalf("issue #30 expected catalog rows = %d, want 1667", len(expected))
	}
	if *updateIssue30Catalog {
		issue30WriteCatalog(t, root, expected)
		return
	}

	got := issue30CatalogEntries(t, root)
	if len(got) != len(expected) {
		t.Fatalf("catalog issue #30 rows = %d, want %d", len(got), len(expected))
	}
	for i := range expected {
		if !reflect.DeepEqual(got[i], expected[i]) {
			t.Errorf("catalog row %s differs\n got: %+v\nwant: %+v", expected[i].ID, got[i], expected[i])
		}
	}
}

func TestIssue30LockedGoAPISnapshot(t *testing.T) {
	root := issue30RepoRoot(t)
	symbols, err := surface.LoadSymbols(filepath.Join(root, "parity", "surface", "symbols.jsonl"))
	if err != nil {
		t.Fatalf("LoadSymbols: %v", err)
	}
	got, err := issue30APISnapshot(root, symbols)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "agent", "testdata", "issue30_surface_golden.txt")
	if *updateIssue30Surface {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if got != string(want) {
		t.Fatalf("issue #30 Go API snapshot drifted; regenerate with -update-issue30-surface\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestIssue30ScaffoldedMemberTargetsResolve(t *testing.T) {
	root := issue30RepoRoot(t)
	symbols, err := surface.LoadSymbols(filepath.Join(root, "parity", "surface", "symbols.jsonl"))
	if err != nil {
		t.Fatalf("LoadSymbols: %v", err)
	}
	indexes := make(map[string]map[string]*issue30GoDecl)
	for _, rel := range []string{"agent", "agent/node", "agent/session/testing"} {
		index, err := issue30LoadDecls(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		indexes[rel] = index
	}
	for _, entry := range issue30ExpectedCatalogEntries(symbols) {
		if entry.Mapping.Kind != "contract" || entry.Status != catalog.StatusScaffolded {
			continue
		}
		dir, typeName, member, err := issue30MemberTargetParts(entry.Mapping.Target)
		if err != nil {
			t.Errorf("%s: %v", entry.ID, err)
			continue
		}
		decl := indexes[dir][typeName]
		if decl == nil {
			t.Errorf("%s maps to missing Go type %s", entry.ID, typeName)
			continue
		}
		if _, ok := decl.members[member]; !ok {
			t.Errorf("%s maps to missing Go member %s.%s", entry.ID, typeName, member)
		}
	}
}

type issue30GoDecl struct {
	declaration string
	methods     []string
	members     map[string]struct{}
	embeds      []string
}

func issue30APISnapshot(root string, symbols []surface.Symbol) (string, error) {
	indexes := make(map[string]map[string]*issue30GoDecl)
	for _, rel := range []string{"agent", "agent/node", "agent/session/testing"} {
		index, err := issue30LoadDecls(filepath.Join(root, filepath.FromSlash(rel)))
		if err != nil {
			return "", err
		}
		indexes[rel] = index
	}
	var selected []surface.Symbol
	for _, symbol := range symbols {
		if issue30HarnessReference(symbol.Upstream.Reference) {
			selected = append(selected, symbol)
		}
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].ID < selected[j].ID })
	var b strings.Builder
	var missing []string
	for i, symbol := range selected {
		target := issue30SymbolTarget(symbol)
		dir, name, err := issue30TargetParts(target)
		if err != nil {
			return "", fmt.Errorf("%s: %w", symbol.ID, err)
		}
		decl := indexes[dir][name]
		if decl == nil {
			missing = append(missing, fmt.Sprintf("%s -> %s", symbol.ID, target))
			continue
		}
		if i > 0 {
			b.WriteByte('\n')
		}
		fmt.Fprintf(&b, "## %s\nreference: %s\ntarget: %s\n%s\n", symbol.ID, symbol.Upstream.Reference, target, decl.declaration)
		for _, method := range decl.methods {
			b.WriteString(method)
			b.WriteByte('\n')
		}
	}
	if len(missing) != 0 {
		return "", fmt.Errorf("missing %d Go declarations:\n%s", len(missing), strings.Join(missing, "\n"))
	}
	return b.String(), nil
}

func issue30LoadDecls(dir string) (map[string]*issue30GoDecl, error) {
	fset := token.NewFileSet()
	packages, err := parser.ParseDir(fset, dir, func(info os.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", dir, err)
	}
	index := make(map[string]*issue30GoDecl)
	for _, pkg := range packages {
		for _, file := range pkg.Files {
			for _, rawDecl := range file.Decls {
				switch decl := rawDecl.(type) {
				case *ast.GenDecl:
					for _, spec := range decl.Specs {
						text, err := issue30FormatNode(fset, &ast.GenDecl{Tok: decl.Tok, Specs: []ast.Spec{spec}})
						if err != nil {
							return nil, err
						}
						for _, name := range issue30SpecNames(spec) {
							_, isType := spec.(*ast.TypeSpec)
							if !ast.IsExported(name) && !isType {
								continue
							}
							entry := index[name]
							if entry == nil {
								entry = &issue30GoDecl{members: make(map[string]struct{})}
								index[name] = entry
							}
							entry.declaration = text
							if typeSpec, ok := spec.(*ast.TypeSpec); ok {
								members, embeds := issue30TypeMembers(typeSpec.Type)
								for _, member := range members {
									entry.members[member] = struct{}{}
								}
								entry.embeds = append(entry.embeds, embeds...)
							}
						}
					}
				case *ast.FuncDecl:
					clone := *decl
					clone.Doc = nil
					clone.Body = nil
					text, err := issue30FormatNode(fset, &clone)
					if err != nil {
						return nil, err
					}
					if decl.Recv == nil {
						if ast.IsExported(decl.Name.Name) {
							entry := index[decl.Name.Name]
							if entry == nil {
								entry = &issue30GoDecl{members: make(map[string]struct{})}
								index[decl.Name.Name] = entry
							}
							entry.declaration = text
						}
						continue
					}
					receiver := issue30ReceiverName(decl.Recv.List[0].Type)
					if receiver == "" || !ast.IsExported(decl.Name.Name) {
						continue
					}
					entry := index[receiver]
					if entry == nil {
						entry = &issue30GoDecl{members: make(map[string]struct{})}
						index[receiver] = entry
					}
					entry.methods = append(entry.methods, text)
					entry.members[decl.Name.Name] = struct{}{}
				}
			}
		}
	}
	for _, decl := range index {
		sort.Strings(decl.methods)
	}
	for changed := true; changed; {
		changed = false
		for _, decl := range index {
			for _, embed := range decl.embeds {
				embedded := index[embed]
				if embedded == nil {
					continue
				}
				for member := range embedded.members {
					if _, ok := decl.members[member]; !ok {
						decl.members[member] = struct{}{}
						changed = true
					}
				}
			}
		}
	}
	return index, nil
}

func issue30SpecNames(spec ast.Spec) []string {
	switch spec := spec.(type) {
	case *ast.TypeSpec:
		return []string{spec.Name.Name}
	case *ast.ValueSpec:
		names := make([]string, len(spec.Names))
		for i := range spec.Names {
			names[i] = spec.Names[i].Name
		}
		return names
	}
	return nil
}

func issue30TypeMembers(expr ast.Expr) (members, embeds []string) {
	switch expr := expr.(type) {
	case *ast.StructType:
		for _, field := range expr.Fields.List {
			if len(field.Names) == 0 {
				if name := issue30ReceiverName(field.Type); name != "" {
					embeds = append(embeds, name)
				}
				continue
			}
			for _, name := range field.Names {
				if ast.IsExported(name.Name) {
					members = append(members, name.Name)
				}
			}
		}
	case *ast.InterfaceType:
		for _, field := range expr.Methods.List {
			if len(field.Names) == 0 {
				if name := issue30ReceiverName(field.Type); name != "" {
					if name == "error" {
						members = append(members, "Error")
						continue
					}
					embeds = append(embeds, name)
				}
				continue
			}
			for _, name := range field.Names {
				if ast.IsExported(name.Name) {
					members = append(members, name.Name)
				}
			}
		}
	default:
		if name := issue30ReceiverName(expr); name != "" {
			embeds = append(embeds, name)
		}
	}
	return members, embeds
}

func issue30ReceiverName(expr ast.Expr) string {
	switch expr := expr.(type) {
	case *ast.Ident:
		return expr.Name
	case *ast.StarExpr:
		return issue30ReceiverName(expr.X)
	case *ast.IndexExpr:
		return issue30ReceiverName(expr.X)
	case *ast.IndexListExpr:
		return issue30ReceiverName(expr.X)
	default:
		return ""
	}
}

func issue30FormatNode(fset *token.FileSet, node any) (string, error) {
	var b bytes.Buffer
	if err := format.Node(&b, fset, node); err != nil {
		return "", err
	}
	return b.String(), nil
}

func issue30TargetParts(target string) (string, string, error) {
	for _, prefix := range []struct {
		path string
		dir  string
	}{
		{"github.com/nankedr/pig/agent/session/testing.", "agent/session/testing"},
		{"github.com/nankedr/pig/agent/node.", "agent/node"},
		{"github.com/nankedr/pig/agent.", "agent"},
	} {
		if strings.HasPrefix(target, prefix.path) {
			return prefix.dir, strings.TrimPrefix(target, prefix.path), nil
		}
	}
	return "", "", fmt.Errorf("unsupported Go target %q", target)
}

func issue30MemberTargetParts(target string) (dir, typeName, member string, err error) {
	dir, rest, err := issue30TargetParts(target)
	if err != nil {
		return "", "", "", err
	}
	typeName, member, ok := strings.Cut(rest, ".")
	if !ok || typeName == "" || member == "" {
		return "", "", "", fmt.Errorf("member target %q has no member", target)
	}
	return dir, typeName, member, nil
}

func issue30ExpectedCatalogEntries(symbols []surface.Symbol) []catalog.Entry {
	var entries []catalog.Entry
	for _, symbol := range symbols {
		if !issue30HarnessReference(symbol.Upstream.Reference) {
			continue
		}
		target := issue30SymbolTarget(symbol)
		entries = append(entries, catalog.Entry{
			SchemaVersion:  catalog.SchemaVersion,
			ID:             symbol.ID,
			Upstream:       catalog.Upstream(symbol.Upstream),
			Mapping:        catalog.Mapping{Module: "agent", Target: target, Kind: "symbol"},
			Status:         catalog.StatusScaffolded,
			Milestone:      "M8",
			Classification: "public-api",
			Notes:          "Issue #30 concrete Harness symbol mapping authority. Behavioral status remains on contract:session/v4-harness.",
		})
		for _, member := range symbol.Members {
			id := "member:" + strings.TrimPrefix(symbol.ID, "symbol:") + "." + member
			memberTarget, inventoryReason := issue30MemberTarget(symbol, target, member)
			entry := catalog.Entry{
				SchemaVersion:  catalog.SchemaVersion,
				ID:             id,
				Upstream:       catalog.Upstream{Module: "agent", Repository: symbol.Upstream.Repository, Commit: symbol.Upstream.Commit, Reference: "packages/" + strings.TrimPrefix(id, "member:")},
				Mapping:        catalog.Mapping{Module: "agent", Target: memberTarget, Kind: "contract"},
				Status:         catalog.StatusScaffolded,
				Milestone:      "M8",
				Classification: "public-api",
				Notes:          fmt.Sprintf("Issue #30 concrete %s member mapping authority. Behavioral status remains on contract:session/v4-harness.", symbol.Name),
			}
			if inventoryReason != "" {
				entry.Status = catalog.StatusInventoried
				entry.Notes = fmt.Sprintf("Issue #30 records %s.%s as %s. It maps to the Go carrier shown by mapping.target; no separate option or value member is required. Behavioral status remains on contract:session/v4-harness.", symbol.Name, member, inventoryReason)
			} else {
				entry.Evidence = []catalog.Evidence{{
					Kind:            "go-test",
					Ref:             issue30MemberTestRef,
					Baseline:        issue30BaselineCommit,
					CaseID:          id,
					InputHash:       issue30SurfaceHash,
					ExecutionMethod: issue30MemberTestRun,
					Expected:        fmt.Sprintf("the pinned Pi %s.%s member maps to the declared scaffolded Go contract target", symbol.Name, member),
					Actual:          fmt.Sprintf("PASS; %s resolved to %s", id, memberTarget),
					Platform:        "any",
					CatalogID:       id,
				}}
			}
			entries = append(entries, entry)
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	return entries
}

func issue30SymbolTarget(symbol surface.Symbol) string {
	pkg := "github.com/nankedr/pig/agent"
	if symbol.Name == "NodeExecutionEnv" {
		pkg += "/node"
	} else if reflect.DeepEqual(symbol.ExportSubpaths, []string{"./session/testing"}) {
		pkg += "/session/testing"
	}
	return pkg + "." + issue30GoName(symbol.Name)
}

func issue30MemberTarget(symbol surface.Symbol, parentTarget, member string) (string, string) {
	if reflect.DeepEqual(symbol.Members, issue30StringInheritedMembers) || issue30ErrorInheritedMember(symbol, member) {
		return parentTarget, "a TypeScript runtime-inherited member represented by Go's built-in value or error contract"
	}
	if symbol.Name == "GenerateBranchSummaryOptions" && member == "signal" {
		return "github.com/nankedr/pig/agent.GenerateBranchSummary", "a cancellation input carried by the mapped operation's context.Context parameter in Go"
	}
	if member == "abortSignal" {
		switch symbol.Name {
		case "ShellExecOptions":
			return "github.com/nankedr/pig/agent.Shell.Exec", "a cancellation input carried by the mapped operation's context.Context parameter in Go"
		case "ShellCaptureOptions":
			return "github.com/nankedr/pig/agent.ExecuteShellWithCapture", "a cancellation input carried by the mapped operation's context.Context parameter in Go"
		}
	}
	if member == "message" && (symbol.Name == "HarnessClosed" || strings.HasSuffix(symbol.Name, "Rejected")) {
		return parentTarget + ".Error", ""
	}
	if member == "type" {
		switch symbol.Name {
		case "Entry", "EntryBase", "ProvisionedEntry":
			return "github.com/nankedr/pig/agent.Entry.EntryType", ""
		case "MessageEntry", "ModelChangeEntry", "ThinkingLevelEntry", "ActiveToolsEntry", "CompactionEntry", "BranchSummaryEntry", "CustomEntry":
			return parentTarget + ".EntryType", ""
		case "LaneRecord", "NewRecord":
			return "github.com/nankedr/pig/agent.LaneRecord.RecordType", ""
		case "OperationStartedRecord", "AbortRequestedRecord", "OperationFinishedRecord", "StepAttemptRecord", "ToolStartedRecord", "QueueEnqueuedRecord", "QueueCancelledRecord", "WriteDeferredRecord", "UsageRecord":
			return parentTarget + ".RecordType", ""
		}
	}
	if symbol.Name == "Entry" || symbol.Name == "ProvisionedEntry" {
		if target, ok := issue30BaseMemberTarget("EntryBase", member); ok {
			return target, ""
		}
	}
	if symbol.Name == "LaneRecord" || symbol.Name == "NewRecord" {
		if target, ok := issue30BaseMemberTarget("RecordBase", member); ok {
			return target, ""
		}
	}
	if symbol.Name == "LogItem" {
		switch member {
		case "kind":
			return parentTarget + ".LogItemKind", ""
		case "seq":
			return parentTarget + ".LogSequence", ""
		}
	}
	return parentTarget + "." + issue30GoName(member), ""
}

func issue30BaseMemberTarget(base, member string) (string, bool) {
	switch member {
	case "id", "seq", "parentId", "timestamp", "lane":
		return "github.com/nankedr/pig/agent." + base + "." + issue30GoName(member), true
	default:
		return "", false
	}
}

func issue30ErrorInheritedMember(symbol surface.Symbol, member string) bool {
	if !strings.HasSuffix(symbol.Name, "Rejected") && !issue30HasMembers(symbol, "cause", "message", "name", "stack") {
		return false
	}
	switch member {
	case "cause", "name", "stack":
		return true
	default:
		return false
	}
}

func issue30HasMembers(symbol surface.Symbol, names ...string) bool {
	members := make(map[string]struct{}, len(symbol.Members))
	for _, member := range symbol.Members {
		members[member] = struct{}{}
	}
	for _, name := range names {
		if _, ok := members[name]; !ok {
			return false
		}
	}
	return true
}

func issue30GoName(name string) string {
	if name == "MissingIdentities" {
		return name
	}
	if name == "fs" {
		return "FS"
	}
	if name == "ok" {
		return "OK"
	}
	if name == "err" {
		return "Err"
	}
	if name == "" {
		return ""
	}
	if strings.Contains(name, "_") {
		parts := strings.Split(strings.ToLower(name), "_")
		var b strings.Builder
		for _, part := range parts {
			if part != "" {
				b.WriteString(strings.ToUpper(part[:1]) + part[1:])
			}
		}
		name = b.String()
	}
	name = strings.ToUpper(name[:1]) + name[1:]
	name = strings.NewReplacer(
		"Jsonl", "JSONL", "Json", "JSON", "Llm", "LLM", "Api", "API",
		"Url", "URL", "Uri", "URI", "Uuid", "UUID", "Id", "ID",
		"Mime", "MIME", "Cwd", "CWD", "Ai", "AI", "Ms", "MS",
	).Replace(name)
	return strings.ReplaceAll(name, "IDle", "Idle")
}

func issue30CatalogEntries(t *testing.T, root string) []catalog.Entry {
	t.Helper()
	entries, err := catalog.LoadCatalog(filepath.Join(root, "parity", "catalog.jsonl"))
	if err != nil {
		t.Fatalf("LoadCatalog: %v", err)
	}
	var filtered []catalog.Entry
	for _, entry := range entries {
		if issue30CatalogID(entry.ID) {
			filtered = append(filtered, entry)
		}
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].ID < filtered[j].ID })
	return filtered
}

func issue30CatalogID(id string) bool {
	for _, prefix := range []string{"symbol:agent/src/harness/", "member:agent/src/harness/"} {
		if strings.HasPrefix(id, prefix) {
			return true
		}
	}
	return false
}

func issue30WriteCatalog(t *testing.T, root string, expected []catalog.Entry) {
	t.Helper()
	path := filepath.Join(root, "parity", "catalog.jsonl")
	entries, err := catalog.LoadCatalog(path)
	if err != nil {
		t.Fatalf("LoadCatalog: %v", err)
	}
	kept := entries[:0]
	for _, entry := range entries {
		if issue30CatalogID(entry.ID) {
			continue
		}
		if entry.ID == "contract:session/v4-harness" {
			entry.Status = catalog.StatusScaffolded
			entry.Evidence = []catalog.Evidence{
				{Kind: "go-test", Ref: "agent/issue30_surface_test.go", Baseline: issue30BaselineCommit},
				{Kind: "go-test", Ref: "agent/harness_test.go", Baseline: issue30BaselineCommit},
				{Kind: "go-test", Ref: "agent/harness_session_test.go", Baseline: issue30BaselineCommit},
				{Kind: "go-test", Ref: "agent/session/testing/conformance_test.go", Baseline: issue30BaselineCommit},
			}
			entry.Notes = "Issue #30 scaffolds all 265 fixed-snapshot Harness symbols on the root, node, and session/testing export surfaces, including the independent v4 Session, records, reducer/compaction substrate, and Harness Tool contracts. Per ADR-0003, Pi's shared root/node re-exports map once to Go agent, while NodeExecutionEnv maps to agent/node and conformance contracts map to agent/session/testing. Operations that the fixed snapshot leaves unimplemented fail explicitly without side effects and remain targeted for M8; the Harness path has no production v3 bridge and does not migrate production AgentSession."
		}
		kept = append(kept, entry)
	}
	data, err := catalog.EncodeEntries(append(kept, expected...))
	if err != nil {
		t.Fatalf("EncodeEntries: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write catalog: %v", err)
	}
}

var issue30StringInheritedMembers = []string{
	"anchor", "at", "big", "blink", "bold", "charAt", "charCodeAt", "codePointAt", "concat",
	"endsWith", "fixed", "fontcolor", "fontsize", "includes", "indexOf", "italics",
	"lastIndexOf", "length", "link", "localeCompare", "match", "matchAll", "normalize",
	"padEnd", "padStart", "repeat", "replace", "replaceAll", "search", "slice", "small",
	"split", "startsWith", "strike", "sub", "substr", "substring", "sup",
	"toLocaleLowerCase", "toLocaleUpperCase", "toLowerCase", "toString", "toUpperCase",
	"trim", "trimEnd", "trimLeft", "trimRight", "trimStart", "valueOf",
}

func issue30HarnessReference(reference string) bool {
	return strings.HasPrefix(reference, "packages/agent/src/harness/")
}

func issue30CatalogSymbolEntries(t *testing.T, root string) []catalog.Entry {
	t.Helper()
	entries, err := catalog.LoadCatalog(filepath.Join(root, "parity", "catalog.jsonl"))
	if err != nil {
		t.Fatalf("LoadCatalog: %v", err)
	}
	var filtered []catalog.Entry
	for _, entry := range entries {
		if entry.Mapping.Module == "agent" && entry.Mapping.Kind == "symbol" && issue30HarnessReference(entry.Upstream.Reference) {
			filtered = append(filtered, entry)
		}
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].ID < filtered[j].ID })
	return filtered
}

func issue30RepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
}
