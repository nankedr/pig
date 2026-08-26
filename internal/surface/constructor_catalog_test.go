package surface_test

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/nankedr/pig/internal/catalog"
	"github.com/nankedr/pig/internal/surface"
)

var updateConstructors = flag.Bool("update-constructors", false, "replace non-Coding-Agent constructor rows in parity/catalog.jsonl")

const expectedConstructorCount = 110

var expectedConstructorCounts = map[string]int{
	"agent":       28,
	"ai":          6,
	"client":      6,
	"codingagent": 38,
	"protocol":    6,
	"telemetry":   1,
	"tui":         25,
}

type constructorMappingSpec struct {
	module       string
	target       string
	status       string
	milestone    string
	metadataFrom string
}

// nonCodingAgentConstructorMappings is the reviewed operation-level mapping
// for every constructor owned by this generator. Callable package factories are
// scaffolded; error and other non-callable shapes remain inventoried on their
// public carrier. Full symbol ids are keys because names such as
// KeybindingsManager occur in more than one module.
var nonCodingAgentConstructorMappings = map[string]constructorMappingSpec{
	"symbol:agent/src/agent.ts#Agent":                                   {module: "agent", target: "github.com/nankedr/pig/agent.NewAgent", status: catalog.StatusScaffolded, milestone: "M1"},
	"symbol:agent/src/harness/agent-harness.ts#Closed":                  {module: "agent", target: "github.com/nankedr/pig/agent.Closed", status: catalog.StatusInventoried, milestone: "M8"},
	"symbol:agent/src/harness/agent-harness.ts#HarnessClosed":           {module: "agent", target: "github.com/nankedr/pig/agent.HarnessClosed", status: catalog.StatusInventoried, milestone: "M8"},
	"symbol:agent/src/harness/agent-harness.ts#HarnessFault":            {module: "agent", target: "github.com/nankedr/pig/agent.HarnessFault", status: catalog.StatusInventoried, milestone: "M8"},
	"symbol:agent/src/harness/agent-harness.ts#HarnessNotImplemented":   {module: "agent", target: "github.com/nankedr/pig/agent.HarnessNotImplemented", status: catalog.StatusInventoried, milestone: "M8"},
	"symbol:agent/src/harness/agent-harness.ts#InvalidLane":             {module: "agent", target: "github.com/nankedr/pig/agent.InvalidLane", status: catalog.StatusInventoried, milestone: "M8"},
	"symbol:agent/src/harness/agent-harness.ts#InvalidMessage":          {module: "agent", target: "github.com/nankedr/pig/agent.InvalidMessage", status: catalog.StatusInventoried, milestone: "M8"},
	"symbol:agent/src/harness/agent-harness.ts#LaneBusy":                {module: "agent", target: "github.com/nankedr/pig/agent.LaneBusy", status: catalog.StatusInventoried, milestone: "M8"},
	"symbol:agent/src/harness/agent-harness.ts#LaneExists":              {module: "agent", target: "github.com/nankedr/pig/agent.LaneExists", status: catalog.StatusInventoried, milestone: "M8"},
	"symbol:agent/src/harness/agent-harness.ts#MissingIdentities":       {module: "agent", target: "github.com/nankedr/pig/agent.MissingIdentities", status: catalog.StatusInventoried, milestone: "M8"},
	"symbol:agent/src/harness/agent-harness.ts#NoActiveOperation":       {module: "agent", target: "github.com/nankedr/pig/agent.NoActiveOperation", status: catalog.StatusInventoried, milestone: "M8"},
	"symbol:agent/src/harness/agent-harness.ts#NoActiveRun":             {module: "agent", target: "github.com/nankedr/pig/agent.NoActiveRun", status: catalog.StatusInventoried, milestone: "M8"},
	"symbol:agent/src/harness/agent-harness.ts#NothingToCompact":        {module: "agent", target: "github.com/nankedr/pig/agent.NothingToCompact", status: catalog.StatusInventoried, milestone: "M8"},
	"symbol:agent/src/harness/agent-harness.ts#NothingToResume":         {module: "agent", target: "github.com/nankedr/pig/agent.NothingToResume", status: catalog.StatusInventoried, milestone: "M8"},
	"symbol:agent/src/harness/agent-harness.ts#UnknownQueueItem":        {module: "agent", target: "github.com/nankedr/pig/agent.UnknownQueueItem", status: catalog.StatusInventoried, milestone: "M8"},
	"symbol:agent/src/harness/agent-harness.ts#UnknownSkill":            {module: "agent", target: "github.com/nankedr/pig/agent.UnknownSkill", status: catalog.StatusInventoried, milestone: "M8"},
	"symbol:agent/src/harness/agent-harness.ts#UnknownTarget":           {module: "agent", target: "github.com/nankedr/pig/agent.UnknownTarget", status: catalog.StatusInventoried, milestone: "M8"},
	"symbol:agent/src/harness/agent-harness.ts#UnknownTemplate":         {module: "agent", target: "github.com/nankedr/pig/agent.UnknownTemplate", status: catalog.StatusInventoried, milestone: "M8"},
	"symbol:agent/src/harness/env/nodejs.ts#NodeExecutionEnv":           {module: "agent", target: "github.com/nankedr/pig/agent/node.NewNodeExecutionEnv", status: catalog.StatusScaffolded, milestone: "M8"},
	"symbol:agent/src/harness/session/jsonl/repo.ts#JsonlSessionRepo":   {module: "agent", target: "github.com/nankedr/pig/agent.NewJSONLSessionRepo", status: catalog.StatusScaffolded, milestone: "M8"},
	"symbol:agent/src/harness/session/memory.ts#InMemorySessionRepo":    {module: "agent", target: "github.com/nankedr/pig/agent.NewInMemorySessionRepo", status: catalog.StatusScaffolded, milestone: "M8"},
	"symbol:agent/src/harness/session/memory.ts#InMemorySessionStorage": {module: "agent", target: "github.com/nankedr/pig/agent.NewInMemorySessionStorage", status: catalog.StatusScaffolded, milestone: "M8"},
	"symbol:agent/src/harness/session/session.ts#Session":               {module: "agent", target: "github.com/nankedr/pig/agent.NewSession", status: catalog.StatusScaffolded, milestone: "M8"},
	"symbol:agent/src/harness/session/types.ts#SessionError":            {module: "agent", target: "github.com/nankedr/pig/agent.SessionError", status: catalog.StatusInventoried, milestone: "M8"},
	"symbol:agent/src/harness/types.ts#BranchSummaryError":              {module: "agent", target: "github.com/nankedr/pig/agent.BranchSummaryError", status: catalog.StatusInventoried, milestone: "M8"},
	"symbol:agent/src/harness/types.ts#CompactionError":                 {module: "agent", target: "github.com/nankedr/pig/agent.CompactionError", status: catalog.StatusInventoried, milestone: "M8"},
	"symbol:agent/src/harness/types.ts#ExecutionError":                  {module: "agent", target: "github.com/nankedr/pig/agent.ExecutionError", status: catalog.StatusInventoried, milestone: "M8"},
	"symbol:agent/src/harness/types.ts#FileError":                       {module: "agent", target: "github.com/nankedr/pig/agent.FileError", status: catalog.StatusInventoried, milestone: "M8"},

	"symbol:ai/src/api/pi-messages.ts#PiMessagesResponseError":        {module: "ai", target: "github.com/nankedr/pig/ai.PiMessagesResponseError", status: catalog.StatusInventoried, milestone: "M11", metadataFrom: "contract:ai/api-entrypoints"},
	"symbol:ai/src/auth/credential-store.ts#InMemoryCredentialStore":  {module: "ai", target: "github.com/nankedr/pig/ai.NewInMemoryCredentialStore", status: catalog.StatusScaffolded, milestone: "M1", metadataFrom: "contract:ai/auth"},
	"symbol:ai/src/auth/resolve.ts#ModelsError":                       {module: "ai", target: "github.com/nankedr/pig/ai.ModelsError", status: catalog.StatusInventoried, milestone: "M1", metadataFrom: "contract:ai/models-runtime"},
	"symbol:ai/src/models-store.ts#InMemoryModelsStore":               {module: "ai", target: "github.com/nankedr/pig/ai.NewInMemoryModelsStore", status: catalog.StatusScaffolded, milestone: "M1", metadataFrom: "contract:ai/models-store"},
	"symbol:ai/src/utils/event-stream.ts#AssistantMessageEventStream": {module: "ai", target: "github.com/nankedr/pig/ai.NewAssistantMessageEventStream", status: catalog.StatusScaffolded, milestone: "M1", metadataFrom: "contract:ai/event-stream"},
	"symbol:ai/src/utils/event-stream.ts#EventStream":                 {module: "ai", target: "github.com/nankedr/pig/ai.NewEventStream", status: catalog.StatusScaffolded, milestone: "M1", metadataFrom: "contract:ai/event-stream"},

	"symbol:client/src/client.ts#PiClient":                {module: "client", target: "github.com/nankedr/pig/client.NewClient", status: catalog.StatusScaffolded, milestone: "M9", metadataFrom: "module-client"},
	"symbol:client/src/errors.ts#PiClientDisposedError":   {module: "client", target: "github.com/nankedr/pig/client.ClientDisposedError", status: catalog.StatusInventoried, milestone: "M9", metadataFrom: "module-client"},
	"symbol:client/src/errors.ts#PiDisconnectedError":     {module: "client", target: "github.com/nankedr/pig/client.DisconnectedError", status: catalog.StatusInventoried, milestone: "M9", metadataFrom: "module-client"},
	"symbol:client/src/errors.ts#PiServerError":           {module: "client", target: "github.com/nankedr/pig/client.NewServerError", status: catalog.StatusScaffolded, milestone: "M9", metadataFrom: "module-client"},
	"symbol:client/src/errors.ts#PiSessionDetachedError":  {module: "client", target: "github.com/nankedr/pig/client.SessionDetachedError", status: catalog.StatusInventoried, milestone: "M9", metadataFrom: "module-client"},
	"symbol:client/src/errors.ts#PiSessionOwnershipError": {module: "client", target: "github.com/nankedr/pig/client.SessionOwnershipError", status: catalog.StatusInventoried, milestone: "M9", metadataFrom: "module-client"},

	"symbol:protocol/src/cbor/options.ts#CborError":        {module: "protocol", target: "github.com/nankedr/pig/protocol.CBORError", status: catalog.StatusInventoried, milestone: "M9", metadataFrom: "module-protocol"},
	"symbol:protocol/src/codec.ts#ClientMessageDecoder":    {module: "protocol", target: "github.com/nankedr/pig/protocol.NewClientMessageDecoder", status: catalog.StatusScaffolded, milestone: "M9", metadataFrom: "module-protocol"},
	"symbol:protocol/src/codec.ts#ProtocolValidationError": {module: "protocol", target: "github.com/nankedr/pig/protocol.ProtocolValidationError", status: catalog.StatusInventoried, milestone: "M9", metadataFrom: "module-protocol"},
	"symbol:protocol/src/codec.ts#ServerMessageDecoder":    {module: "protocol", target: "github.com/nankedr/pig/protocol.NewServerMessageDecoder", status: catalog.StatusScaffolded, milestone: "M9", metadataFrom: "module-protocol"},
	"symbol:protocol/src/framing.ts#FrameDecoder":          {module: "protocol", target: "github.com/nankedr/pig/protocol.NewFrameDecoder", status: catalog.StatusScaffolded, milestone: "M9", metadataFrom: "module-protocol"},
	"symbol:protocol/src/framing.ts#FrameError":            {module: "protocol", target: "github.com/nankedr/pig/protocol.FrameError", status: catalog.StatusInventoried, milestone: "M9", metadataFrom: "module-protocol"},

	"symbol:telemetry/src/memory.ts#InMemoryTelemetryContext": {module: "telemetry", target: "github.com/nankedr/pig/telemetry.InMemoryTelemetryContext", status: catalog.StatusInventoried, milestone: "M2", metadataFrom: "module-telemetry"},

	"symbol:tui/src/autocomplete.ts#CombinedAutocompleteProvider":           {module: "tui", target: "github.com/nankedr/pig/tui.NewCombinedAutocompleteProvider", status: catalog.StatusScaffolded, milestone: "M6"},
	"symbol:tui/src/components/alt-screen-flash.ts#AltScreenFlashContainer": {module: "tui", target: "github.com/nankedr/pig/tui.NewAltScreenFlashContainer", status: catalog.StatusScaffolded, milestone: "M6"},
	"symbol:tui/src/components/box.ts#Box":                                  {module: "tui", target: "github.com/nankedr/pig/tui.NewBox", status: catalog.StatusScaffolded, milestone: "M6"},
	"symbol:tui/src/components/cancellable-loader.ts#CancellableLoader":     {module: "tui", target: "github.com/nankedr/pig/tui.NewCancellableLoader", status: catalog.StatusScaffolded, milestone: "M6"},
	"symbol:tui/src/components/editor.ts#Editor":                            {module: "tui", target: "github.com/nankedr/pig/tui.NewEditor", status: catalog.StatusScaffolded, milestone: "M6"},
	"symbol:tui/src/components/h-stack.ts#HStack":                           {module: "tui", target: "github.com/nankedr/pig/tui.NewHStack", status: catalog.StatusScaffolded, milestone: "M6"},
	"symbol:tui/src/components/image.ts#Image":                              {module: "tui", target: "github.com/nankedr/pig/tui.NewImage", status: catalog.StatusScaffolded, milestone: "M6"},
	"symbol:tui/src/components/input.ts#Input":                              {module: "tui", target: "github.com/nankedr/pig/tui.NewInput", status: catalog.StatusScaffolded, milestone: "M6"},
	"symbol:tui/src/components/loader.ts#Loader":                            {module: "tui", target: "github.com/nankedr/pig/tui.NewLoader", status: catalog.StatusScaffolded, milestone: "M6"},
	"symbol:tui/src/components/markdown.ts#Markdown":                        {module: "tui", target: "github.com/nankedr/pig/tui.NewMarkdown", status: catalog.StatusScaffolded, milestone: "M6"},
	"symbol:tui/src/components/scroll-view.ts#ScrollView":                   {module: "tui", target: "github.com/nankedr/pig/tui.NewScrollView", status: catalog.StatusScaffolded, milestone: "M6"},
	"symbol:tui/src/components/select-list.ts#SelectList":                   {module: "tui", target: "github.com/nankedr/pig/tui.NewSelectList", status: catalog.StatusScaffolded, milestone: "M6"},
	"symbol:tui/src/components/settings-list.ts#SettingsList":               {module: "tui", target: "github.com/nankedr/pig/tui.NewSettingsList", status: catalog.StatusScaffolded, milestone: "M6"},
	"symbol:tui/src/components/spacer.ts#Spacer":                            {module: "tui", target: "github.com/nankedr/pig/tui.NewSpacer", status: catalog.StatusScaffolded, milestone: "M6"},
	"symbol:tui/src/components/text.ts#Text":                                {module: "tui", target: "github.com/nankedr/pig/tui.NewText", status: catalog.StatusScaffolded, milestone: "M6"},
	"symbol:tui/src/components/truncated-text.ts#TruncatedText":             {module: "tui", target: "github.com/nankedr/pig/tui.NewTruncatedText", status: catalog.StatusScaffolded, milestone: "M6"},
	"symbol:tui/src/components/v-stack.ts#VStack":                           {module: "tui", target: "github.com/nankedr/pig/tui.NewVStack", status: catalog.StatusScaffolded, milestone: "M6"},
	"symbol:tui/src/keybindings.ts#KeybindingsManager":                      {module: "tui", target: "github.com/nankedr/pig/tui.NewKeybindingsManager", status: catalog.StatusScaffolded, milestone: "M6"},
	"symbol:tui/src/kill-ring.ts#KillRing":                                  {module: "tui", target: "github.com/nankedr/pig/tui.NewKillRing", status: catalog.StatusScaffolded, milestone: "M6"},
	"symbol:tui/src/stdin-buffer.ts#StdinBuffer":                            {module: "tui", target: "github.com/nankedr/pig/tui.NewStdinBuffer", status: catalog.StatusScaffolded, milestone: "M6"},
	"symbol:tui/src/terminal.ts#ProcessTerminal":                            {module: "tui", target: "github.com/nankedr/pig/tui.NewProcessTerminal", status: catalog.StatusScaffolded, milestone: "M6"},
	"symbol:tui/src/tui-alt-screen.ts#TuiAltScreen":                         {module: "tui", target: "github.com/nankedr/pig/tui.NewTUIAltScreen", status: catalog.StatusScaffolded, milestone: "M6"},
	"symbol:tui/src/tui-main-screen.ts#TuiMainScreen":                       {module: "tui", target: "github.com/nankedr/pig/tui.NewTUIMainScreen", status: catalog.StatusScaffolded, milestone: "M6"},
	"symbol:tui/src/tui.ts#Container":                                       {module: "tui", target: "github.com/nankedr/pig/tui.NewContainer", status: catalog.StatusScaffolded, milestone: "M6"},
	"symbol:tui/src/undo-stack.ts#UndoStack":                                {module: "tui", target: "github.com/nankedr/pig/tui.UndoStack", status: catalog.StatusInventoried, milestone: "M6"},
}

