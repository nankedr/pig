package agent_test

import (
	"context"
	"crypto/sha256"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/internal/catalog"
	"github.com/nankedr/pig/internal/surface"
)

var updateIssue29Surface = flag.Bool("update-issue29-surface", false, "regenerate the issue #29 Go API snapshot")

func TestIssue29MappingsMatchLockedLegacyAgentSurface(t *testing.T) {
	root := issue29RepoRoot(t)
	symbols, err := surface.LoadSymbols(filepath.Join(root, "parity", "surface", "symbols.jsonl"))
	if err != nil {
		t.Fatalf("LoadSymbols: %v", err)
	}
	want := make(map[string]surface.Symbol)
	for _, symbol := range symbols {
		if issue29LegacyAgentSymbol(symbol) {
			want[symbol.ID] = symbol
		}
	}
	if len(want) != 32 {
		t.Fatalf("locked legacy Agent surface count = %d, want 32", len(want))
	}

	entries := issue29CatalogSymbolEntries(t, root)
	if len(entries) != len(want) {
		t.Fatalf("catalog issue29 symbol row count = %d, want %d", len(entries), len(want))
	}

	got := make(map[string]catalog.Entry, len(entries))
	for _, entry := range entries {
		if entry.Mapping.Module != "agent" || entry.Mapping.Kind != "symbol" {
			t.Errorf("%s has mapping (%s, %s), want (agent, symbol)", entry.ID, entry.Mapping.Module, entry.Mapping.Kind)
			continue
		}
		symbol, ok := want[entry.ID]
		if !ok {
			t.Errorf("catalog mapping is outside locked legacy Agent surface: %s", entry.ID)
			continue
		}
		if entry.Upstream.Reference != symbol.Upstream.Reference {
			t.Errorf("%s upstream reference = %q, want %q", entry.ID, entry.Upstream.Reference, symbol.Upstream.Reference)
		}
		if _, ok := issue29SnapshotResolver(entry.Mapping.Target); !ok {
			t.Errorf("%s maps to unsupported Go target %q", entry.ID, entry.Mapping.Target)
		}
		switch entry.Status {
		case catalog.StatusScaffolded, catalog.StatusPartial, catalog.StatusImplemented, catalog.StatusVerified:
		default:
			t.Errorf("%s has inactive Capability Status %q", entry.ID, entry.Status)
		}
		got[entry.ID] = entry
	}
	for id := range want {
		if _, ok := got[id]; !ok {
			t.Errorf("locked legacy Agent symbol has no Catalog symbol mapping: %s", id)
		}
	}
}

func TestIssue29MemberMappingsMatchLockedLegacyAgentSurface(t *testing.T) {
	root := issue29RepoRoot(t)
	surfacePath := filepath.Join(root, "parity", "surface", "symbols.jsonl")
	surfaceJSONL, err := os.ReadFile(surfacePath)
	if err != nil {
		t.Fatalf("read locked surface: %v", err)
	}
	if got := fmt.Sprintf("sha256:%x", sha256.Sum256(surfaceJSONL)); got != issue29SurfaceHash {
		t.Fatalf("locked surface hash = %s, want %s", got, issue29SurfaceHash)
	}
	symbols, err := surface.LoadSymbols(surfacePath)
	if err != nil {
		t.Fatalf("LoadSymbols: %v", err)
	}
	want := make(map[string]issue29ExpectedMember)
	coveredSymbols := make(map[string]struct{})
	var scaffoldedCount, languageInheritedCount int
	for _, symbol := range symbols {
		if !issue29LegacyAgentSymbol(symbol) || len(symbol.Members) == 0 {
			continue
		}
		targets, ok := issue29MemberTargets[symbol.ID]
		if !ok {
			t.Errorf("locked issue #29 symbol with members has no expected mapping table: %s", symbol.ID)
			continue
		}
		coveredSymbols[symbol.ID] = struct{}{}
		if targets.languageInherited {
			if err := issue29LanguageInheritedMembersError(symbol.Members); err != nil {
				t.Errorf("%s: %v", symbol.ID, err)
			}
		} else if len(targets.members) != len(symbol.Members) {
			t.Errorf("%s target mapping count = %d, locked member count = %d", symbol.ID, len(targets.members), len(symbol.Members))
		}
		for _, member := range symbol.Members {
			target := targets.parentTarget
			status := catalog.StatusInventoried
			if !targets.languageInherited {
				var exists bool
				target, exists = targets.members[member]
				if !exists {
					t.Errorf("locked %s member has no expected Go mapping: %s", symbol.ID, member)
					continue
				}
				status = catalog.StatusScaffolded
				scaffoldedCount++
			} else {
				languageInheritedCount++
			}
			id := "member:" + strings.TrimPrefix(symbol.ID, "symbol:") + "." + member
			want[id] = issue29ExpectedMember{symbol: symbol.Name, member: member, target: target, status: status, broaderContract: targets.broaderContract}
		}
	}
	if len(want) != 279 || scaffoldedCount != 132 || languageInheritedCount != 147 {
		t.Fatalf("locked issue #29 member counts = total %d, scaffolded %d, language-inherited %d; want 279, 132, 147", len(want), scaffoldedCount, languageInheritedCount)
	}
	for symbolID := range issue29MemberTargets {
		if _, ok := coveredSymbols[symbolID]; !ok {
			t.Errorf("expected member mapping table is outside non-empty locked issue #29 surface: %s", symbolID)
		}
	}
	ids := make([]string, 0, len(want))
	for id := range want {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		member := want[id]
		if member.status != catalog.StatusScaffolded {
			continue
		}
		for _, target := range strings.Split(member.target, " / ") {
			if err := issue29MemberTargetError(target); err != nil {
				t.Errorf("%s: %v", id, err)
			}
		}
	}
	issue29ValidateEventVariants(t)

	entries := issue29CatalogMemberEntries(t, root)
	if len(entries) != len(want) {
		t.Fatalf("catalog issue #29 member row count = %d, want %d", len(entries), len(want))
	}
	got := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		wantMember, ok := want[entry.ID]
		if !ok {
			t.Errorf("unexpected issue #29 member mapping: %s", entry.ID)
			continue
		}
		if _, duplicate := got[entry.ID]; duplicate {
			t.Errorf("duplicate issue #29 member mapping: %s", entry.ID)
			continue
		}
		wantReference := "packages/" + strings.TrimPrefix(entry.ID, "member:")
		if entry.Upstream.Reference != wantReference {
			t.Errorf("%s upstream reference = %q, want %q", entry.ID, entry.Upstream.Reference, wantReference)
		}
		if entry.SchemaVersion != catalog.SchemaVersion || entry.Upstream.Module != "agent" || entry.Upstream.Repository != "https://github.com/badlogic/pi-mono" || entry.Upstream.Commit != issue29BaselineCommit {
			t.Errorf("%s has unexpected catalog provenance: schema=%q module=%q repository=%q commit=%q", entry.ID, entry.SchemaVersion, entry.Upstream.Module, entry.Upstream.Repository, entry.Upstream.Commit)
		}
		if entry.Mapping.Module != "agent" || entry.Mapping.Kind != "contract" {
			t.Errorf("%s has mapping (%s, %s), want (agent, contract)", entry.ID, entry.Mapping.Module, entry.Mapping.Kind)
		}
		if entry.Mapping.Target != wantMember.target {
			t.Errorf("%s target = %q, want %q", entry.ID, entry.Mapping.Target, wantMember.target)
		}
		if entry.Status != wantMember.status {
			t.Errorf("%s status = %q, want %q", entry.ID, entry.Status, wantMember.status)
		}
		if entry.Milestone != issue29MemberMilestone(entry.ID) {
			t.Errorf("%s milestone = %q, want %q", entry.ID, entry.Milestone, issue29MemberMilestone(entry.ID))
		}
		if entry.Classification != "public-api" {
			t.Errorf("%s classification = %q, want public-api", entry.ID, entry.Classification)
		}
		if wantMember.status == catalog.StatusScaffolded {
			issue29ValidateScaffoldedMemberEvidence(t, entry, wantMember)
		} else {
			issue29ValidateLanguageInheritedMember(t, entry, wantMember)
		}
		got[entry.ID] = struct{}{}
	}
	for id := range want {
		if _, ok := got[id]; !ok {
			t.Errorf("locked issue #29 member has no Catalog mapping: %s", id)
		}
	}
}

type issue29ExpectedMember struct {
	symbol          string
	member          string
	target          string
	status          string
	broaderContract string
}

type issue29MemberMappings struct {
	members           map[string]string
	parentTarget      string
	broaderContract   string
	languageInherited bool
}

