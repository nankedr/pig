package surface_test

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/nankedr/pig/internal/catalog"
	"github.com/nankedr/pig/internal/surface"
)

// updateManifest regenerates parity/surface/manifest.json from the committed
// symbols.jsonl when set (go test ./internal/surface -run GenerateManifest
// -update). The manifest is a derived artifact (counts + baseline anchor), so
// this keeps it in sync via BuildManifest rather than by hand-editing. It is
// pure-Go and offline: it reads only the committed surface view, needing no Node
// and no Pi checkout.
var updateManifest = flag.Bool("update", false, "regenerate the committed surface manifest from symbols.jsonl")

const baselineCommit = "936aff00918de1187f085f123c2812d8f2d67745"

// repoRoot locates the repository root relative to this test file, mirroring the
// runtime.Caller approach used by internal/inventory and internal/catalog.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

// realCatalogIDs loads the committed Parity Catalog and returns the set of entry
// ids, so surface validation resolves owning module entries against the real
// authority rather than a hand-maintained list.
func realCatalogIDs(t *testing.T, root string) map[string]bool {
	t.Helper()
	entries, err := catalog.LoadCatalog(filepath.Join(root, "parity", "catalog.jsonl"))
	if err != nil {
		t.Fatalf("LoadCatalog: %v", err)
	}
	ids := make(map[string]bool, len(entries))
	for _, e := range entries {
		ids[e.ID] = true
	}
	return ids
}

// sym builds a valid symbol for the given module and name so tests read as data.
func sym(module, relpath, name, kind string) surface.Symbol {
	mod := moduleByPig(module)
	ref := mod.UpstreamDir + "/" + relpath + "#" + name
	return surface.Symbol{
		SchemaVersion: surface.SchemaVersion,
		ID:            "symbol:" + module + "/" + relpath + "#" + name,
		Module:        module,
		Name:          name,
		Kind:          kind,
		Constructible: kind == "class",
		Upstream: surface.Upstream{
			Module:     mod.UpstreamName(),
			Repository: surface.Repository,
			Commit:     baselineCommit,
			Reference:  ref,
		},
		ExportSubpaths: []string{"."},
		Members:        []string{},
		StaticMembers:  []string{},
	}
}

func moduleByPig(pig string) surface.Module {
	for _, m := range surface.Modules {
		if m.PigModule == pig {
			return m
		}
	}
	panic("unknown pig module in test: " + pig)
}

// baseSymbols returns a small, fully valid synthetic surface: one symbol per
// in-scope module. Rejection tests mutate a fresh copy so cases stay independent.
func baseSymbols() ([]surface.Symbol, surface.Manifest, map[string]bool) {
	symbols := []surface.Symbol{
		sym("agent", "src/index.ts", "Agent", "class"),
		sym("ai", "src/index.ts", "streamText", "function"),
		sym("client", "src/index.ts", "Client", "class"),
		sym("codingagent", "src/index.ts", "RpcCommand", "type"),
		sym("protocol", "src/framing.ts", "encodeFrame", "function"),
		sym("telemetry", "src/index.ts", "Telemetry", "interface"),
		sym("tui", "src/index.ts", "render", "function"),
	}
	// Give one symbol members so member coverage is exercised.
	symbols[0].Members = []string{"abort", "continue", "followUp"}
	symbols[0].StaticMembers = []string{"create", "fromJSON"}
	manifest := surface.BuildManifest(symbols, baselineCommit)
	ids := map[string]bool{
		"module-agent": true, "module-ai": true, "module-client": true,
		"module-codingagent": true, "module-protocol": true,
		"module-telemetry": true, "module-tui": true,
	}
	return symbols, manifest, ids
}

func TestValidateAcceptsSyntheticSurface(t *testing.T) {
	symbols, manifest, ids := baseSymbols()
	if err := surface.Validate(symbols, manifest, ids); err != nil {
		t.Fatalf("Validate(valid surface) = %v", err)
	}
}