func TestRealSurfaceConstructorsAreCataloged(t *testing.T) {
	root := repoRoot(t)
	symbols, entries := loadConstructorCatalogInputs(t, root)
	expectedNonCodingAgent, err := expectedNonCodingAgentConstructors(symbols, entries)
	if err != nil {
		t.Fatal(err)
	}

	symbolsByID := make(map[string]surface.Symbol, len(symbols))
	for _, symbol := range symbols {
		symbolsByID[symbol.ID] = symbol
	}
	entriesByID := make(map[string][]catalog.Entry, len(entries))
	var constructorRows []catalog.Entry
	for _, entry := range entries {
		entriesByID[entry.ID] = append(entriesByID[entry.ID], entry)
		if strings.HasPrefix(entry.ID, "constructor:") {
			constructorRows = append(constructorRows, entry)
		}
	}

	constructibleByModule := map[string]int{}
	for _, symbol := range symbols {
		id := constructorID(symbol.ID)
		rows := entriesByID[id]
		if !symbol.Constructible {
			if len(rows) != 0 {
				t.Errorf("non-constructible surface symbol %s has %d constructor rows", symbol.ID, len(rows))
			}
			continue
		}

		constructibleByModule[symbol.Module]++
		if len(rows) != 1 {
			t.Errorf("constructible surface symbol %s has %d constructor rows, want exactly 1", symbol.ID, len(rows))
			continue
		}
		entry := rows[0]
		assertConstructorProvenance(t, symbol, entry)

		if symbol.Module != "codingagent" {
			want, ok := expectedNonCodingAgent[id]
			if !ok {
				t.Errorf("non-Coding-Agent constructor %s has no reviewed expected row", id)
				continue
			}
			if !reflect.DeepEqual(entry, want) {
				t.Errorf("constructor row %s differs\n got: %+v\nwant: %+v", id, entry, want)
			}
		} else {
			assertCodingAgentConstructorMetadata(t, entriesByID, symbol, entry)
		}

		for _, member := range symbol.StaticMembers {
			staticID := "static-member:" + strings.TrimPrefix(symbol.ID, "symbol:") + "." + member
			if staticID == id {
				t.Errorf("constructor and static-member identities collapsed for %s", symbol.ID)
			}
			if len(entriesByID[staticID]) != 1 {
				t.Errorf("static member %s has %d Catalog rows while constructor %s is cataloged", staticID, len(entriesByID[staticID]), id)
			}
		}
	}

	if len(constructorRows) != expectedConstructorCount {
		t.Errorf("constructor Catalog row count = %d, want %d", len(constructorRows), expectedConstructorCount)
	}
	if !reflect.DeepEqual(constructibleByModule, expectedConstructorCounts) {
		t.Errorf("constructible symbols by module = %v, want %v", constructibleByModule, expectedConstructorCounts)
	}
	for _, entry := range constructorRows {
		symbolID := symbolIDForConstructor(entry.ID)
		symbol, ok := symbolsByID[symbolID]
		if !ok {
			t.Errorf("orphan constructor Catalog row %s has no surface symbol %s", entry.ID, symbolID)
			continue
		}
		if !symbol.Constructible {
			t.Errorf("constructor Catalog row %s belongs to non-constructible symbol %s", entry.ID, symbol.ID)
		}
		if len(entriesByID[entry.ID]) != 1 {
			t.Errorf("constructor Catalog id %s occurs %d times", entry.ID, len(entriesByID[entry.ID]))
		}
	}
}