const (
	issue29BaselineCommit = "936aff00918de1187f085f123c2812d8f2d67745"
	issue29SurfaceHash    = "sha256:353816fada23e5469f2357d8a5e1b034481b0ada916e8ea129773f934a42c689"
	issue29MemberTestRef  = "agent/issue29_surface_test.go#TestIssue29MemberMappingsMatchLockedLegacyAgentSurface"
	issue29MemberTestRun  = "go test ./agent -run '^TestIssue29MemberMappingsMatchLockedLegacyAgentSurface$' -count=1"
)

func issue29MemberMilestone(id string) string {
	if strings.HasPrefix(id, "member:agent/src/proxy.ts#") {
		return "M2"
	}
	return "M1"
}

func issue29ValidateScaffoldedMemberEvidence(t *testing.T, entry catalog.Entry, member issue29ExpectedMember) {
	t.Helper()
	if len(entry.Evidence) != 1 {
		t.Errorf("%s evidence count = %d, want 1", entry.ID, len(entry.Evidence))
		return
	}
	want := catalog.Evidence{
		Kind:            "go-test",
		Ref:             issue29MemberTestRef,
		Baseline:        issue29BaselineCommit,
		CaseID:          entry.ID,
		InputHash:       issue29SurfaceHash,
		ExecutionMethod: issue29MemberTestRun,
		Expected:        fmt.Sprintf("the pinned Pi %s.%s member maps one-to-one to the declared scaffolded Go contract target", member.symbol, member.member),
		Actual:          fmt.Sprintf("PASS; %s resolved to %s", entry.ID, member.target),
		Platform:        "any",
		CatalogID:       entry.ID,
	}
	if entry.Evidence[0] != want {
		t.Errorf("%s evidence = %+v, want %+v", entry.ID, entry.Evidence[0], want)
	}

	wantNote := fmt.Sprintf("Issue #29 concrete %s member mapping authority. Behavioral status remains on the broader %s row.", member.symbol, member.broaderContract)
	if member.symbol == "Agent" || member.symbol == "AgentOptions" {
		wantNote = fmt.Sprintf("Issue #29 concrete %s member mapping authority. Behavioral status remains on %s.", member.symbol, member.broaderContract)
	}
	if member.symbol == "CustomAgentMessages" {
		wantNote = "Issue #29 concrete CustomAgentMessages declaration-merge key mapping authority for the open AgentMessage seam. The key maps to AgentMessage rather than a closed Go field; behavioral status remains on the broader contract:agent/messages row."
	}
	if entry.Notes != wantNote {
		t.Errorf("%s notes = %q, want %q", entry.ID, entry.Notes, wantNote)
	}
}

func issue29ValidateLanguageInheritedMember(t *testing.T, entry catalog.Entry, member issue29ExpectedMember) {
	t.Helper()
	if len(entry.Evidence) != 0 {
		t.Errorf("%s has %d evidence rows, want none while inventoried", entry.ID, len(entry.Evidence))
	}
	wantNote := fmt.Sprintf("Issue #29 records %s.%s as a TypeScript default-library inherited String member. It maps to the parent Go type; no separate Go method is required. Behavioral status remains on the broader %s row.", member.symbol, member.member, member.broaderContract)
	if entry.Notes != wantNote {
		t.Errorf("%s notes = %q, want %q", entry.ID, entry.Notes, wantNote)
	}
	targetType, ok := issue29StringBackedTargets[member.target]
	if !ok {
		t.Errorf("%s maps inherited String member to unsupported parent target %q", entry.ID, member.target)
		return
	}
	if targetType.Kind() != reflect.String {
		t.Errorf("%s parent target %q has kind %s, want string", entry.ID, member.target, targetType.Kind())
	}
}

func issue29LanguageInheritedMembersError(got []string) error {
	if reflect.DeepEqual(got, issue29StringInheritedMembers) {
		return nil
	}
	return fmt.Errorf("language-inherited members = %q, want shared locked list %q", got, issue29StringInheritedMembers)
}

var issue29StringInheritedMembers = []string{
	"anchor", "at", "big", "blink", "bold", "charAt", "charCodeAt", "codePointAt", "concat",
	"endsWith", "fixed", "fontcolor", "fontsize", "includes", "indexOf", "italics",
	"lastIndexOf", "length", "link", "localeCompare", "match", "matchAll", "normalize",
	"padEnd", "padStart", "repeat", "replace", "replaceAll", "search", "slice", "small",
	"split", "startsWith", "strike", "sub", "substr", "substring", "sup",
	"toLocaleLowerCase", "toLocaleUpperCase", "toLowerCase", "toString", "toUpperCase",
	"trim", "trimEnd", "trimLeft", "trimRight", "trimStart", "valueOf",
}

var issue29StringBackedTargets = map[string]reflect.Type{
	"github.com/nankedr/pig/agent.QueueMode":         reflect.TypeOf(agent.QueueMode("")),
	"github.com/nankedr/pig/agent.ThinkingLevel":     reflect.TypeOf(agent.ThinkingLevel("")),
	"github.com/nankedr/pig/agent.ToolExecutionMode": reflect.TypeOf(agent.ToolExecutionMode("")),
}

type issue29MemberRootKind uint8

const (
	issue29StructFields issue29MemberRootKind = iota
	issue29PointerMethods
	issue29InterfaceMethods
	issue29OpenSeam
)

type issue29MemberRoot struct {
	typ  reflect.Type
	kind issue29MemberRootKind
}

var issue29MemberRoots = map[string]issue29MemberRoot{
	"github.com/nankedr/pig/agent.(*Agent)":                   {reflect.TypeOf((*agent.Agent)(nil)), issue29PointerMethods},
	"github.com/nankedr/pig/agent.AfterToolCallContext":       {reflect.TypeOf(agent.AfterToolCallContext{}), issue29StructFields},
	"github.com/nankedr/pig/agent.AfterToolCallResult":        {reflect.TypeOf(agent.AfterToolCallResult{}), issue29StructFields},
	"github.com/nankedr/pig/agent.AgentContext":               {reflect.TypeOf(agent.AgentContext{}), issue29StructFields},
	"github.com/nankedr/pig/agent.AgentEvent":                 {reflect.TypeOf((*agent.AgentEvent)(nil)).Elem(), issue29InterfaceMethods},
	"github.com/nankedr/pig/agent.AgentLoopConfig":            {reflect.TypeOf(agent.AgentLoopConfig{}), issue29StructFields},
	"github.com/nankedr/pig/agent.AgentLoopTurnUpdate":        {reflect.TypeOf(agent.AgentLoopTurnUpdate{}), issue29StructFields},
	"github.com/nankedr/pig/agent.AgentMessage":               {reflect.TypeOf((*agent.AgentMessage)(nil)).Elem(), issue29OpenSeam},
	"github.com/nankedr/pig/agent.AgentOptions":               {reflect.TypeOf(agent.AgentOptions{}), issue29StructFields},
	"github.com/nankedr/pig/agent.AgentState":                 {reflect.TypeOf(agent.AgentState{}), issue29StructFields},
	"github.com/nankedr/pig/agent.AgentTool":                  {reflect.TypeOf(agent.AgentTool[map[string]any, map[string]any]{}), issue29StructFields},
	"github.com/nankedr/pig/agent.AgentToolResult":            {reflect.TypeOf(agent.AgentToolResult[map[string]any]{}), issue29StructFields},
	"github.com/nankedr/pig/agent.BeforeToolCallContext":      {reflect.TypeOf(agent.BeforeToolCallContext{}), issue29StructFields},
	"github.com/nankedr/pig/agent.BeforeToolCallResult":       {reflect.TypeOf(agent.BeforeToolCallResult{}), issue29StructFields},
	"github.com/nankedr/pig/agent.PrepareNextTurnContext":     {reflect.TypeOf(agent.PrepareNextTurnContext{}), issue29StructFields},
	"github.com/nankedr/pig/agent.ProxyAssistantMessageEvent": {reflect.TypeOf((*agent.ProxyAssistantMessageEvent)(nil)).Elem(), issue29InterfaceMethods},
	"github.com/nankedr/pig/agent.ProxyStreamOptions":         {reflect.TypeOf(agent.ProxyStreamOptions{}), issue29StructFields},
	"github.com/nankedr/pig/agent.ShouldStopAfterTurnContext": {reflect.TypeOf(agent.ShouldStopAfterTurnContext{}), issue29StructFields},
}