func TestBuildManifestCountsConstructibleSymbols(t *testing.T) {
	symbols, manifest, _ := baseSymbols()
	if manifest.ConstructorCount != 2 {
		t.Fatalf("BuildManifest(%d symbols).ConstructorCount = %d, want 2", len(symbols), manifest.ConstructorCount)
	}
}

func TestLoadSymbolsRequiresConstructible(t *testing.T) {
	symbol := sym("ai", "src/index.ts", "streamText", "function")
	data, err := surface.EncodeSymbols([]surface.Symbol{symbol})
	if err != nil {
		t.Fatalf("EncodeSymbols: %v", err)
	}
	if !strings.Contains(string(data), `"constructible":false`) {
		t.Fatalf("EncodeSymbols omitted explicit false constructible field: %s", data)
	}

	t.Run("explicit false", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "symbols.jsonl")
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := surface.LoadSymbols(path)
		if err != nil {
			t.Fatalf("LoadSymbols: %v", err)
		}
		if len(got) != 1 || got[0].Constructible {
			t.Fatalf("LoadSymbols explicit false = %+v, want one non-constructible symbol", got)
		}
	})

	t.Run("missing", func(t *testing.T) {
		missing := strings.Replace(string(data), `"constructible":false,`, "", 1)
		path := filepath.Join(t.TempDir(), "symbols.jsonl")
		if err := os.WriteFile(path, append(data, missing...), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := surface.LoadSymbols(path)
		if err == nil {
			t.Fatal("LoadSymbols(missing constructible) = nil, want error")
		}
		if !errors.Is(err, surface.ErrMalformedLine) {
			t.Fatalf("errors.Is(%v, ErrMalformedLine) = false", err)
		}
		var perr *surface.ParseError
		if !errors.As(err, &perr) {
			t.Fatalf("errors.As(%v, *ParseError) = false", err)
		}
		if perr.Line != 2 {
			t.Fatalf("ParseError.Line = %d, want 2", perr.Line)
		}
		if message := err.Error(); !strings.Contains(message, "constructible") || !strings.Contains(message, "missing") {
			t.Fatalf("LoadSymbols error = %q, want missing constructible cause", message)
		}
	})
}