func TestReplaceNonCodingAgentConstructorRows(t *testing.T) {
	codingAgent := catalog.Entry{ID: "constructor:codingagent/src/example.ts#Example", Notes: "owned by issue #32"}
	staticMember := catalog.Entry{ID: "static-member:agent/src/example.ts#Example.create", Notes: "separate factory row"}
	ordinary := catalog.Entry{ID: "module-agent"}
	stale := catalog.Entry{ID: "constructor:agent/src/old.ts#Old"}
	wantConstructor := catalog.Entry{ID: "constructor:agent/src/new.ts#New"}

	got := replaceNonCodingAgentConstructorRows(
		[]catalog.Entry{stale, codingAgent, staticMember, ordinary},
		[]catalog.Entry{wantConstructor},
	)
	want := []catalog.Entry{codingAgent, staticMember, ordinary, wantConstructor}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("replacement result = %+v, want %+v", got, want)
	}
}

// TestGenerateConstructorCatalog replaces only non-Coding-Agent constructor
// rows. Coding Agent constructors remain owned by the issue #32 generator. It
// deliberately updates only the authoritative Catalog JSONL; the Catalog
// manifest and report remain the responsibility of their existing generator.
func TestGenerateConstructorCatalog(t *testing.T) {
	if !*updateConstructors {
		t.Skip("set -update-constructors to replace non-Coding-Agent constructor rows")
	}
	root := repoRoot(t)
	symbols, entries := loadConstructorCatalogInputs(t, root)
	expected, err := expectedNonCodingAgentConstructors(symbols, entries)
	if err != nil {
		t.Fatal(err)
	}
	generated := make([]catalog.Entry, 0, len(expected))
	for _, entry := range expected {
		generated = append(generated, entry)
	}
	sort.Slice(generated, func(i, j int) bool { return generated[i].ID < generated[j].ID })
	updated := replaceNonCodingAgentConstructorRows(entries, generated)
	data, err := catalog.EncodeEntries(updated)
	if err != nil {
		t.Fatalf("EncodeEntries: %v", err)
	}
	path := filepath.Join(root, "parity", "catalog.jsonl")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	t.Logf("replaced %d non-Coding-Agent constructor rows; preserved Coding Agent constructor rows", len(generated))
}