func issue29MemberTargetError(target string) error {
	if target == "" || strings.TrimSpace(target) != target {
		return fmt.Errorf("malformed issue #29 member target component %q", target)
	}
	if root, ok := issue29MemberRoots[target]; ok {
		if root.kind != issue29OpenSeam {
			return fmt.Errorf("bare issue #29 member target %q is not an open seam", target)
		}
		return issue29AgentMessageSeamError(root.typ)
	}

	for rootTarget, root := range issue29MemberRoots {
		prefix := rootTarget + "."
		if !strings.HasPrefix(target, prefix) {
			continue
		}
		name := strings.TrimPrefix(target, prefix)
		if name == "" || strings.Contains(name, ".") {
			return fmt.Errorf("malformed member name in Go target %q", target)
		}
		switch root.kind {
		case issue29PointerMethods, issue29InterfaceMethods:
			method, ok := root.typ.MethodByName(name)
			if !ok {
				return fmt.Errorf("Go target method %q does not exist", target)
			}
			if method.PkgPath != "" {
				return fmt.Errorf("Go target method %q is not exported", target)
			}
			if target == "github.com/nankedr/pig/agent.AgentEvent.AgentEventType" {
				return issue29InterfaceMethodSignatureError(method, reflect.TypeOf(agent.AgentEventType("")))
			}
			if target == "github.com/nankedr/pig/agent.ProxyAssistantMessageEvent.ProxyAssistantMessageEventType" {
				return issue29InterfaceMethodSignatureError(method, reflect.TypeOf(ai.AssistantMessageEventType("")))
			}
		case issue29StructFields:
			field, ok := root.typ.FieldByName(name)
			if !ok {
				return fmt.Errorf("Go target field %q does not exist", target)
			}
			if field.PkgPath != "" {
				return fmt.Errorf("Go target field %q is not exported", target)
			}
			if len(field.Index) != 1 {
				return fmt.Errorf("Go target field %q is not declared directly", target)
			}
		case issue29OpenSeam:
			return fmt.Errorf("open-seam target %q must not name a fictitious member", target)
		default:
			return fmt.Errorf("unsupported issue #29 member target kind for %q", target)
		}
		return nil
	}
	return fmt.Errorf("unsupported issue #29 member target component %q", target)
}

func issue29InterfaceMethodSignatureError(method reflect.Method, result reflect.Type) error {
	if method.Type.NumIn() != 0 || method.Type.NumOut() != 1 || method.Type.Out(0) != result {
		return fmt.Errorf("interface method %s has signature %s, want func() %s", method.Name, method.Type, result)
	}
	return nil
}

func issue29AgentMessageSeamError(t reflect.Type) error {
	if t.Kind() != reflect.Interface {
		return fmt.Errorf("AgentMessage open seam kind = %s, want interface", t.Kind())
	}
	method, ok := t.MethodByName("MessageRole")
	if !ok || method.PkgPath != "" {
		return fmt.Errorf("AgentMessage open seam lacks exported MessageRole")
	}
	wantResult := reflect.TypeOf(ai.MessageRole(""))
	if method.Type.NumIn() != 0 || method.Type.NumOut() != 1 || method.Type.Out(0) != wantResult {
		return fmt.Errorf("AgentMessage.MessageRole has signature %s, want func() ai.MessageRole", method.Type)
	}
	return nil
}