func TestValidateRejections(t *testing.T) {
	for _, tt := range []struct {
		name     string
		mutate   func(symbols []surface.Symbol, m *surface.Manifest, ids map[string]bool) []surface.Symbol
		sentinel error
		kind     surface.Kind
	}{
		{
			name: "duplicate id",
			mutate: func(symbols []surface.Symbol, m *surface.Manifest, _ map[string]bool) []surface.Symbol {
				dup := symbols[1]
				dup.ID = symbols[0].ID
				symbols = append(symbols, dup)
				*m = surface.BuildManifest(symbols, baselineCommit)
				return symbols
			},
			sentinel: surface.ErrDuplicateID,
			kind:     surface.KindDuplicateID,
		},
		{
			name: "missing name",
			mutate: func(symbols []surface.Symbol, _ *surface.Manifest, _ map[string]bool) []surface.Symbol {
				symbols[0].Name = ""
				return symbols
			},
			sentinel: surface.ErrMissingField,
			kind:     surface.KindMissingField,
		},
		{
			name: "illegal kind",
			mutate: func(symbols []surface.Symbol, _ *surface.Manifest, _ map[string]bool) []surface.Symbol {
				symbols[0].Kind = "widget"
				return symbols
			},
			sentinel: surface.ErrIllegalKind,
			kind:     surface.KindIllegalKind,
		},
		{
			name: "non-class marked constructible",
			mutate: func(symbols []surface.Symbol, m *surface.Manifest, _ map[string]bool) []surface.Symbol {
				symbols[1].Constructible = true
				*m = surface.BuildManifest(symbols, baselineCommit)
				return symbols
			},
			sentinel: surface.ErrIllegalKind,
			kind:     surface.KindIllegalKind,
		},
		{
			name: "unknown module",
			mutate: func(symbols []surface.Symbol, _ *surface.Manifest, _ map[string]bool) []surface.Symbol {
				symbols[0].Module = "server"
				return symbols
			},
			sentinel: surface.ErrUnknownModule,
			kind:     surface.KindUnknownModule,
		},
		{
			name: "id name disagrees with name",
			mutate: func(symbols []surface.Symbol, _ *surface.Manifest, _ map[string]bool) []surface.Symbol {
				symbols[0].Name = "Renamed"
				return symbols
			},
			sentinel: surface.ErrMalformedID,
			kind:     surface.KindMalformedID,
		},
		{
			name: "id missing symbol prefix",
			mutate: func(symbols []surface.Symbol, _ *surface.Manifest, _ map[string]bool) []surface.Symbol {
				symbols[0].ID = "agent/src/index.ts#Agent"
				return symbols
			},
			sentinel: surface.ErrMalformedID,
			kind:     surface.KindMalformedID,
		},
		{
			name: "upstream module not upstream name",
			mutate: func(symbols []surface.Symbol, _ *surface.Manifest, _ map[string]bool) []surface.Symbol {
				// codingagent's upstream module must be coding-agent.
				symbols[3].Upstream.Module = "codingagent"
				return symbols
			},
			sentinel: surface.ErrProvenance,
			kind:     surface.KindProvenance,
		},
		{
			name: "reference prefix mismatch",
			mutate: func(symbols []surface.Symbol, _ *surface.Manifest, _ map[string]bool) []surface.Symbol {
				symbols[0].Upstream.Reference = "packages/ai/src/index.ts#Agent"
				return symbols
			},
			sentinel: surface.ErrProvenance,
			kind:     surface.KindProvenance,
		},
		{
			name: "commit mismatch",
			mutate: func(symbols []surface.Symbol, _ *surface.Manifest, _ map[string]bool) []surface.Symbol {
				symbols[0].Upstream.Commit = "0000000000000000000000000000000000000000"
				return symbols
			},
			sentinel: surface.ErrCommitMismatch,
			kind:     surface.KindCommitMismatch,
		},
		{
			name: "module unmapped in catalog",
			mutate: func(symbols []surface.Symbol, _ *surface.Manifest, ids map[string]bool) []surface.Symbol {
				delete(ids, "module-agent")
				return symbols
			},
			sentinel: surface.ErrUnmapped,
			kind:     surface.KindUnmapped,
		},
		{
			name: "module uncovered",
			mutate: func(symbols []surface.Symbol, m *surface.Manifest, _ map[string]bool) []surface.Symbol {
				kept := symbols[1:] // drop the only agent symbol
				*m = surface.BuildManifest(kept, baselineCommit)
				return kept
			},
			sentinel: surface.ErrModuleUncovered,
			kind:     surface.KindModuleUncovered,
		},
		{
			name: "members not sorted",
			mutate: func(symbols []surface.Symbol, _ *surface.Manifest, _ map[string]bool) []surface.Symbol {
				symbols[0].Members = []string{"b", "a"}
				return symbols
			},
			sentinel: surface.ErrNotSorted,
			kind:     surface.KindNotSorted,
		},
		{
			name: "members not unique",
			mutate: func(symbols []surface.Symbol, _ *surface.Manifest, _ map[string]bool) []surface.Symbol {
				symbols[0].Members = []string{"a", "a"}
				return symbols
			},
			sentinel: surface.ErrNotSorted,
			kind:     surface.KindNotSorted,
		},
		{
			name: "static members missing",
			mutate: func(symbols []surface.Symbol, _ *surface.Manifest, _ map[string]bool) []surface.Symbol {
				symbols[0].StaticMembers = nil
				return symbols
			},
			sentinel: surface.ErrMissingField,
			kind:     surface.KindMissingField,
		},
		{
			name: "static members not sorted",
			mutate: func(symbols []surface.Symbol, _ *surface.Manifest, _ map[string]bool) []surface.Symbol {
				symbols[0].StaticMembers = []string{"fromJSON", "create"}
				return symbols
			},
			sentinel: surface.ErrNotSorted,
			kind:     surface.KindNotSorted,
		},
		{
			name: "static members not unique",
			mutate: func(symbols []surface.Symbol, _ *surface.Manifest, _ map[string]bool) []surface.Symbol {
				symbols[0].StaticMembers = []string{"create", "create"}
				return symbols
			},
			sentinel: surface.ErrNotSorted,
			kind:     surface.KindNotSorted,
		},
		{
			name: "empty static member name",
			mutate: func(symbols []surface.Symbol, _ *surface.Manifest, _ map[string]bool) []surface.Symbol {
				symbols[0].StaticMembers = []string{""}
				return symbols
			},
			sentinel: surface.ErrMissingField,
			kind:     surface.KindMissingField,
		},
		{
			name: "empty export subpaths",
			mutate: func(symbols []surface.Symbol, _ *surface.Manifest, _ map[string]bool) []surface.Symbol {
				symbols[0].ExportSubpaths = nil
				return symbols
			},
			sentinel: surface.ErrMissingField,
			kind:     surface.KindMissingField,
		},
		{
			name: "symbol count mismatch",
			mutate: func(symbols []surface.Symbol, m *surface.Manifest, _ map[string]bool) []surface.Symbol {
				m.SymbolCount = 999
				return symbols
			},
			sentinel: surface.ErrManifestMismatch,
			kind:     surface.KindManifestMismatch,
		},
		{
			name: "member count mismatch",
			mutate: func(symbols []surface.Symbol, m *surface.Manifest, _ map[string]bool) []surface.Symbol {
				m.MemberCount = 999
				return symbols
			},
			sentinel: surface.ErrManifestMismatch,
			kind:     surface.KindManifestMismatch,
		},
		{
			name: "static member count mismatch",
			mutate: func(symbols []surface.Symbol, m *surface.Manifest, _ map[string]bool) []surface.Symbol {
				m.StaticMemberCount = 999
				return symbols
			},
			sentinel: surface.ErrManifestMismatch,
			kind:     surface.KindManifestMismatch,
		},
		{
			name: "constructor count mismatch",
			mutate: func(symbols []surface.Symbol, m *surface.Manifest, _ map[string]bool) []surface.Symbol {
				m.ConstructorCount = 999
				return symbols
			},
			sentinel: surface.ErrManifestMismatch,
			kind:     surface.KindManifestMismatch,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			symbols, manifest, ids := baseSymbols()
			symbols = tt.mutate(symbols, &manifest, ids)
			err := surface.Validate(symbols, manifest, ids)
			if err == nil {
				t.Fatalf("Validate(%s) = nil, want error", tt.name)
			}
			if !errors.Is(err, tt.sentinel) {
				t.Fatalf("errors.Is(%v, %v) = false", err, tt.sentinel)
			}
			var serr *surface.Error
			if !errors.As(err, &serr) {
				t.Fatalf("errors.As(%v, *surface.Error) = false", err)
			}
			if serr.Kind != tt.kind {
				t.Fatalf("Error.Kind = %q, want %q", serr.Kind, tt.kind)
			}
		})
	}
}