func loadConstructorCatalogInputs(t *testing.T, root string) ([]surface.Symbol, []catalog.Entry) {
	t.Helper()
	symbols, err := surface.LoadSymbols(filepath.Join(root, "parity", "surface", "symbols.jsonl"))
	if err != nil {
		t.Fatalf("LoadSymbols(real): %v", err)
	}
	entries, err := catalog.LoadCatalog(filepath.Join(root, "parity", "catalog.jsonl"))
	if err != nil {
		t.Fatalf("LoadCatalog(real): %v", err)
	}
	return symbols, entries
}

func expectedNonCodingAgentConstructors(symbols []surface.Symbol, entries []catalog.Entry) (map[string]catalog.Entry, error) {
	if len(nonCodingAgentConstructorMappings) != expectedConstructorCount-expectedConstructorCounts["codingagent"] {
		return nil, fmt.Errorf("reviewed non-Coding-Agent constructor mappings = %d, want 72", len(nonCodingAgentConstructorMappings))
	}

	symbolsByID := make(map[string]surface.Symbol, len(symbols))
	for _, symbol := range symbols {
		symbolsByID[symbol.ID] = symbol
	}
	entriesByID := make(map[string][]catalog.Entry, len(entries))
	for _, entry := range entries {
		entriesByID[entry.ID] = append(entriesByID[entry.ID], entry)
	}

	expected := make(map[string]catalog.Entry, len(nonCodingAgentConstructorMappings))
	for symbolID, spec := range nonCodingAgentConstructorMappings {
		symbol, ok := symbolsByID[symbolID]
		if !ok {
			return nil, fmt.Errorf("reviewed constructor mapping has no surface symbol: %s", symbolID)
		}
		if !symbol.Constructible {
			return nil, fmt.Errorf("reviewed constructor mapping belongs to non-constructible symbol: %s", symbolID)
		}
		if symbol.Module == "codingagent" || symbol.Module != spec.module {
			return nil, fmt.Errorf("reviewed constructor mapping %s module = %q, surface module = %q", symbolID, spec.module, symbol.Module)
		}

		metadataID := symbolID
		metadataRows := entriesByID[metadataID]
		if len(metadataRows) == 0 && spec.metadataFrom != "" {
			metadataID = spec.metadataFrom
			metadataRows = entriesByID[metadataID]
		}
		if len(metadataRows) != 1 {
			return nil, fmt.Errorf("constructor %s metadata source %s has %d Catalog rows, want 1", symbolID, metadataID, len(metadataRows))
		}
		metadata := metadataRows[0]
		if metadataID == symbolID {
			wantUpstream := catalog.Upstream(symbol.Upstream)
			if metadata.Upstream != wantUpstream {
				return nil, fmt.Errorf("symbol Catalog row %s provenance = %+v, want %+v", symbolID, metadata.Upstream, wantUpstream)
			}
		}
		if metadata.Mapping.Module != spec.module || metadata.Upstream.Module != symbol.Upstream.Module ||
			metadata.Upstream.Repository != symbol.Upstream.Repository || metadata.Upstream.Commit != symbol.Upstream.Commit {
			return nil, fmt.Errorf("constructor %s metadata source %s does not share module/baseline provenance", symbolID, metadataID)
		}
		if metadata.Milestone != spec.milestone {
			return nil, fmt.Errorf("constructor %s milestone source = %q, reviewed milestone = %q", symbolID, metadata.Milestone, spec.milestone)
		}
		if metadata.Classification != "public-api" {
			return nil, fmt.Errorf("constructor %s classification source = %q, want public-api", symbolID, metadata.Classification)
		}

		id := constructorID(symbol.ID)
		note := fmt.Sprintf("Global constructor audit maps the public %s constructor to callable Go factory %s; behavioral parity remains on its owning capability.", symbol.Name, spec.target)
		if spec.status == catalog.StatusInventoried {
			note = fmt.Sprintf("Global constructor audit inventories construction for public %s on carrier %s; no callable Go factory or constructor behavior is claimed.", symbol.Name, spec.target)
		}
		expected[id] = catalog.Entry{
			SchemaVersion: catalog.SchemaVersion,
			ID:            id,
			Upstream: catalog.Upstream{
				Module:     symbol.Upstream.Module,
				Repository: symbol.Upstream.Repository,
				Commit:     symbol.Upstream.Commit,
				Reference:  symbol.Upstream.Reference + ".constructor",
			},
			Mapping:        catalog.Mapping{Module: spec.module, Target: spec.target, Kind: "contract"},
			Status:         spec.status,
			Milestone:      metadata.Milestone,
			Classification: metadata.Classification,
			Notes:          note,
		}
	}

	for _, symbol := range symbols {
		if symbol.Module == "codingagent" || !symbol.Constructible {
			continue
		}
		if _, ok := nonCodingAgentConstructorMappings[symbol.ID]; !ok {
			return nil, fmt.Errorf("constructible non-Coding-Agent symbol has no reviewed constructor mapping: %s", symbol.ID)
		}
	}
	return expected, nil
}