var issue29MemberTargets = map[string]issue29MemberMappings{
	"symbol:agent/src/agent.ts#Agent": {
		broaderContract: "contract:agent/state-and-queues",
		members: map[string]string{
			"abort":                      "github.com/nankedr/pig/agent.(*Agent).Abort",
			"afterToolCall":              "github.com/nankedr/pig/agent.(*Agent).AfterToolCall / github.com/nankedr/pig/agent.(*Agent).SetAfterToolCall",
			"beforeToolCall":             "github.com/nankedr/pig/agent.(*Agent).BeforeToolCall / github.com/nankedr/pig/agent.(*Agent).SetBeforeToolCall",
			"clearAllQueues":             "github.com/nankedr/pig/agent.(*Agent).ClearAllQueues",
			"clearFollowUpQueue":         "github.com/nankedr/pig/agent.(*Agent).ClearFollowUpQueue",
			"clearSteeringQueue":         "github.com/nankedr/pig/agent.(*Agent).ClearSteeringQueue",
			"continue":                   "github.com/nankedr/pig/agent.(*Agent).Continue",
			"convertToLlm":               "github.com/nankedr/pig/agent.(*Agent).ConvertToLLM / github.com/nankedr/pig/agent.(*Agent).SetConvertToLLM",
			"followUp":                   "github.com/nankedr/pig/agent.(*Agent).FollowUp",
			"followUpMode":               "github.com/nankedr/pig/agent.(*Agent).FollowUpMode / github.com/nankedr/pig/agent.(*Agent).SetFollowUpMode",
			"getApiKey":                  "github.com/nankedr/pig/agent.(*Agent).GetAPIKey / github.com/nankedr/pig/agent.(*Agent).SetGetAPIKey",
			"hasQueuedMessages":          "github.com/nankedr/pig/agent.(*Agent).HasQueuedMessages",
			"maxRetryDelayMs":            "github.com/nankedr/pig/agent.(*Agent).MaxRetryDelayMS / github.com/nankedr/pig/agent.(*Agent).SetMaxRetryDelayMS",
			"onPayload":                  "github.com/nankedr/pig/agent.(*Agent).OnPayload / github.com/nankedr/pig/agent.(*Agent).SetOnPayload",
			"onResponse":                 "github.com/nankedr/pig/agent.(*Agent).OnResponse / github.com/nankedr/pig/agent.(*Agent).SetOnResponse",
			"prepareNextTurn":            "github.com/nankedr/pig/agent.(*Agent).PrepareNextTurn / github.com/nankedr/pig/agent.(*Agent).SetPrepareNextTurn",
			"prepareNextTurnWithContext": "github.com/nankedr/pig/agent.(*Agent).PrepareNextTurnWithContext / github.com/nankedr/pig/agent.(*Agent).SetPrepareNextTurnWithContext",
			"prompt":                     "github.com/nankedr/pig/agent.(*Agent).Prompt / github.com/nankedr/pig/agent.(*Agent).PromptText",
			"reset":                      "github.com/nankedr/pig/agent.(*Agent).Reset",
			"sessionId":                  "github.com/nankedr/pig/agent.(*Agent).SessionID / github.com/nankedr/pig/agent.(*Agent).SetSessionID",
			"shouldStopAfterTurn":        "github.com/nankedr/pig/agent.(*Agent).ShouldStopAfterTurn / github.com/nankedr/pig/agent.(*Agent).SetShouldStopAfterTurn",
			"signal":                     "github.com/nankedr/pig/agent.(*Agent).ActiveContext",
			"state":                      "github.com/nankedr/pig/agent.(*Agent).State",
			"steer":                      "github.com/nankedr/pig/agent.(*Agent).Steer",
			"steeringMode":               "github.com/nankedr/pig/agent.(*Agent).SteeringMode / github.com/nankedr/pig/agent.(*Agent).SetSteeringMode",
			"streamFunction":             "github.com/nankedr/pig/agent.(*Agent).StreamFunction / github.com/nankedr/pig/agent.(*Agent).SetStreamFunction",
			"subscribe":                  "github.com/nankedr/pig/agent.(*Agent).Subscribe",
			"thinkingBudgets":            "github.com/nankedr/pig/agent.(*Agent).ThinkingBudgets / github.com/nankedr/pig/agent.(*Agent).SetThinkingBudgets",
			"toolExecution":              "github.com/nankedr/pig/agent.(*Agent).ToolExecution / github.com/nankedr/pig/agent.(*Agent).SetToolExecution",
			"transformContext":           "github.com/nankedr/pig/agent.(*Agent).TransformContext / github.com/nankedr/pig/agent.(*Agent).SetTransformContext",
			"transport":                  "github.com/nankedr/pig/agent.(*Agent).Transport / github.com/nankedr/pig/agent.(*Agent).SetTransport",
			"waitForIdle":                "github.com/nankedr/pig/agent.(*Agent).WaitForIdle",
		},
	},
	"symbol:agent/src/agent.ts#AgentOptions": {
		broaderContract: "contract:agent/state-and-queues",
		members: map[string]string{
			"afterToolCall":              "github.com/nankedr/pig/agent.AgentOptions.AfterToolCall",
			"beforeToolCall":             "github.com/nankedr/pig/agent.AgentOptions.BeforeToolCall",
			"convertToLlm":               "github.com/nankedr/pig/agent.AgentOptions.ConvertToLLM",
			"followUpMode":               "github.com/nankedr/pig/agent.AgentOptions.FollowUpMode",
			"getApiKey":                  "github.com/nankedr/pig/agent.AgentOptions.GetAPIKey",
			"initialState":               "github.com/nankedr/pig/agent.AgentOptions.InitialState",
			"maxRetryDelayMs":            "github.com/nankedr/pig/agent.AgentOptions.MaxRetryDelayMS",
			"onPayload":                  "github.com/nankedr/pig/agent.AgentOptions.OnPayload",
			"onResponse":                 "github.com/nankedr/pig/agent.AgentOptions.OnResponse",
			"prepareNextTurn":            "github.com/nankedr/pig/agent.AgentOptions.PrepareNextTurn",
			"prepareNextTurnWithContext": "github.com/nankedr/pig/agent.AgentOptions.PrepareNextTurnWithContext",
			"sessionId":                  "github.com/nankedr/pig/agent.AgentOptions.SessionID",
			"shouldStopAfterTurn":        "github.com/nankedr/pig/agent.AgentOptions.ShouldStopAfterTurn",
			"steeringMode":               "github.com/nankedr/pig/agent.AgentOptions.SteeringMode",
			"streamFn":                   "github.com/nankedr/pig/agent.AgentOptions.StreamFunction",
			"thinkingBudgets":            "github.com/nankedr/pig/agent.AgentOptions.ThinkingBudgets",
			"toolExecution":              "github.com/nankedr/pig/agent.AgentOptions.ToolExecution",
			"transformContext":           "github.com/nankedr/pig/agent.AgentOptions.TransformContext",
			"transport":                  "github.com/nankedr/pig/agent.AgentOptions.Transport",
		},
	},
	"symbol:agent/src/proxy.ts#ProxyAssistantMessageEvent": {
		broaderContract: "contract:agent/proxy",
		members: map[string]string{
			"type": "github.com/nankedr/pig/agent.ProxyAssistantMessageEvent.ProxyAssistantMessageEventType",
		},
	},
	"symbol:agent/src/proxy.ts#ProxyStreamOptions": {
		broaderContract: "contract:agent/proxy",
		members: map[string]string{
			"authToken":       "github.com/nankedr/pig/agent.ProxyStreamOptions.AuthToken",
			"cacheRetention":  "github.com/nankedr/pig/agent.ProxyStreamOptions.CacheRetention",
			"headers":         "github.com/nankedr/pig/agent.ProxyStreamOptions.Headers",
			"maxRetryDelayMs": "github.com/nankedr/pig/agent.ProxyStreamOptions.MaxRetryDelayMS",
			"maxTokens":       "github.com/nankedr/pig/agent.ProxyStreamOptions.MaxTokens",
			"metadata":        "github.com/nankedr/pig/agent.ProxyStreamOptions.Metadata",
			"proxyUrl":        "github.com/nankedr/pig/agent.ProxyStreamOptions.ProxyURL",
			"reasoning":       "github.com/nankedr/pig/agent.ProxyStreamOptions.Reasoning",
			"samplingParams":  "github.com/nankedr/pig/agent.ProxyStreamOptions.SamplingParams",
			"sessionId":       "github.com/nankedr/pig/agent.ProxyStreamOptions.SessionID",
			"signal":          "github.com/nankedr/pig/agent.ProxyStreamOptions.Signal",
			"temperature":     "github.com/nankedr/pig/agent.ProxyStreamOptions.Temperature",
			"thinkingBudgets": "github.com/nankedr/pig/agent.ProxyStreamOptions.ThinkingBudgets",
			"transport":       "github.com/nankedr/pig/agent.ProxyStreamOptions.Transport",
		},
	},
	"symbol:agent/src/types.ts#AfterToolCallContext": {
		broaderContract: "contract:agent/tools",
		members: map[string]string{
			"args":             "github.com/nankedr/pig/agent.AfterToolCallContext.Args",
			"assistantMessage": "github.com/nankedr/pig/agent.AfterToolCallContext.AssistantMessage",
			"context":          "github.com/nankedr/pig/agent.AfterToolCallContext.Context",
			"isError":          "github.com/nankedr/pig/agent.AfterToolCallContext.IsError",
			"result":           "github.com/nankedr/pig/agent.AfterToolCallContext.Result",
			"toolCall":         "github.com/nankedr/pig/agent.AfterToolCallContext.ToolCall",
		},
	},
	"symbol:agent/src/types.ts#AfterToolCallResult": {
		broaderContract: "contract:agent/tools",
		members: map[string]string{
			"content":   "github.com/nankedr/pig/agent.AfterToolCallResult.Content",
			"details":   "github.com/nankedr/pig/agent.AfterToolCallResult.Details",
			"isError":   "github.com/nankedr/pig/agent.AfterToolCallResult.IsError",
			"terminate": "github.com/nankedr/pig/agent.AfterToolCallResult.Terminate",
			"usage":     "github.com/nankedr/pig/agent.AfterToolCallResult.Usage",
		},
	},
	"symbol:agent/src/types.ts#AgentContext": {
		broaderContract: "contract:agent/runtime",
		members: map[string]string{
			"messages":     "github.com/nankedr/pig/agent.AgentContext.Messages",
			"systemPrompt": "github.com/nankedr/pig/agent.AgentContext.SystemPrompt",
			"tools":        "github.com/nankedr/pig/agent.AgentContext.Tools",
		},
	},
	"symbol:agent/src/types.ts#AgentEvent": {
		broaderContract: "contract:agent/events",
		members: map[string]string{
			"type": "github.com/nankedr/pig/agent.AgentEvent.AgentEventType",
		},
	},
	"symbol:agent/src/types.ts#AgentLoopConfig": {
		broaderContract: "contract:agent/runtime",
		members: map[string]string{
			"afterToolCall":       "github.com/nankedr/pig/agent.AgentLoopConfig.AfterToolCall",
			"beforeToolCall":      "github.com/nankedr/pig/agent.AgentLoopConfig.BeforeToolCall",
			"convertToLlm":        "github.com/nankedr/pig/agent.AgentLoopConfig.ConvertToLLM",
			"getApiKey":           "github.com/nankedr/pig/agent.AgentLoopConfig.GetAPIKey",
			"getFollowUpMessages": "github.com/nankedr/pig/agent.AgentLoopConfig.GetFollowUpMessages",
			"getSteeringMessages": "github.com/nankedr/pig/agent.AgentLoopConfig.GetSteeringMessages",
			"model":               "github.com/nankedr/pig/agent.AgentLoopConfig.Model",
			"prepareNextTurn":     "github.com/nankedr/pig/agent.AgentLoopConfig.PrepareNextTurn",
			"shouldStopAfterTurn": "github.com/nankedr/pig/agent.AgentLoopConfig.ShouldStopAfterTurn",
			"toolExecution":       "github.com/nankedr/pig/agent.AgentLoopConfig.ToolExecution",
			"transformContext":    "github.com/nankedr/pig/agent.AgentLoopConfig.TransformContext",
		},
	},
	"symbol:agent/src/types.ts#AgentLoopTurnUpdate": {
		broaderContract: "contract:agent/runtime",
		members: map[string]string{
			"context":       "github.com/nankedr/pig/agent.AgentLoopTurnUpdate.Context",
			"model":         "github.com/nankedr/pig/agent.AgentLoopTurnUpdate.Model",
			"thinkingLevel": "github.com/nankedr/pig/agent.AgentLoopTurnUpdate.ThinkingLevel",
		},
	},
	"symbol:agent/src/types.ts#AgentState": {
		broaderContract: "contract:agent/state-and-queues",
		members: map[string]string{
			"errorMessage":     "github.com/nankedr/pig/agent.AgentState.ErrorMessage",
			"isStreaming":      "github.com/nankedr/pig/agent.AgentState.IsStreaming",
			"messages":         "github.com/nankedr/pig/agent.AgentState.Messages",
			"model":            "github.com/nankedr/pig/agent.AgentState.Model",
			"pendingToolCalls": "github.com/nankedr/pig/agent.AgentState.PendingToolCalls",
			"streamingMessage": "github.com/nankedr/pig/agent.AgentState.StreamingMessage",
			"systemPrompt":     "github.com/nankedr/pig/agent.AgentState.SystemPrompt",
			"thinkingLevel":    "github.com/nankedr/pig/agent.AgentState.ThinkingLevel",
			"tools":            "github.com/nankedr/pig/agent.AgentState.Tools",
		},
	},
	"symbol:agent/src/types.ts#AgentTool": {
		broaderContract: "contract:agent/tools",
		members: map[string]string{
			"execute":          "github.com/nankedr/pig/agent.AgentTool.Execute",
			"executionMode":    "github.com/nankedr/pig/agent.AgentTool.ExecutionMode",
			"label":            "github.com/nankedr/pig/agent.AgentTool.Label",
			"prepareArguments": "github.com/nankedr/pig/agent.AgentTool.PrepareArguments",
		},
	},
	"symbol:agent/src/types.ts#AgentToolResult": {
		broaderContract: "contract:agent/tools",
		members: map[string]string{
			"addedToolNames": "github.com/nankedr/pig/agent.AgentToolResult.AddedToolNames",
			"content":        "github.com/nankedr/pig/agent.AgentToolResult.Content",
			"details":        "github.com/nankedr/pig/agent.AgentToolResult.Details",
			"terminate":      "github.com/nankedr/pig/agent.AgentToolResult.Terminate",
			"usage":          "github.com/nankedr/pig/agent.AgentToolResult.Usage",
		},
	},
	"symbol:agent/src/types.ts#BeforeToolCallContext": {
		broaderContract: "contract:agent/tools",
		members: map[string]string{
			"args":             "github.com/nankedr/pig/agent.BeforeToolCallContext.Args",
			"assistantMessage": "github.com/nankedr/pig/agent.BeforeToolCallContext.AssistantMessage",
			"context":          "github.com/nankedr/pig/agent.BeforeToolCallContext.Context",
			"toolCall":         "github.com/nankedr/pig/agent.BeforeToolCallContext.ToolCall",
		},
	},
	"symbol:agent/src/types.ts#BeforeToolCallResult": {
		broaderContract: "contract:agent/tools",
		members: map[string]string{
			"block":     "github.com/nankedr/pig/agent.BeforeToolCallResult.Block",
			"reason":    "github.com/nankedr/pig/agent.BeforeToolCallResult.Reason",
			"terminate": "github.com/nankedr/pig/agent.BeforeToolCallResult.Terminate",
		},
	},
	"symbol:agent/src/types.ts#CustomAgentMessages": {
		broaderContract: "contract:agent/messages",
		members: map[string]string{
			"bashExecution":     "github.com/nankedr/pig/agent.AgentMessage",
			"branchSummary":     "github.com/nankedr/pig/agent.AgentMessage",
			"compactionSummary": "github.com/nankedr/pig/agent.AgentMessage",
			"custom":            "github.com/nankedr/pig/agent.AgentMessage",
		},
	},
	"symbol:agent/src/types.ts#PrepareNextTurnContext": {
		broaderContract: "contract:agent/runtime",
		members: map[string]string{
			"context":     "github.com/nankedr/pig/agent.PrepareNextTurnContext.Context",
			"message":     "github.com/nankedr/pig/agent.PrepareNextTurnContext.Message",
			"newMessages": "github.com/nankedr/pig/agent.PrepareNextTurnContext.NewMessages",
			"toolResults": "github.com/nankedr/pig/agent.PrepareNextTurnContext.ToolResults",
		},
	},
	"symbol:agent/src/types.ts#ShouldStopAfterTurnContext": {
		broaderContract: "contract:agent/runtime",
		members: map[string]string{
			"context":     "github.com/nankedr/pig/agent.ShouldStopAfterTurnContext.Context",
			"message":     "github.com/nankedr/pig/agent.ShouldStopAfterTurnContext.Message",
			"newMessages": "github.com/nankedr/pig/agent.ShouldStopAfterTurnContext.NewMessages",
			"toolResults": "github.com/nankedr/pig/agent.ShouldStopAfterTurnContext.ToolResults",
		},
	},
	"symbol:agent/src/types.ts#QueueMode": {
		parentTarget:      "github.com/nankedr/pig/agent.QueueMode",
		broaderContract:   "contract:agent/state-and-queues",
		languageInherited: true,
	},
	"symbol:agent/src/types.ts#ThinkingLevel": {
		parentTarget:      "github.com/nankedr/pig/agent.ThinkingLevel",
		broaderContract:   "contract:agent/state-and-queues",
		languageInherited: true,
	},
	"symbol:agent/src/types.ts#ToolExecutionMode": {
		parentTarget:      "github.com/nankedr/pig/agent.ToolExecutionMode",
		broaderContract:   "contract:agent/tools",
		languageInherited: true,
	},
}

