package tui_test

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

var updateIssue31Catalog = flag.Bool("update-issue31-catalog", false, "regenerate the issue #31 Parity Catalog rows")
var updateIssue31Surface = flag.Bool("update-issue31-surface", false, "regenerate the issue #31 Go API snapshot")

const (
	issue31BaselineCommit = "936aff00918de1187f085f123c2812d8f2d67745"
	issue31SurfaceHash    = "sha256:353816fada23e5469f2357d8a5e1b034481b0ada916e8ea129773f934a42c689"
	issue31GoPackage      = "github.com/nankedr/pig/tui"
	issue31MemberTestRef  = "tui/issue31_surface_test.go#TestIssue31ScaffoldedMemberTargetsResolve"
	issue31MemberTestRun  = "go test ./tui -run '^(TestIssue31MemberMappingsMatchLockedTUISurface|TestIssue31ScaffoldedMemberTargetsResolve)$' -count=1"
)

func TestIssue31MappingsMatchLockedTUISurface(t *testing.T) {
	root := issue31RepoRoot(t)
	symbols, err := surface.LoadSymbols(filepath.Join(root, "parity", "surface", "symbols.jsonl"))
	if err != nil {
		t.Fatalf("LoadSymbols: %v", err)
	}
	want := make(map[string]surface.Symbol)
	for _, symbol := range symbols {
		if issue31TUISymbol(symbol) {
			want[symbol.ID] = symbol
		}
	}
	if len(want) != 191 {
		t.Fatalf("locked TUI surface count = %d, want 191", len(want))
	}

	entries := issue31CatalogSymbolEntries(t, root)
	if len(entries) != len(want) {
		t.Fatalf("catalog issue #31 symbol row count = %d, want %d", len(entries), len(want))
	}
	for _, entry := range entries {
		symbol, ok := want[entry.ID]
		if !ok {
			t.Errorf("catalog mapping is outside locked TUI surface: %s", entry.ID)
			continue
		}
		if entry.Upstream.Reference != symbol.Upstream.Reference {
			t.Errorf("%s upstream reference = %q, want %q", entry.ID, entry.Upstream.Reference, symbol.Upstream.Reference)
		}
		if entry.Mapping.Module != "tui" || entry.Mapping.Kind != "symbol" || entry.Mapping.Target != issue31SymbolTarget(symbol) {
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
		t.Errorf("locked TUI symbol has no Catalog mapping: %s", id)
	}
}

func TestIssue31ExportSubpathsUseCanonicalGoPackage(t *testing.T) {
	root := issue31RepoRoot(t)
	symbols, err := surface.LoadSymbols(filepath.Join(root, "parity", "surface", "symbols.jsonl"))
	if err != nil {
		t.Fatalf("LoadSymbols: %v", err)
	}
	counts := map[string]int{}
	for _, symbol := range symbols {
		if !issue31TUISymbol(symbol) {
			continue
		}
		target := issue31SymbolTarget(symbol)
		if !strings.HasPrefix(target, issue31GoPackage+".") {
			t.Errorf("%s does not use the canonical Go TUI package: %s", symbol.ID, target)
		}
		for _, subpath := range symbol.ExportSubpaths {
			counts[subpath]++
		}
	}
	if !reflect.DeepEqual(counts, issue31ExportSubpathCounts) {
		t.Fatalf("TUI export-subpath counts = %v, want %v", counts, issue31ExportSubpathCounts)
	}
	if len(counts) != 38 {
		t.Fatalf("TUI export-subpath count = %d, want 38", len(counts))
	}
}

func TestIssue31BroaderContractIsPartial(t *testing.T) {
	root := issue31RepoRoot(t)
	entries, err := catalog.LoadCatalog(filepath.Join(root, "parity", "catalog.jsonl"))
	if err != nil {
		t.Fatalf("LoadCatalog: %v", err)
	}
	for _, entry := range entries {
		if entry.ID != "module-tui" {
			continue
		}
		if entry.Status != catalog.StatusPartial {
			t.Fatalf("TUI module status = %q, want partial", entry.Status)
		}
		if entry.Mapping.Module != "tui" || entry.Mapping.Kind != "package" || entry.Mapping.Target != issue31GoPackage {
			t.Fatalf("TUI module mapping = %+v, want canonical Go package", entry.Mapping)
		}
		if !issue31HasEvidenceRef(entry.Evidence, "tui/issue31_surface_test.go") ||
			!issue31HasEvidenceRef(entry.Evidence, "tui/terminal_test.go") ||
			!issue31HasEvidenceRef(entry.Evidence, "tui/components_test.go") ||
			!issue31HasEvidenceRef(entry.Evidence, "tui/input_test.go") {
			t.Fatalf("TUI module does not carry surface and side-effect scaffold evidence: %+v", entry.Evidence)
		}
		if entry.Partial == nil || len(entry.Partial.Supported) == 0 || len(entry.Partial.Unsupported) == 0 {
			t.Fatalf("TUI module does not describe supported and unsupported branches: %+v", entry.Partial)
		}
		for _, phrase := range []string{"191", "1,035", "38", "canonical Go package", "interactive", "side effects", "CGO-free"} {
			if !strings.Contains(entry.Notes, phrase) {
				t.Errorf("TUI module notes do not mention %q: %q", phrase, entry.Notes)
			}
		}
		return
	}
	t.Fatal("missing module-tui")
}

func TestIssue31MemberMappingsMatchLockedTUISurface(t *testing.T) {
	root := issue31RepoRoot(t)
	surfacePath := filepath.Join(root, "parity", "surface", "symbols.jsonl")
	surfaceJSONL, err := os.ReadFile(surfacePath)
	if err != nil {
		t.Fatalf("read locked surface: %v", err)
	}
	if got := fmt.Sprintf("sha256:%x", sha256.Sum256(surfaceJSONL)); got != issue31SurfaceHash {
		t.Fatalf("locked surface hash = %s, want %s", got, issue31SurfaceHash)
	}
	symbols, err := surface.LoadSymbols(surfacePath)
	if err != nil {
		t.Fatalf("LoadSymbols: %v", err)
	}
	expected := issue31ExpectedCatalogEntries(symbols)
	if len(expected) != 1226 {
		t.Fatalf("issue #31 expected catalog rows = %d, want 1226 (191 symbols plus 1,035 members)", len(expected))
	}
	if got := issue31CountCatalogKind(expected, "symbol"); got != 191 {
		t.Fatalf("issue #31 expected symbol rows = %d, want 191", got)
	}
	if got := issue31CountCatalogKind(expected, "contract"); got != 1035 {
		t.Fatalf("issue #31 expected member rows = %d, want 1035", got)
	}
	if got := issue31CountCatalogStatus(expected, catalog.StatusInventoried); got != 395 {
		t.Fatalf("issue #31 inherited primitive member rows = %d, want 395", got)
	}
	if got := issue31CountCatalogStatus(expected, catalog.StatusScaffolded); got != 831 {
		t.Fatalf("issue #31 scaffolded symbol/member rows = %d, want 831", got)
	}
	if *updateIssue31Catalog {
		issue31WriteCatalog(t, root, expected)
		return
	}

	got := issue31CatalogEntries(t, root)
	if len(got) != len(expected) {
		t.Fatalf("catalog issue #31 rows = %d, want %d", len(got), len(expected))
	}
	for i := range expected {
		if !reflect.DeepEqual(got[i], expected[i]) {
			t.Errorf("catalog row %s differs\n got: %+v\nwant: %+v", expected[i].ID, got[i], expected[i])
		}
	}
}

func TestIssue31LockedGoAPISnapshot(t *testing.T) {
	root := issue31RepoRoot(t)
	symbols, err := surface.LoadSymbols(filepath.Join(root, "parity", "surface", "symbols.jsonl"))
	if err != nil {
		t.Fatalf("LoadSymbols: %v", err)
	}
	got, err := issue31APISnapshot(root, symbols)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "tui", "testdata", "issue31_surface_golden.txt")
	if *updateIssue31Surface {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create golden directory: %v", err)
		}
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
		t.Fatalf("issue #31 Go API snapshot drifted; regenerate with -update-issue31-surface\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestIssue31ScaffoldedMemberTargetsResolve(t *testing.T) {
	root := issue31RepoRoot(t)
	symbols, err := surface.LoadSymbols(filepath.Join(root, "parity", "surface", "symbols.jsonl"))
	if err != nil {
		t.Fatalf("LoadSymbols: %v", err)
	}
	index, err := issue31LoadDecls(filepath.Join(root, "tui"))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range issue31ExpectedCatalogEntries(symbols) {
		if entry.Mapping.Kind != "contract" || entry.Status != catalog.StatusScaffolded {
			continue
		}
		typeName, member, err := issue31MemberTargetParts(entry.Mapping.Target)
		if err != nil {
			t.Errorf("%s: %v", entry.ID, err)
			continue
		}
		decl := index[typeName]
		if decl == nil {
			t.Errorf("%s maps to missing Go type %s", entry.ID, typeName)
			continue
		}
		if _, ok := decl.members[member]; !ok {
			t.Errorf("%s maps to missing Go member %s.%s", entry.ID, typeName, member)
		}
	}
}

type issue31GoDecl struct {
	declaration string
	methods     []string
	members     map[string]struct{}
	embeds      []string
}

func issue31APISnapshot(root string, symbols []surface.Symbol) (string, error) {
	index, err := issue31LoadDecls(filepath.Join(root, "tui"))
	if err != nil {
		return "", err
	}
	var selected []surface.Symbol
	for _, symbol := range symbols {
		if issue31TUISymbol(symbol) {
			selected = append(selected, symbol)
		}
	}
	sort.Slice(selected, func(i, j int) bool { return selected[i].ID < selected[j].ID })
	var b strings.Builder
	var missing []string
	for _, symbol := range selected {
		target := issue31SymbolTarget(symbol)
		name, err := issue31TargetName(target)
		if err != nil {
			return "", fmt.Errorf("%s: %w", symbol.ID, err)
		}
		decl := index[name]
		if decl == nil {
			missing = append(missing, fmt.Sprintf("%s -> %s", symbol.ID, target))
			continue
		}
		if b.Len() > 0 {
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

func issue31LoadDecls(dir string) (map[string]*issue31GoDecl, error) {
	fset := token.NewFileSet()
	packages, err := parser.ParseDir(fset, dir, func(info os.FileInfo) bool {
		return !strings.HasSuffix(info.Name(), "_test.go")
	}, parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", dir, err)
	}
	index := make(map[string]*issue31GoDecl)
	for _, pkg := range packages {
		for _, file := range pkg.Files {
			for _, rawDecl := range file.Decls {
				switch decl := rawDecl.(type) {
				case *ast.GenDecl:
					for _, spec := range decl.Specs {
						text, err := issue31FormatNode(fset, &ast.GenDecl{Tok: decl.Tok, Specs: []ast.Spec{spec}})
						if err != nil {
							return nil, err
						}
						for _, name := range issue31SpecNames(spec) {
							_, isType := spec.(*ast.TypeSpec)
							if !ast.IsExported(name) && !isType {
								continue
							}
							entry := issue31Decl(index, name)
							entry.declaration = text
							if typeSpec, ok := spec.(*ast.TypeSpec); ok {
								members, embeds := issue31TypeMembers(typeSpec.Type)
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
					text, err := issue31FormatNode(fset, &clone)
					if err != nil {
						return nil, err
					}
					if decl.Recv == nil {
						if ast.IsExported(decl.Name.Name) {
							issue31Decl(index, decl.Name.Name).declaration = text
						}
						continue
					}
					receiver := issue31ReceiverName(decl.Recv.List[0].Type)
					if receiver == "" || !ast.IsExported(decl.Name.Name) {
						continue
					}
					entry := issue31Decl(index, receiver)
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

func issue31Decl(index map[string]*issue31GoDecl, name string) *issue31GoDecl {
	entry := index[name]
	if entry == nil {
		entry = &issue31GoDecl{members: make(map[string]struct{})}
		index[name] = entry
	}
	return entry
}

func issue31SpecNames(spec ast.Spec) []string {
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

func issue31TypeMembers(expr ast.Expr) (members, embeds []string) {
	switch expr := expr.(type) {
	case *ast.StructType:
		for _, field := range expr.Fields.List {
			if len(field.Names) == 0 {
				if name := issue31ReceiverName(field.Type); name != "" {
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
				if name := issue31ReceiverName(field.Type); name != "" {
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
		if name := issue31ReceiverName(expr); name != "" {
			embeds = append(embeds, name)
		}
	}
	return members, embeds
}

func issue31ReceiverName(expr ast.Expr) string {
	switch expr := expr.(type) {
	case *ast.Ident:
		return expr.Name
	case *ast.StarExpr:
		return issue31ReceiverName(expr.X)
	case *ast.IndexExpr:
		return issue31ReceiverName(expr.X)
	case *ast.IndexListExpr:
		return issue31ReceiverName(expr.X)
	default:
		return ""
	}
}

func issue31FormatNode(fset *token.FileSet, node any) (string, error) {
	var b bytes.Buffer
	if err := format.Node(&b, fset, node); err != nil {
		return "", err
	}
	return b.String(), nil
}

func issue31TargetName(target string) (string, error) {
	prefix := issue31GoPackage + "."
	if !strings.HasPrefix(target, prefix) {
		return "", fmt.Errorf("unsupported Go target %q", target)
	}
	name := strings.TrimPrefix(target, prefix)
	if name == "" || strings.Contains(name, ".") {
		return "", fmt.Errorf("symbol target %q is not a declaration in the canonical TUI package", target)
	}
	return name, nil
}

func issue31MemberTargetParts(target string) (typeName, member string, err error) {
	prefix := issue31GoPackage + "."
	if !strings.HasPrefix(target, prefix) {
		return "", "", fmt.Errorf("unsupported Go target %q", target)
	}
	typeName, member, ok := strings.Cut(strings.TrimPrefix(target, prefix), ".")
	if !ok || typeName == "" || member == "" || strings.Contains(member, ".") {
		return "", "", fmt.Errorf("member target %q does not name one member in the canonical TUI package", target)
	}
	return typeName, member, nil
}

func issue31ExpectedCatalogEntries(symbols []surface.Symbol) []catalog.Entry {
	var entries []catalog.Entry
	for _, symbol := range symbols {
		if !issue31TUISymbol(symbol) {
			continue
		}
		target := issue31SymbolTarget(symbol)
		entries = append(entries, catalog.Entry{
			SchemaVersion:  catalog.SchemaVersion,
			ID:             symbol.ID,
			Upstream:       catalog.Upstream(symbol.Upstream),
			Mapping:        catalog.Mapping{Module: "tui", Target: target, Kind: "symbol"},
			Status:         catalog.StatusScaffolded,
			Milestone:      "M6",
			Classification: "public-api",
			Notes:          "Issue #31 concrete TUI symbol mapping authority. TypeScript root exports and technically reachable deep imports map to the single canonical Go tui package; behavioral status remains on module-tui.",
		})
		for _, member := range symbol.Members {
			id := "member:" + strings.TrimPrefix(symbol.ID, "symbol:") + "." + member
			memberTarget, inventoryReason := issue31MemberTarget(symbol, target, member)
			entry := catalog.Entry{
				SchemaVersion:  catalog.SchemaVersion,
				ID:             id,
				Upstream:       catalog.Upstream{Module: "tui", Repository: symbol.Upstream.Repository, Commit: symbol.Upstream.Commit, Reference: "packages/" + strings.TrimPrefix(id, "member:")},
				Mapping:        catalog.Mapping{Module: "tui", Target: memberTarget, Kind: "contract"},
				Status:         catalog.StatusScaffolded,
				Milestone:      "M6",
				Classification: "public-api",
				Notes:          fmt.Sprintf("Issue #31 concrete %s member mapping authority. Behavioral status remains on module-tui.", symbol.Name),
			}
			if inventoryReason != "" {
				entry.Status = catalog.StatusInventoried
				entry.Notes = fmt.Sprintf("Issue #31 records %s.%s as %s. It maps to the Go carrier shown by mapping.target; no separate Go member is required. Behavioral status remains on module-tui.", symbol.Name, member, inventoryReason)
			} else {
				entry.Evidence = []catalog.Evidence{{
					Kind:            "go-test",
					Ref:             issue31MemberTestRef,
					Baseline:        issue31BaselineCommit,
					CaseID:          id,
					InputHash:       issue31SurfaceHash,
					ExecutionMethod: issue31MemberTestRun,
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

func issue31SymbolTarget(symbol surface.Symbol) string {
	if mapped, ok := issue31SymbolTargetExceptions[symbol.ID]; ok {
		return issue31GoPackage + "." + mapped
	}
	return issue31GoPackage + "." + issue31GoName(symbol.Name)
}

func issue31MemberTarget(symbol surface.Symbol, parentTarget, member string) (string, string) {
	if reflect.DeepEqual(symbol.Members, issue31StringInheritedMembers) {
		return parentTarget, "a TypeScript default-library inherited String member"
	}
	if reflect.DeepEqual(symbol.Members, issue31NumberInheritedMembers) {
		return parentTarget, "a TypeScript default-library inherited Number member"
	}
	if target, ok := issue31MemberTargetExceptions[symbol.Name+"."+member]; ok {
		return issue31GoPackage + "." + target, ""
	}
	return parentTarget + "." + issue31GoName(member), ""
}

func issue31GoName(name string) string {
	if mapped, ok := issue31NameExceptions[name]; ok {
		return mapped
	}
	if name == "" {
		return ""
	}
	if strings.Contains(name, ".") {
		parts := strings.Split(name, ".")
		var b strings.Builder
		for _, part := range parts {
			b.WriteString(issue31GoName(part))
		}
		return b.String()
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
	return strings.NewReplacer(
		"Tui", "TUI", "Rgb", "RGB", "Cjk", "CJK", "Ansi", "ANSI",
		"Osc", "OSC", "Jsonl", "JSONL", "Json", "JSON", "Api", "API",
		"Url", "URL", "Uri", "URI", "Uuid", "UUID", "Id", "ID",
		"Gif", "GIF", "Jpeg", "JPEG", "Png", "PNG", "Webp", "WebP",
		"Latex", "LaTeX", "Llm", "LLM", "Mime", "MIME", "Cwd", "CWD",
		"Ai", "AI", "Px", "PX", "Ms", "MS", "Bg", "BG",
	).Replace(name)
}

func issue31CatalogEntries(t *testing.T, root string) []catalog.Entry {
	t.Helper()
	entries, err := catalog.LoadCatalog(filepath.Join(root, "parity", "catalog.jsonl"))
	if err != nil {
		t.Fatalf("LoadCatalog: %v", err)
	}
	var filtered []catalog.Entry
	for _, entry := range entries {
		if issue31CatalogID(entry.ID) {
			filtered = append(filtered, entry)
		}
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].ID < filtered[j].ID })
	return filtered
}

func issue31CatalogID(id string) bool {
	return strings.HasPrefix(id, "symbol:tui/") || strings.HasPrefix(id, "member:tui/")
}

func issue31WriteCatalog(t *testing.T, root string, expected []catalog.Entry) {
	t.Helper()
	path := filepath.Join(root, "parity", "catalog.jsonl")
	entries, err := catalog.LoadCatalog(path)
	if err != nil {
		t.Fatalf("LoadCatalog: %v", err)
	}
	kept := entries[:0]
	for _, entry := range entries {
		if issue31CatalogID(entry.ID) {
			continue
		}
		if entry.ID == "module-tui" {
			entry.Status = catalog.StatusPartial
			entry.Evidence = []catalog.Evidence{
				{Kind: "go-test", Ref: "tui/issue31_surface_test.go", Baseline: issue31BaselineCommit},
				{Kind: "go-test", Ref: "tui/terminal_test.go", Baseline: issue31BaselineCommit},
				{Kind: "go-test", Ref: "tui/components_test.go", Baseline: issue31BaselineCommit},
				{Kind: "go-test", Ref: "tui/input_test.go", Baseline: issue31BaselineCommit},
			}
			entry.Partial = &catalog.Partial{
				Supported: []string{
					"all fixed-snapshot TUI symbols and extracted members have compile-usable Go mappings, with safe local constructors, value access, callback wiring, container composition and kill-ring behavior",
					"the public scaffold and its tests remain CGO-free and do not access ambient process input, terminal output, timers or native helpers",
				},
				Unsupported: []string{
					"interactive terminal lifecycle, input parsing, rendering, layout, autocomplete, terminal-image and platform-helper behavior remain explicit Capability Stubs until M6",
					"the API and Catalog checks prove contract coverage and target resolution, not runtime parity for the unsupported branches",
				},
			}
			entry.Notes = "Issue #31 maps all 191 fixed-snapshot TUI symbols and all 1,035 extracted members across 38 TypeScript root and technically reachable deep-import export subpaths into the single canonical Go package github.com/nankedr/pig/tui. TypeScript default-library inherited String and Number members are inventoried on their Go carriers instead of becoming artificial methods. Safe local state behavior is implemented; interactive Capability Stubs fail explicitly without terminal input, output, mode changes, timers, native-helper loading, or other side effects, and the core remains CGO-free."
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

func issue31CatalogSymbolEntries(t *testing.T, root string) []catalog.Entry {
	t.Helper()
	entries, err := catalog.LoadCatalog(filepath.Join(root, "parity", "catalog.jsonl"))
	if err != nil {
		t.Fatalf("LoadCatalog: %v", err)
	}
	var filtered []catalog.Entry
	for _, entry := range entries {
		if entry.Mapping.Module == "tui" && entry.Mapping.Kind == "symbol" {
			filtered = append(filtered, entry)
		}
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].ID < filtered[j].ID })
	return filtered
}

func issue31CountCatalogKind(entries []catalog.Entry, kind string) int {
	count := 0
	for _, entry := range entries {
		if entry.Mapping.Kind == kind {
			count++
		}
	}
	return count
}

func issue31CountCatalogStatus(entries []catalog.Entry, status string) int {
	count := 0
	for _, entry := range entries {
		if entry.Status == status {
			count++
		}
	}
	return count
}

func issue31HasEvidenceRef(evidence []catalog.Evidence, ref string) bool {
	for _, item := range evidence {
		if item.Ref == ref {
			return true
		}
	}
	return false
}

func issue31TUISymbol(symbol surface.Symbol) bool {
	return symbol.Module == "tui" && symbol.Upstream.Module == "tui" && strings.HasPrefix(symbol.Upstream.Reference, "packages/tui/src/")
}

func issue31RepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
}

var issue31NameExceptions = map[string]string{
	"LAYOUT_NODE":       "LayoutNodeMarker",
	"VIEWPORT_TUI":      "ViewportTUIMarker",
	"CURSOR_MARKER":     "CursorMarker",
	"TUI_KEYBINDINGS":   "TUIKeybindings",
	"PUNCTUATION_REGEX": "PunctuationRegex",
}

var issue31SymbolTargetExceptions = map[string]string{
	"symbol:tui/src/fuzzy.ts#FuzzyMatch": "FuzzyMatchResult",
}

// issue31MemberTargetExceptions records genuine Go carrier seams where a
// TypeScript member cannot live directly on the mechanically named parent.
// Every target remains subject to the AST resolver above.
var issue31MemberTargetExceptions = map[string]string{
	"Box.setBgFn":                               "Box.SetBGFunc",
	"Component.handleInput":                     "ComponentInputHandler.HandleInput",
	"Component.wantsKeyRelease":                 "KeyReleaseRequester.WantsKeyRelease",
	"Image.getImageId":                          "Image.GetImageID",
	"MarkdownOptions.renderLatex":               "MarkdownOptions.RenderLatex",
	"MarkdownTheme.hr":                          "MarkdownTheme.HR",
	"EditorComponent.addToHistory":              "EditorHistoryComponent.AddToHistory",
	"EditorComponent.borderColor":               "EditorBorderColorSetter.SetBorderColor",
	"EditorComponent.getExpandedText":           "EditorExpandedTextComponent.GetExpandedText",
	"EditorComponent.insertTextAtCursor":        "EditorInsertionComponent.InsertTextAtCursor",
	"EditorComponent.onChange":                  "EditorChangeSetter.SetOnChange",
	"EditorComponent.onSubmit":                  "EditorSubmitSetter.SetOnSubmit",
	"EditorComponent.setAutocompleteMaxVisible": "AutocompleteSizeEditorComponent.SetAutocompleteMaxVisible",
	"EditorComponent.setAutocompleteProvider":   "AutocompleteEditorComponent.SetAutocompleteProvider",
	"EditorComponent.setPaddingX":               "PaddedEditorComponent.SetPaddingX",
	"EditorComponent.wantsKeyRelease":           "KeyReleaseRequester.WantsKeyRelease",
	"Focusable.focused":                         "Focusable.SetFocusState",
	"LayoutComponent.handleInput":               "ComponentInputHandler.HandleInput",
	"LayoutComponent.wantsKeyRelease":           "KeyReleaseRequester.WantsKeyRelease",
	"LayoutNode.type":                           "LayoutNode.NodeType",
	"ScrollLayoutState.overscroll":              "ScrollLayoutState.OverscrollBehavior",
	"ScrollLayoutState.primary":                 "ScrollLayoutState.IsPrimary",
	"TUI.onDebug":                               "TUI.SetOnDebug",
	"TuiAltScreen.onDebug":                      "TUIAltScreen.SetOnDebug",
	"TuiBase.onDebug":                           "TUIBase.SetOnDebug",
	"TuiMainScreen.onDebug":                     "TUIMainScreen.SetOnDebug",
	"ViewportTUI.onDebug":                       "ViewportTUI.SetOnDebug",
}

var issue31StringInheritedMembers = []string{
	"anchor", "at", "big", "blink", "bold", "charAt", "charCodeAt", "codePointAt", "concat",
	"endsWith", "fixed", "fontcolor", "fontsize", "includes", "indexOf", "italics",
	"lastIndexOf", "length", "link", "localeCompare", "match", "matchAll", "normalize",
	"padEnd", "padStart", "repeat", "replace", "replaceAll", "search", "slice", "small",
	"split", "startsWith", "strike", "sub", "substr", "substring", "sup",
	"toLocaleLowerCase", "toLocaleUpperCase", "toLowerCase", "toString", "toUpperCase",
	"trim", "trimEnd", "trimLeft", "trimRight", "trimStart", "valueOf",
}

var issue31NumberInheritedMembers = []string{"toLocaleString", "toString", "valueOf"}

var issue31ExportSubpathCounts = map[string]int{
	".":                             129,
	"autocomplete":                  5,
	"components/alt-screen-flash":   1,
	"components/box":                1,
	"components/cancellable-loader": 1,
	"components/editor":             5,
	"components/h-stack":            1,
	"components/image":              3,
	"components/input":              1,
	"components/loader":             2,
	"components/markdown":           4,
	"components/scroll-view":        3,
	"components/select-list":        5,
	"components/settings-list":      4,
	"components/spacer":             1,
	"components/stack":              7,
	"components/text":               1,
	"components/truncated-text":     1,
	"components/v-stack":            5,
	"editor-component":              1,
	"fuzzy":                         3,
	"keybindings":                   10,
	"keys":                          11,
	"kill-ring":                     1,
	"latex":                         2,
	"layout":                        8,
	"layout-node":                   9,
	"native-modifiers":              2,
	"stdin-buffer":                  3,
	"terminal":                      7,
	"terminal-colors":               5,
	"terminal-image":                35,
	"tui":                           22,
	"tui-alt-screen":                2,
	"tui-main-screen":               2,
	"undo-stack":                    1,
	"utils":                         18,
	"word-navigation":               3,
}