func assertConstructorProvenance(t *testing.T, symbol surface.Symbol, entry catalog.Entry) {
	t.Helper()
	wantID := constructorID(symbol.ID)
	if entry.ID != wantID {
		t.Errorf("constructor id = %q, want %q", entry.ID, wantID)
	}
	wantUpstream := catalog.Upstream{
		Module:     symbol.Upstream.Module,
		Repository: symbol.Upstream.Repository,
		Commit:     symbol.Upstream.Commit,
		Reference:  symbol.Upstream.Reference + ".constructor",
	}
	if entry.Upstream != wantUpstream {
		t.Errorf("%s upstream = %+v, want exact symbol provenance %+v", entry.ID, entry.Upstream, wantUpstream)
	}
	if entry.SchemaVersion != catalog.SchemaVersion {
		t.Errorf("%s schema_version = %q, want %q", entry.ID, entry.SchemaVersion, catalog.SchemaVersion)
	}
	if entry.Mapping.Module != symbol.Module || entry.Mapping.Kind != "contract" || entry.Mapping.Target == "" {
		t.Errorf("%s mapping = %+v, want module %q, non-empty target, contract kind", entry.ID, entry.Mapping, symbol.Module)
	}
}

func assertCodingAgentConstructorMetadata(t *testing.T, entriesByID map[string][]catalog.Entry, symbol surface.Symbol, entry catalog.Entry) {
	t.Helper()
	parents := entriesByID[symbol.ID]
	if len(parents) != 1 {
		t.Errorf("Coding Agent constructor %s parent symbol row count = %d, want 1", entry.ID, len(parents))
		return
	}
	parent := parents[0]
	if entry.Milestone != parent.Milestone || entry.Classification != parent.Classification {
		t.Errorf("%s milestone/classification = %s/%s, want parent %s/%s", entry.ID, entry.Milestone, entry.Classification, parent.Milestone, parent.Classification)
	}
	switch entry.Status {
	case catalog.StatusScaffolded:
		if len(entry.Evidence) == 0 {
			t.Errorf("scaffolded Coding Agent constructor %s has no target-resolution evidence", entry.ID)
		}
	case catalog.StatusInventoried:
		if len(entry.Evidence) != 0 {
			t.Errorf("inventoried Coding Agent constructor %s claims behavioral evidence", entry.ID)
		}
	default:
		t.Errorf("Coding Agent constructor %s status = %q, want scaffolded or inventoried", entry.ID, entry.Status)
	}
}

func replaceNonCodingAgentConstructorRows(entries, replacements []catalog.Entry) []catalog.Entry {
	updated := make([]catalog.Entry, 0, len(entries)+len(replacements))
	for _, entry := range entries {
		if strings.HasPrefix(entry.ID, "constructor:") && !strings.HasPrefix(entry.ID, "constructor:codingagent/") {
			continue
		}
		updated = append(updated, entry)
	}
	return append(updated, replacements...)
}

func constructorID(symbolID string) string {
	return "constructor:" + strings.TrimPrefix(symbolID, "symbol:")
}

func symbolIDForConstructor(constructor string) string {
	return "symbol:" + strings.TrimPrefix(constructor, "constructor:")
}