// TestEncodeSymbolsRoundTrip verifies symbols survive an encode/load cycle and
// stay sorted by id.
func TestEncodeSymbolsRoundTrip(t *testing.T) {
	symbols, _, _ := baseSymbols()
	data, err := surface.EncodeSymbols(symbols)
	if err != nil {
		t.Fatalf("EncodeSymbols: %v", err)
	}
	tmp := filepath.Join(t.TempDir(), "symbols.jsonl")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	got, err := surface.LoadSymbols(tmp)
	if err != nil {
		t.Fatalf("LoadSymbols: %v", err)
	}
	if len(got) != len(symbols) {
		t.Fatalf("round-trip len = %d, want %d", len(got), len(symbols))
	}
	if got[0].Constructible != symbols[0].Constructible {
		t.Fatalf("round-trip constructible = %t, want %t", got[0].Constructible, symbols[0].Constructible)
	}
	if !slices.Equal(got[0].StaticMembers, symbols[0].StaticMembers) {
		t.Fatalf("round-trip static members = %v, want %v", got[0].StaticMembers, symbols[0].StaticMembers)
	}
	ids := make([]string, len(got))
	for i, s := range got {
		ids[i] = s.ID
	}
	if !sort.StringsAreSorted(ids) {
		t.Fatalf("encoded symbols not sorted by id: %v", ids)
	}
}