type issue29EventVariant struct {
	name          string
	typ           reflect.Type
	discriminator reflect.Value
}

func issue29ValidateEventVariants(t *testing.T) {
	t.Helper()
	agentEventType := reflect.TypeOf((*agent.AgentEvent)(nil)).Elem()
	agentDiscriminatorType := reflect.TypeOf(agent.AgentEventType(""))
	agentVariants := []issue29EventVariant{
		{"AgentStartEvent", reflect.TypeOf(agent.AgentStartEvent{}), reflect.ValueOf(agent.AgentEventTypeAgentStart)},
		{"AgentEndEvent", reflect.TypeOf(agent.AgentEndEvent{}), reflect.ValueOf(agent.AgentEventTypeAgentEnd)},
		{"TurnStartEvent", reflect.TypeOf(agent.TurnStartEvent{}), reflect.ValueOf(agent.AgentEventTypeTurnStart)},
		{"TurnEndEvent", reflect.TypeOf(agent.TurnEndEvent{}), reflect.ValueOf(agent.AgentEventTypeTurnEnd)},
		{"MessageStartEvent", reflect.TypeOf(agent.MessageStartEvent{}), reflect.ValueOf(agent.AgentEventTypeMessageStart)},
		{"MessageUpdateEvent", reflect.TypeOf(agent.MessageUpdateEvent{}), reflect.ValueOf(agent.AgentEventTypeMessageUpdate)},
		{"MessageEndEvent", reflect.TypeOf(agent.MessageEndEvent{}), reflect.ValueOf(agent.AgentEventTypeMessageEnd)},
		{"ToolExecutionStartEvent", reflect.TypeOf(agent.ToolExecutionStartEvent{}), reflect.ValueOf(agent.AgentEventTypeToolExecutionStart)},
		{"ToolExecutionUpdateEvent", reflect.TypeOf(agent.ToolExecutionUpdateEvent{}), reflect.ValueOf(agent.AgentEventTypeToolExecutionUpdate)},
		{"ToolExecutionEndEvent", reflect.TypeOf(agent.ToolExecutionEndEvent{}), reflect.ValueOf(agent.AgentEventTypeToolExecutionEnd)},
	}
	issue29ValidateEventVariantSet(t, agentEventType, agentDiscriminatorType, "AgentEventType", agentVariants)

	proxyEventType := reflect.TypeOf((*agent.ProxyAssistantMessageEvent)(nil)).Elem()
	proxyDiscriminatorType := reflect.TypeOf(ai.AssistantMessageEventType(""))
	proxyVariants := []issue29EventVariant{
		{"ProxyStartEvent", reflect.TypeOf(agent.ProxyStartEvent{}), reflect.ValueOf(ai.AssistantMessageEventTypeStart)},
		{"ProxyTextStartEvent", reflect.TypeOf(agent.ProxyTextStartEvent{}), reflect.ValueOf(ai.AssistantMessageEventTypeTextStart)},
		{"ProxyTextDeltaEvent", reflect.TypeOf(agent.ProxyTextDeltaEvent{}), reflect.ValueOf(ai.AssistantMessageEventTypeTextDelta)},
		{"ProxyTextEndEvent", reflect.TypeOf(agent.ProxyTextEndEvent{}), reflect.ValueOf(ai.AssistantMessageEventTypeTextEnd)},
		{"ProxyThinkingStartEvent", reflect.TypeOf(agent.ProxyThinkingStartEvent{}), reflect.ValueOf(ai.AssistantMessageEventTypeThinkingStart)},
		{"ProxyThinkingDeltaEvent", reflect.TypeOf(agent.ProxyThinkingDeltaEvent{}), reflect.ValueOf(ai.AssistantMessageEventTypeThinkingDelta)},
		{"ProxyThinkingEndEvent", reflect.TypeOf(agent.ProxyThinkingEndEvent{}), reflect.ValueOf(ai.AssistantMessageEventTypeThinkingEnd)},
		{"ProxyToolCallStartEvent", reflect.TypeOf(agent.ProxyToolCallStartEvent{}), reflect.ValueOf(ai.AssistantMessageEventTypeToolCallStart)},
		{"ProxyToolCallDeltaEvent", reflect.TypeOf(agent.ProxyToolCallDeltaEvent{}), reflect.ValueOf(ai.AssistantMessageEventTypeToolCallDelta)},
		{"ProxyToolCallEndEvent", reflect.TypeOf(agent.ProxyToolCallEndEvent{}), reflect.ValueOf(ai.AssistantMessageEventTypeToolCallEnd)},
		{"ProxyDoneEvent", reflect.TypeOf(agent.ProxyDoneEvent{}), reflect.ValueOf(ai.AssistantMessageEventTypeDone)},
		{"ProxyErrorEvent", reflect.TypeOf(agent.ProxyErrorEvent{}), reflect.ValueOf(ai.AssistantMessageEventTypeError)},
	}
	issue29ValidateEventVariantSet(t, proxyEventType, proxyDiscriminatorType, "ProxyAssistantMessageEventType", proxyVariants)
}

func issue29ValidateEventVariantSet(t *testing.T, interfaceType, discriminatorType reflect.Type, methodName string, variants []issue29EventVariant) {
	t.Helper()
	seen := make(map[any]string, len(variants))
	for _, variant := range variants {
		if !variant.typ.Implements(interfaceType) {
			t.Errorf("agent.%s does not implement %s", variant.name, interfaceType)
			continue
		}
		field, ok := variant.typ.FieldByName("Type")
		if !ok || field.PkgPath != "" || len(field.Index) != 1 {
			t.Errorf("agent.%s lacks a directly declared exported Type field", variant.name)
			continue
		}
		if field.Type != discriminatorType {
			t.Errorf("agent.%s.Type type = %s, want %s", variant.name, field.Type, discriminatorType)
			continue
		}
		value := reflect.New(variant.typ).Elem()
		value.FieldByIndex(field.Index).Set(variant.discriminator)
		got := value.MethodByName(methodName).Call(nil)[0].Interface()
		want := variant.discriminator.Interface()
		if got != want {
			t.Errorf("agent.%s.%s() = %v, want its Type field %v", variant.name, methodName, got, want)
		}
		if previous, duplicate := seen[want]; duplicate {
			t.Errorf("agent.%s and agent.%s share discriminator %v", previous, variant.name, want)
		}
		seen[want] = variant.name
	}
}

func TestIssue29LockedGoAPISnapshot(t *testing.T) {
	root := issue29RepoRoot(t)
	goldenPath := filepath.Join(root, "agent", "testdata", "issue29_surface_golden.txt")
	got, err := issue29SurfaceSnapshot(issue29CatalogSymbolEntries(t, root))
	if err != nil {
		t.Fatalf("issue29SurfaceSnapshot: %v", err)
	}
	if *updateIssue29Surface {
		if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden %s: %v", goldenPath, err)
		}
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %s: %v\n--- generated snapshot ---\n%s", goldenPath, err, got)
	}
	if got != string(want) {
		t.Fatalf("issue29 Go API snapshot drifted from %s\n--- got ---\n%s\n--- want ---\n%s", goldenPath, got, string(want))
	}
}

func issue29LegacyAgentSymbol(symbol surface.Symbol) bool {
	return symbol.Module == "agent" && issue29LegacyAgentReference(symbol.Upstream.Reference)
}

func issue29LegacyAgentReference(reference string) bool {
	for _, prefix := range []string{
		"packages/agent/src/agent.ts#",
		"packages/agent/src/agent-loop.ts#",
		"packages/agent/src/types.ts#",
		"packages/agent/src/proxy.ts#",
		"packages/agent/src/stream-fn.ts#",
	} {
		if strings.HasPrefix(reference, prefix) {
			return true
		}
	}
	return false
}

func issue29CatalogSymbolEntries(t *testing.T, root string) []catalog.Entry {
	t.Helper()
	entries, err := catalog.LoadCatalog(filepath.Join(root, "parity", "catalog.jsonl"))
	if err != nil {
		t.Fatalf("LoadCatalog: %v", err)
	}
	var filtered []catalog.Entry
	for _, entry := range entries {
		if entry.Mapping.Module != "agent" || entry.Mapping.Kind != "symbol" {
			continue
		}
		if !issue29LegacyAgentReference(entry.Upstream.Reference) {
			continue
		}
		filtered = append(filtered, entry)
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].ID < filtered[j].ID })
	return filtered
}

func issue29CatalogMemberEntries(t *testing.T, root string) []catalog.Entry {
	t.Helper()
	entries, err := catalog.LoadCatalog(filepath.Join(root, "parity", "catalog.jsonl"))
	if err != nil {
		t.Fatalf("LoadCatalog: %v", err)
	}
	var filtered []catalog.Entry
	for _, entry := range entries {
		if !issue29LegacyAgentMemberID(entry.ID) {
			continue
		}
		filtered = append(filtered, entry)
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].ID < filtered[j].ID })
	return filtered
}

func issue29LegacyAgentMemberID(id string) bool {
	if !strings.HasPrefix(id, "member:") {
		return false
	}
	return issue29LegacyAgentReference("packages/" + strings.TrimPrefix(id, "member:"))
}

func issue29RepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
}

func issue29SurfaceSnapshot(entries []catalog.Entry) (string, error) {
	var b strings.Builder
	for index, entry := range entries {
		if index != 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "## %s\n", entry.ID)
		fmt.Fprintf(&b, "reference: %s\n", entry.Upstream.Reference)
		fmt.Fprintf(&b, "target: %s\n", entry.Mapping.Target)
		resolver, ok := issue29SnapshotResolver(entry.Mapping.Target)
		if !ok {
			return "", fmt.Errorf("missing snapshot resolver for %s", entry.Mapping.Target)
		}
		b.WriteString(resolver())
		b.WriteString("\n")
	}
	return b.String(), nil
}

type issue29SnapshotFunc func() string

func issue29SnapshotResolver(target string) (issue29SnapshotFunc, bool) {
	switch target {
	case "github.com/nankedr/pig/agent.Agent":
		return issue29AgentTypeSnapshot, true
	case "github.com/nankedr/pig/agent.AgentContext":
		return func() string {
			return issue29StructSnapshot("agent.AgentContext", reflect.TypeOf(agent.AgentContext{}))
		}, true
	case "github.com/nankedr/pig/agent.AgentEvent":
		return issue29AgentEventSnapshot, true
	case "github.com/nankedr/pig/agent.AgentEventSink":
		return func() string {
			return issue29NamedFuncType("agent.AgentEventSink", reflect.TypeOf((*agent.AgentEventSink)(nil)).Elem())
		}, true
	case "github.com/nankedr/pig/agent.AgentLoop":
		return func() string { return issue29FuncValue("agent.AgentLoop", agent.AgentLoop) }, true
	case "github.com/nankedr/pig/agent.AgentLoopConfig":
		return func() string {
			return issue29StructSnapshot("agent.AgentLoopConfig", reflect.TypeOf(agent.AgentLoopConfig{}))
		}, true
	case "github.com/nankedr/pig/agent.AgentLoopContinue":
		return func() string { return issue29FuncValue("agent.AgentLoopContinue", agent.AgentLoopContinue) }, true
	case "github.com/nankedr/pig/agent.AgentLoopTurnUpdate":
		return func() string {
			return issue29StructSnapshot("agent.AgentLoopTurnUpdate", reflect.TypeOf(agent.AgentLoopTurnUpdate{}))
		}, true
	case "github.com/nankedr/pig/agent.AgentMessage":
		return func() string {
			return issue29InterfaceSnapshot("agent.AgentMessage", reflect.TypeOf((*agent.AgentMessage)(nil)).Elem())
		}, true
	case "github.com/nankedr/pig/agent.AgentOptions":
		return func() string {
			return issue29StructSnapshot("agent.AgentOptions", reflect.TypeOf(agent.AgentOptions{}))
		}, true
	case "github.com/nankedr/pig/agent.AgentState":
		return func() string { return issue29StructSnapshot("agent.AgentState", reflect.TypeOf(agent.AgentState{})) }, true
	case "github.com/nankedr/pig/agent.AgentTool":
		return func() string {
			return issue29StructSnapshot("agent.AgentTool[map[string]any, map[string]any]", reflect.TypeOf(agent.AgentTool[map[string]any, map[string]any]{}))
		}, true
	case "github.com/nankedr/pig/agent.AgentToolCall":
		return func() string { return "type agent.AgentToolCall = ai.ToolCall" }, true
	case "github.com/nankedr/pig/agent.AgentToolResult":
		return func() string {
			return issue29StructSnapshot("agent.AgentToolResult[map[string]any]", reflect.TypeOf(agent.AgentToolResult[map[string]any]{}))
		}, true
	case "github.com/nankedr/pig/agent.AgentToolUpdateCallback":
		return func() string {
			return issue29NamedFuncType("agent.AgentToolUpdateCallback[map[string]any]", reflect.TypeOf((*agent.AgentToolUpdateCallback[map[string]any])(nil)).Elem())
		}, true
	case "github.com/nankedr/pig/agent.AfterToolCallContext":
		return func() string {
			return issue29StructSnapshot("agent.AfterToolCallContext", reflect.TypeOf(agent.AfterToolCallContext{}))
		}, true
	case "github.com/nankedr/pig/agent.AfterToolCallResult":
		return func() string {
			return issue29StructSnapshot("agent.AfterToolCallResult", reflect.TypeOf(agent.AfterToolCallResult{}))
		}, true
	case "github.com/nankedr/pig/agent.BeforeToolCallContext":
		return func() string {
			return issue29StructSnapshot("agent.BeforeToolCallContext", reflect.TypeOf(agent.BeforeToolCallContext{}))
		}, true
	case "github.com/nankedr/pig/agent.BeforeToolCallResult":
		return func() string {
			return issue29StructSnapshot("agent.BeforeToolCallResult", reflect.TypeOf(agent.BeforeToolCallResult{}))
		}, true
	case "github.com/nankedr/pig/agent.CustomAgentMessages":
		return func() string { return "type agent.CustomAgentMessages = agent.AgentMessage" }, true
	case "github.com/nankedr/pig/agent.PrepareNextTurnContext":
		return func() string { return "type agent.PrepareNextTurnContext = agent.ShouldStopAfterTurnContext" }, true
	case "github.com/nankedr/pig/agent.ProxyAssistantMessageEvent":
		return issue29ProxyAssistantMessageEventSnapshot, true
	case "github.com/nankedr/pig/agent.ProxyStreamOptions":
		return func() string {
			return issue29StructSnapshot("agent.ProxyStreamOptions", reflect.TypeOf(agent.ProxyStreamOptions{}))
		}, true
	case "github.com/nankedr/pig/agent.QueueMode":
		return func() string { return "type agent.QueueMode string" }, true
	case "github.com/nankedr/pig/agent.RunAgentLoop":
		return func() string { return issue29FuncValue("agent.RunAgentLoop", agent.RunAgentLoop) }, true
	case "github.com/nankedr/pig/agent.RunAgentLoopContinue":
		return func() string { return issue29FuncValue("agent.RunAgentLoopContinue", agent.RunAgentLoopContinue) }, true
	case "github.com/nankedr/pig/agent.SetDefaultStreamFunction":
		return func() string {
			return issue29FuncValue("agent.SetDefaultStreamFunction", agent.SetDefaultStreamFunction)
		}, true
	case "github.com/nankedr/pig/agent.ShouldStopAfterTurnContext":
		return func() string {
			return issue29StructSnapshot("agent.ShouldStopAfterTurnContext", reflect.TypeOf(agent.ShouldStopAfterTurnContext{}))
		}, true
	case "github.com/nankedr/pig/agent.StreamFunction":
		return func() string {
			return issue29NamedFuncType("agent.StreamFunction", reflect.TypeOf((*agent.StreamFunction)(nil)).Elem())
		}, true
	case "github.com/nankedr/pig/agent.StreamProxy":
		return func() string { return issue29FuncValue("agent.StreamProxy", agent.StreamProxy) }, true
	case "github.com/nankedr/pig/agent.ThinkingLevel":
		return func() string { return "type agent.ThinkingLevel = ai.ModelThinkingLevel" }, true
	case "github.com/nankedr/pig/agent.ToolExecutionMode":
		return func() string { return "type agent.ToolExecutionMode string" }, true
	default:
		return nil, false
	}
}