func TestLoadSymbolsMalformedLine(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "symbols.jsonl")
	content := `{"schema_version":"1.1.0","id":"symbol:agent/src/index.ts#Agent","module":"agent","name":"Agent","kind":"class","constructible":true,"upstream":{"module":"agent","repository":"https://github.com/badlogic/pi-mono","commit":"936aff00918de1187f085f123c2812d8f2d67745","reference":"packages/agent/src/index.ts#Agent"},"export_subpaths":["."],"members":[],"static_members":[]}
{"id": not json}
`
	if err := os.WriteFile(tmp, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp: %v", err)
	}
	_, err := surface.LoadSymbols(tmp)
	if err == nil {
		t.Fatal("LoadSymbols(malformed) = nil, want error")
	}
	if !errors.Is(err, surface.ErrMalformedLine) {
		t.Fatalf("errors.Is(%v, ErrMalformedLine) = false", err)
	}
	var perr *surface.ParseError
	if !errors.As(err, &perr) {
		t.Fatalf("errors.As(%v, *ParseError) = false", err)
	}
	if perr.Line != 2 {
		t.Fatalf("ParseError.Line = %d, want 2", perr.Line)
	}
}

// TestLoadAndValidateRealSurface is the offline consistency check that runs in
// normal `go test`: the committed symbols.jsonl and manifest.json must load,
// validate against the real Parity Catalog ids, cover every in-scope module and
// carry the locked baseline commit.
func TestLoadAndValidateRealSurface(t *testing.T) {
	root := repoRoot(t)
	symbols, err := surface.LoadSymbols(filepath.Join(root, "parity", "surface", "symbols.jsonl"))
	if err != nil {
		t.Fatalf("LoadSymbols(real) = %v", err)
	}
	if len(symbols) == 0 {
		t.Fatal("real surface has no symbols")
	}

	// The canonical JSONL must be sorted by id for determinism.
	ids := make([]string, len(symbols))
	for i, s := range symbols {
		ids[i] = s.ID
	}
	if !sort.StringsAreSorted(ids) {
		t.Fatal("real surface not sorted by id")
	}

	manifest, err := surface.LoadManifest(filepath.Join(root, "parity", "surface", "manifest.json"))
	if err != nil {
		t.Fatalf("LoadManifest(real) = %v", err)
	}
	if manifest.BaselineCommit != baselineCommit {
		t.Fatalf("manifest baseline = %q, want %q", manifest.BaselineCommit, baselineCommit)
	}
	if manifest.SchemaVersion != surface.SchemaVersion {
		t.Fatalf("manifest schema = %q, want %q", manifest.SchemaVersion, surface.SchemaVersion)
	}
	if manifest.ConstructorCount != 110 {
		t.Fatalf("manifest constructor_count = %d, want 110", manifest.ConstructorCount)
	}

	if err := surface.Validate(symbols, manifest, realCatalogIDs(t, root)); err != nil {
		t.Fatalf("Validate(real surface) = %v", err)
	}

	byID := make(map[string]surface.Symbol, len(symbols))
	constructorCount := 0
	codingAgentConstructorCount := 0
	for _, symbol := range symbols {
		if symbol.StaticMembers == nil {
			t.Fatalf("%s has no static_members array", symbol.ID)
		}
		if symbol.Constructible {
			constructorCount++
			if symbol.Module == "codingagent" {
				codingAgentConstructorCount++
			}
		}
		byID[symbol.ID] = symbol
	}
	if constructorCount != 110 {
		t.Fatalf("real surface constructor count = %d, want 110", constructorCount)
	}
	if codingAgentConstructorCount != 38 {
		t.Fatalf("real codingagent constructor count = %d, want 38", codingAgentConstructorCount)
	}
	nonConstructibleClasses := make([]string, 0, 7)
	for _, symbol := range symbols {
		if symbol.Kind == "class" && !symbol.Constructible {
			nonConstructibleClasses = append(nonConstructibleClasses, symbol.ID)
		}
	}
	wantNonConstructibleClasses := []string{
		"symbol:agent/src/harness/agent-harness.ts#AgentHarness",
		"symbol:codingagent/src/client/remote-session.ts#RemoteSession",
		"symbol:codingagent/src/core/model-runtime.ts#ModelRuntime",
		"symbol:codingagent/src/core/session-manager.ts#SessionManager",
		"symbol:codingagent/src/core/settings-manager.ts#SettingsManager",
		"symbol:tui/src/components/stack.ts#Stack",
		"symbol:tui/src/tui.ts#TuiBase",
	}
	if !slices.Equal(nonConstructibleClasses, wantNonConstructibleClasses) {
		t.Fatalf("real non-constructible classes = %v, want %v", nonConstructibleClasses, wantNonConstructibleClasses)
	}
	for id, member := range map[string]string{
		"symbol:agent/src/harness/result.ts#TaggedErrorValue":           "_tag",
		"symbol:codingagent/src/core/session-manager.ts#SessionManager": "_persist",
	} {
		symbol, ok := byID[id]
		if !ok {
			t.Fatalf("real surface missing %s", id)
		}
		if !slices.Contains(symbol.Members, member) {
			t.Errorf("%s members = %v, want public underscore member %q", id, symbol.Members, member)
		}
	}
	for id, want := range map[string][]string{
		"symbol:agent/src/harness/agent-harness.ts#AgentHarness":          {"create"},
		"symbol:agent/src/harness/agent-harness.ts#Closed":                {"is"},
		"symbol:agent/src/harness/agent-harness.ts#HarnessFault":          {},
		"symbol:agent/src/harness/agent-harness.ts#InvalidLane":           {"is"},
		"symbol:agent/src/harness/agent-harness.ts#InvalidMessage":        {"is"},
		"symbol:agent/src/harness/agent-harness.ts#LaneBusy":              {"is"},
		"symbol:agent/src/harness/agent-harness.ts#LaneExists":            {"is"},
		"symbol:agent/src/harness/agent-harness.ts#MissingIdentities":     {"is"},
		"symbol:agent/src/harness/agent-harness.ts#NoActiveOperation":     {"is"},
		"symbol:agent/src/harness/agent-harness.ts#NoActiveRun":           {"is"},
		"symbol:agent/src/harness/agent-harness.ts#NothingToCompact":      {"is"},
		"symbol:agent/src/harness/agent-harness.ts#NothingToResume":       {"is"},
		"symbol:agent/src/harness/agent-harness.ts#UnknownQueueItem":      {"is"},
		"symbol:agent/src/harness/agent-harness.ts#UnknownSkill":          {"is"},
		"symbol:agent/src/harness/agent-harness.ts#UnknownTarget":         {"is"},
		"symbol:agent/src/harness/agent-harness.ts#UnknownTemplate":       {"is"},
		"symbol:client/src/client.ts#PiClient":                            {"connect"},
		"symbol:codingagent/src/client/remote-session.ts#RemoteSession":   {"create", "open"},
		"symbol:codingagent/src/core/settings-manager.ts#SettingsManager": {"create", "fromStorage", "inMemory"},
		"symbol:codingagent/src/core/session-manager.ts#SessionManager":   {"continueRecent", "create", "forkFrom", "inMemory", "list", "listAll", "open"},
	} {
		got, ok := byID[id]
		if !ok {
			t.Fatalf("real surface missing %s", id)
		}
		if !slices.Equal(got.StaticMembers, want) {
			t.Errorf("%s static_members = %v, want %v", id, got.StaticMembers, want)
		}
	}

	keybindingsManagerID := "symbol:codingagent/src/core/keybindings.ts#KeybindingsManager"
	keybindingsManager, ok := byID[keybindingsManagerID]
	if !ok {
		t.Fatalf("real surface missing %s", keybindingsManagerID)
	}
	wantKeybindingsManagerMembers := []string{
		"getConflicts",
		"getDefinition",
		"getEffectiveConfig",
		"getKeys",
		"getResolvedBindings",
		"getUserBindings",
		"matches",
		"reload",
		"setUserBindings",
	}
	if !slices.Equal(keybindingsManager.Members, wantKeybindingsManagerMembers) {
		t.Errorf("%s members = %v, want %v; Pi-owned inherited members must remain in the surface", keybindingsManagerID, keybindingsManager.Members, wantKeybindingsManagerMembers)
	}

	stdinBufferID := "symbol:tui/src/stdin-buffer.ts#StdinBuffer"
	stdinBuffer, ok := byID[stdinBufferID]
	if !ok {
		t.Fatalf("real surface missing %s", stdinBufferID)
	}
	wantStdinBufferMembers := []string{"clear", "destroy", "flush", "getBuffer", "process"}
	if !slices.Equal(stdinBuffer.Members, wantStdinBufferMembers) {
		t.Errorf("%s members = %v, want %v; inherited EventEmitter members must not leak into the surface", stdinBufferID, stdinBuffer.Members, wantStdinBufferMembers)
	}
}

func TestRealSurfaceStaticMembersAreCataloged(t *testing.T) {
	root := repoRoot(t)
	symbols, err := surface.LoadSymbols(filepath.Join(root, "parity", "surface", "symbols.jsonl"))
	if err != nil {
		t.Fatalf("LoadSymbols(real) = %v", err)
	}
	catalogIDs := realCatalogIDs(t, root)
	staticMemberCount := 0
	for _, symbol := range symbols {
		for _, member := range symbol.StaticMembers {
			staticMemberCount++
			id := "static-member:" + strings.TrimPrefix(symbol.ID, "symbol:") + "." + member
			if !catalogIDs[id] {
				t.Errorf("surface static member has no catalog row: %s", id)
			}
		}
	}
	if staticMemberCount != 30 {
		t.Fatalf("real surface static-member count = %d, want 30", staticMemberCount)
	}
}

// TestGenerateManifest regenerates the committed surface manifest from the
// committed symbols.jsonl when run with -update (go test ./internal/surface -run
// GenerateManifest -update). It is pure-Go and offline; without -update it is a
// no-op skip so it never runs during normal `go test`. The symbols.jsonl itself
// is regenerated by parity/extract/surface.mjs (the only Node step).
func TestGenerateManifest(t *testing.T) {
	if !*updateManifest {
		t.Skip("set -update to regenerate the surface manifest from symbols.jsonl")
	}
	root := repoRoot(t)
	dir := filepath.Join(root, "parity", "surface")
	symbols, err := surface.LoadSymbols(filepath.Join(dir, "symbols.jsonl"))
	if err != nil {
		t.Fatalf("LoadSymbols: %v", err)
	}
	manifest := surface.BuildManifest(symbols, baselineCommit)

	// The regenerated manifest must satisfy the same validator as the committed
	// one, resolving owning module ids against the real catalog.
	if err := surface.Validate(symbols, manifest, realCatalogIDs(t, root)); err != nil {
		t.Fatalf("regenerated surface fails Validate: %v", err)
	}

	data, err := surface.EncodeManifest(manifest)
	if err != nil {
		t.Fatalf("EncodeManifest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), data, 0o644); err != nil {
		t.Fatalf("write manifest.json: %v", err)
	}
	t.Logf("regenerated surface manifest: %d symbols, %d members, %d static members, %d constructors", manifest.SymbolCount, manifest.MemberCount, manifest.StaticMemberCount, manifest.ConstructorCount)
}