func issue29FuncValue(name string, value any) string {
	return "func " + name + " " + issue29FuncSignature(reflect.TypeOf(value))
}

func issue29NamedFuncType(name string, t reflect.Type) string {
	return "type " + name + " " + issue29FuncSignature(t)
}

func issue29FuncSignature(t reflect.Type) string {
	var args []string
	for i := 0; i < t.NumIn(); i++ {
		if t.IsVariadic() && i == t.NumIn()-1 {
			args = append(args, "..."+issue29TypeString(t.In(i).Elem()))
			continue
		}
		args = append(args, issue29TypeString(t.In(i)))
	}
	var returns []string
	for i := 0; i < t.NumOut(); i++ {
		returns = append(returns, issue29TypeString(t.Out(i)))
	}
	signature := fmt.Sprintf("func(%s)", strings.Join(args, ", "))
	switch len(returns) {
	case 0:
		return signature
	case 1:
		return signature + " " + returns[0]
	default:
		return signature + " (" + strings.Join(returns, ", ") + ")"
	}
}

func issue29TypeString(t reflect.Type) string {
	if t == reflect.TypeOf([]byte(nil)) {
		return "[]byte"
	}
	return t.String()
}

func issue29InterfaceSnapshot(name string, t reflect.Type) string {
	methods := make([]string, 0, t.NumMethod())
	for i := 0; i < t.NumMethod(); i++ {
		method := t.Method(i)
		methods = append(methods, fmt.Sprintf("- %s %s", method.Name, issue29FuncSignature(method.Type)))
	}
	sort.Strings(methods)
	return strings.Join([]string{
		"type " + name + " interface",
		"methods:",
		strings.Join(methods, "\n"),
	}, "\n")
}

func issue29StructSnapshot(name string, t reflect.Type) string {
	fields := make([]string, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.PkgPath != "" {
			continue
		}
		line := fmt.Sprintf("- %s %s", field.Name, field.Type.String())
		if field.Anonymous {
			line += " embedded"
		}
		if tag := string(field.Tag); tag != "" {
			line += fmt.Sprintf(" tag=%q", tag)
		}
		fields = append(fields, line)
	}
	return strings.Join([]string{
		"type " + name + " struct",
		"fields:",
		strings.Join(fields, "\n"),
	}, "\n")
}

func issue29AgentTypeSnapshot() string {
	receiver := reflect.TypeOf((*agent.Agent)(nil))
	methods := make([]string, 0, receiver.NumMethod())
	for i := 0; i < receiver.NumMethod(); i++ {
		method := receiver.Method(i)
		methods = append(methods, fmt.Sprintf("- %s %s", method.Name, issue29FuncSignature(method.Type)))
	}
	sort.Strings(methods)
	return strings.Join([]string{
		"type agent.Agent struct",
		"constructor " + issue29FuncSignature(reflect.TypeOf(agent.NewAgent)),
		"methods:",
		strings.Join(methods, "\n"),
	}, "\n")
}

func issue29AgentEventSnapshot() string {
	variants := []string{
		issue29VariantStructSnapshot("agent.AgentStartEvent", string(agent.AgentEventTypeAgentStart), reflect.TypeOf(agent.AgentStartEvent{})),
		issue29VariantStructSnapshot("agent.AgentEndEvent", string(agent.AgentEventTypeAgentEnd), reflect.TypeOf(agent.AgentEndEvent{})),
		issue29VariantStructSnapshot("agent.TurnStartEvent", string(agent.AgentEventTypeTurnStart), reflect.TypeOf(agent.TurnStartEvent{})),
		issue29VariantStructSnapshot("agent.TurnEndEvent", string(agent.AgentEventTypeTurnEnd), reflect.TypeOf(agent.TurnEndEvent{})),
		issue29VariantStructSnapshot("agent.MessageStartEvent", string(agent.AgentEventTypeMessageStart), reflect.TypeOf(agent.MessageStartEvent{})),
		issue29VariantStructSnapshot("agent.MessageUpdateEvent", string(agent.AgentEventTypeMessageUpdate), reflect.TypeOf(agent.MessageUpdateEvent{})),
		issue29VariantStructSnapshot("agent.MessageEndEvent", string(agent.AgentEventTypeMessageEnd), reflect.TypeOf(agent.MessageEndEvent{})),
		issue29VariantStructSnapshot("agent.ToolExecutionStartEvent", string(agent.AgentEventTypeToolExecutionStart), reflect.TypeOf(agent.ToolExecutionStartEvent{})),
		issue29VariantStructSnapshot("agent.ToolExecutionUpdateEvent", string(agent.AgentEventTypeToolExecutionUpdate), reflect.TypeOf(agent.ToolExecutionUpdateEvent{})),
		issue29VariantStructSnapshot("agent.ToolExecutionEndEvent", string(agent.AgentEventTypeToolExecutionEnd), reflect.TypeOf(agent.ToolExecutionEndEvent{})),
	}
	return strings.Join([]string{
		issue29InterfaceSnapshot("agent.AgentEvent", reflect.TypeOf((*agent.AgentEvent)(nil)).Elem()),
		"variants:",
		strings.Join(variants, "\n"),
		"codecs:",
		issue29FuncValue("agent.MarshalAgentEvent", agent.MarshalAgentEvent),
		issue29FuncValue("agent.UnmarshalAgentEvent", agent.UnmarshalAgentEvent),
	}, "\n")
}

func issue29ProxyAssistantMessageEventSnapshot() string {
	variants := []string{
		issue29VariantStructSnapshot("agent.ProxyStartEvent", string(ai.AssistantMessageEventTypeStart), reflect.TypeOf(agent.ProxyStartEvent{})),
		issue29VariantStructSnapshot("agent.ProxyTextStartEvent", string(ai.AssistantMessageEventTypeTextStart), reflect.TypeOf(agent.ProxyTextStartEvent{})),
		issue29VariantStructSnapshot("agent.ProxyTextDeltaEvent", string(ai.AssistantMessageEventTypeTextDelta), reflect.TypeOf(agent.ProxyTextDeltaEvent{})),
		issue29VariantStructSnapshot("agent.ProxyTextEndEvent", string(ai.AssistantMessageEventTypeTextEnd), reflect.TypeOf(agent.ProxyTextEndEvent{})),
		issue29VariantStructSnapshot("agent.ProxyThinkingStartEvent", string(ai.AssistantMessageEventTypeThinkingStart), reflect.TypeOf(agent.ProxyThinkingStartEvent{})),
		issue29VariantStructSnapshot("agent.ProxyThinkingDeltaEvent", string(ai.AssistantMessageEventTypeThinkingDelta), reflect.TypeOf(agent.ProxyThinkingDeltaEvent{})),
		issue29VariantStructSnapshot("agent.ProxyThinkingEndEvent", string(ai.AssistantMessageEventTypeThinkingEnd), reflect.TypeOf(agent.ProxyThinkingEndEvent{})),
		issue29VariantStructSnapshot("agent.ProxyToolCallStartEvent", string(ai.AssistantMessageEventTypeToolCallStart), reflect.TypeOf(agent.ProxyToolCallStartEvent{})),
		issue29VariantStructSnapshot("agent.ProxyToolCallDeltaEvent", string(ai.AssistantMessageEventTypeToolCallDelta), reflect.TypeOf(agent.ProxyToolCallDeltaEvent{})),
		issue29VariantStructSnapshot("agent.ProxyToolCallEndEvent", string(ai.AssistantMessageEventTypeToolCallEnd), reflect.TypeOf(agent.ProxyToolCallEndEvent{})),
		issue29VariantStructSnapshot("agent.ProxyDoneEvent", string(ai.AssistantMessageEventTypeDone), reflect.TypeOf(agent.ProxyDoneEvent{})),
		issue29VariantStructSnapshot("agent.ProxyErrorEvent", string(ai.AssistantMessageEventTypeError), reflect.TypeOf(agent.ProxyErrorEvent{})),
	}
	return strings.Join([]string{
		issue29InterfaceSnapshot("agent.ProxyAssistantMessageEvent", reflect.TypeOf((*agent.ProxyAssistantMessageEvent)(nil)).Elem()),
		"variants:",
		strings.Join(variants, "\n"),
		"codecs:",
		issue29FuncValue("agent.MarshalProxyAssistantMessageEvent", agent.MarshalProxyAssistantMessageEvent),
		issue29FuncValue("agent.UnmarshalProxyAssistantMessageEvent", agent.UnmarshalProxyAssistantMessageEvent),
	}, "\n")
}

func issue29VariantStructSnapshot(name, discriminator string, t reflect.Type) string {
	fields := make([]string, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.PkgPath != "" {
			continue
		}
		line := fmt.Sprintf("  - %s %s", field.Name, issue29TypeString(field.Type))
		if tag := string(field.Tag); tag != "" {
			line += fmt.Sprintf(" tag=%q", tag)
		}
		fields = append(fields, line)
	}
	return strings.Join([]string{
		fmt.Sprintf("- %s discriminator=%q", name, discriminator),
		"  fields:",
		strings.Join(fields, "\n"),
	}, "\n")
}

// Compile-time legacy Agent surface parity. Go uses exported names, context
// cancellation, explicit constructors, and typed authoring plus erased Tool
// registries; each marker maps one locked Pi symbol into that Go interface.
var (
	_ agent.AgentEventSink = func(context.Context, agent.AgentEvent) error { return nil } // AgentEventSink
	_                      = agent.AgentLoop                                              // agentLoop
	_                      = agent.AgentLoopContinue                                      // agentLoopContinue
	_                      = agent.RunAgentLoop                                           // runAgentLoop
	_                      = agent.RunAgentLoopContinue                                   // runAgentLoopContinue

	_ *agent.Agent           // Agent
	_ = agent.AgentOptions{} // AgentOptions

	_ agent.ProxyAssistantMessageEvent // ProxyAssistantMessageEvent
	_ = agent.ProxyStreamOptions{}     // ProxyStreamOptions
	_ = agent.StreamProxy              // streamProxy
	_ = agent.SetDefaultStreamFunction // setDefaultStreamFn

	_ = agent.AfterToolCallContext{}                                                           // AfterToolCallContext
	_ = agent.AfterToolCallResult{}                                                            // AfterToolCallResult
	_ = agent.AgentContext{}                                                                   // AgentContext
	_ agent.AgentEvent                                                                         // AgentEvent
	_ = agent.AgentLoopConfig{}                                                                // AgentLoopConfig
	_ = agent.AgentLoopTurnUpdate{}                                                            // AgentLoopTurnUpdate
	_ agent.AgentMessage                                                                       // AgentMessage
	_ = agent.AgentState{}                                                                     // AgentState
	_ = agent.AgentTool[map[string]any, map[string]any]{}                                      // AgentTool
	_ = agent.AgentToolCall{}                                                                  // AgentToolCall
	_ = agent.AgentToolResult[map[string]any]{}                                                // AgentToolResult
	_ agent.AgentToolUpdateCallback[map[string]any]                                            // AgentToolUpdateCallback
	_ = agent.BeforeToolCallContext{}                                                          // BeforeToolCallContext
	_ = agent.BeforeToolCallResult{}                                                           // BeforeToolCallResult
	_ agent.CustomAgentMessages                                                                // CustomAgentMessages
	_                                                     = agent.PrepareNextTurnContext{}     // PrepareNextTurnContext
	_ agent.QueueMode                                     = agent.QueueOneAtATime              // QueueMode
	_                                                     = agent.ShouldStopAfterTurnContext{} // ShouldStopAfterTurnContext
	_ agent.StreamFunction                                                                     // StreamFn
	_ agent.ThinkingLevel                                 = ai.ModelThinkingLevelOff           // ThinkingLevel
	_ agent.ToolExecutionMode                             = agent.ToolExecutionParallel        // ToolExecutionMode
)

// Compile-time Agent member snapshot. Explicit setters replace mutable Pi
// properties while preserving the same observable state and queue operations.
var (
	_ = agent.NewAgent
	_ = (*agent.Agent).State
	_ = (*agent.Agent).Busy
	_ = (*agent.Agent).ConvertToLLM
	_ = (*agent.Agent).SetConvertToLLM
	_ = (*agent.Agent).TransformContext
	_ = (*agent.Agent).SetTransformContext
	_ = (*agent.Agent).StreamFunction
	_ = (*agent.Agent).SetStreamFunction
	_ = (*agent.Agent).GetAPIKey
	_ = (*agent.Agent).SetGetAPIKey
	_ = (*agent.Agent).OnPayload
	_ = (*agent.Agent).SetOnPayload
	_ = (*agent.Agent).OnResponse
	_ = (*agent.Agent).SetOnResponse
	_ = (*agent.Agent).BeforeToolCall
	_ = (*agent.Agent).SetBeforeToolCall
	_ = (*agent.Agent).AfterToolCall
	_ = (*agent.Agent).SetAfterToolCall
	_ = (*agent.Agent).ShouldStopAfterTurn
	_ = (*agent.Agent).SetShouldStopAfterTurn
	_ = (*agent.Agent).PrepareNextTurn
	_ = (*agent.Agent).SetPrepareNextTurn
	_ = (*agent.Agent).PrepareNextTurnWithContext
	_ = (*agent.Agent).SetPrepareNextTurnWithContext
	_ = (*agent.Agent).SessionID
	_ = (*agent.Agent).SetSessionID
	_ = (*agent.Agent).ThinkingBudgets
	_ = (*agent.Agent).SetThinkingBudgets
	_ = (*agent.Agent).Transport
	_ = (*agent.Agent).SetTransport
	_ = (*agent.Agent).MaxRetryDelayMS
	_ = (*agent.Agent).SetMaxRetryDelayMS
	_ = (*agent.Agent).ToolExecution
	_ = (*agent.Agent).SetToolExecution
	_ = (*agent.Agent).SetSystemPrompt
	_ = (*agent.Agent).SetModel
	_ = (*agent.Agent).SetThinkingLevel
	_ = (*agent.Agent).SetTools
	_ = (*agent.Agent).ReplaceMessages
	_ = (*agent.Agent).AppendMessage
	_ = (*agent.Agent).Subscribe
	_ = (*agent.Agent).Steer
	_ = (*agent.Agent).FollowUp
	_ = (*agent.Agent).ClearSteeringQueue
	_ = (*agent.Agent).ClearFollowUpQueue
	_ = (*agent.Agent).ClearAllQueues
	_ = (*agent.Agent).HasQueuedMessages
	_ = (*agent.Agent).SteeringMode
	_ = (*agent.Agent).SetSteeringMode
	_ = (*agent.Agent).FollowUpMode
	_ = (*agent.Agent).SetFollowUpMode
	_ = (*agent.Agent).ActiveContext
	_ = (*agent.Agent).Abort
	_ = (*agent.Agent).WaitForIdle
	_ = (*agent.Agent).Reset
	_ = (*agent.Agent).Prompt
	_ = (*agent.Agent).PromptText
	_ = (*agent.Agent).Continue
)

func TestIssue29FunctionMappingsUseExpectedGoNames(t *testing.T) {
	functions := []struct {
		value any
		want  string
	}{
		{agent.AgentLoop, "AgentLoop"},
		{agent.AgentLoopContinue, "AgentLoopContinue"},
		{agent.RunAgentLoop, "RunAgentLoop"},
		{agent.RunAgentLoopContinue, "RunAgentLoopContinue"},
		{agent.StreamProxy, "StreamProxy"},
		{agent.SetDefaultStreamFunction, "SetDefaultStreamFunction"},
	}
	for _, function := range functions {
		name := runtime.FuncForPC(reflect.ValueOf(function.value).Pointer()).Name()
		if !strings.HasSuffix(name, "/agent."+function.want) {
			t.Errorf("Go function name = %q, want %q", name, function.want)
		}
	}
}
