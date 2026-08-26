package codingagent_test

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/nankedr/pig/internal/catalog"
	"github.com/nankedr/pig/internal/surface"
)

const (
	issue32ConstructorTestRef = "codingagent/issue32_constructor_surface_test.go#TestIssue32ConstructorTargetsResolve"
	issue32ConstructorTestRun = "go test ./codingagent -run '^(TestIssue32ConstructorMappingsMatchLockedSurface|TestIssue32ConstructorTargetsResolve)$' -count=1"
)

type issue32ConstructorMapping struct {
	target string
	status string
	reason string
}

// issue32ConstructorMappings deliberately distinguishes callable Go factories
// from constructor shapes that are only inventoried at M0. A carrier mapping
// records the public TypeScript operation without inventing callback, I/O, or
// lifecycle semantics before its owning runnable slice.
var issue32ConstructorMappings = map[string]issue32ConstructorMapping{
	"AgentSessionRuntime": {target: "NewAgentSessionRuntime", status: catalog.StatusScaffolded},
	"AgentSession":        {target: "NewAgentSession", status: catalog.StatusScaffolded},
	"ExtensionRunner":     {target: "ExtensionRunner", status: catalog.StatusInventoried},
	"KeybindingsManager": {
		target: "KeybindingsManager", status: catalog.StatusInventoried,
		reason: "the direct constructor is distinct from static KeybindingsManager.create, whose Go projection is NewKeybindingsManager",
	},
	"ModelRegistry":                  {target: "NewModelRegistry", status: catalog.StatusScaffolded},
	"CredentialSynchronizationError": {target: "CredentialSynchronizationError", status: catalog.StatusInventoried},
	"DefaultPackageManager":          {target: "NewDefaultPackageManager", status: catalog.StatusScaffolded},
	"DefaultResourceLoader":          {target: "NewDefaultResourceLoader", status: catalog.StatusScaffolded},
	"ProjectTrustStore":              {target: "NewProjectTrustStore", status: catalog.StatusScaffolded},

	"ArminComponent":                    {target: "ArminComponent", status: catalog.StatusInventoried},
	"AssistantMessageComponent":         {target: "AssistantMessageComponent", status: catalog.StatusInventoried},
	"BashExecutionComponent":            {target: "BashExecutionComponent", status: catalog.StatusInventoried},
	"BorderedLoader":                    {target: "BorderedLoader", status: catalog.StatusInventoried},
	"BranchSummaryMessageComponent":     {target: "BranchSummaryMessageComponent", status: catalog.StatusInventoried},
	"CompactionSummaryMessageComponent": {target: "CompactionSummaryMessageComponent", status: catalog.StatusInventoried},
	"CustomEditor":                      {target: "CustomEditor", status: catalog.StatusInventoried},
	"CustomMessageComponent":            {target: "CustomMessageComponent", status: catalog.StatusInventoried},
	"DynamicBorder":                     {target: "DynamicBorder", status: catalog.StatusInventoried},
	"ExtensionEditorComponent":          {target: "ExtensionEditorComponent", status: catalog.StatusInventoried},
	"ExtensionInputComponent":           {target: "ExtensionInputComponent", status: catalog.StatusInventoried},
	"ExtensionSelectorComponent":        {target: "ExtensionSelectorComponent", status: catalog.StatusInventoried},
	"FooterComponent":                   {target: "FooterComponent", status: catalog.StatusInventoried},
	"LoginDialogComponent":              {target: "LoginDialogComponent", status: catalog.StatusInventoried},
	"ModelSelectorComponent":            {target: "ModelSelectorComponent", status: catalog.StatusInventoried},
	"OAuthSelectorComponent":            {target: "OAuthSelectorComponent", status: catalog.StatusInventoried},
	"SessionSelectorComponent":          {target: "SessionSelectorComponent", status: catalog.StatusInventoried},
	"SettingsSelectorComponent":         {target: "SettingsSelectorComponent", status: catalog.StatusInventoried},
	"ShowImagesSelectorComponent":       {target: "ShowImagesSelectorComponent", status: catalog.StatusInventoried},
	"SkillInvocationMessageComponent":   {target: "SkillInvocationMessageComponent", status: catalog.StatusInventoried},
	"ThemeSelectorComponent":            {target: "ThemeSelectorComponent", status: catalog.StatusInventoried},
	"ThinkingSelectorComponent":         {target: "ThinkingSelectorComponent", status: catalog.StatusInventoried},
	"ToolExecutionComponent":            {target: "ToolExecutionComponent", status: catalog.StatusInventoried},
	"TreeSelectorComponent":             {target: "TreeSelectorComponent", status: catalog.StatusInventoried},
	"UserMessageSelectorComponent":      {target: "UserMessageSelectorComponent", status: catalog.StatusInventoried},
	"UserMessageComponent":              {target: "UserMessageComponent", status: catalog.StatusInventoried},

	"InteractiveMode": {target: "NewInteractiveMode", status: catalog.StatusScaffolded},
	"Theme":           {target: "Theme", status: catalog.StatusInventoried},
	"RpcClient":       {target: "NewRPCClient", status: catalog.StatusScaffolded},
}

func issue32ConstructorEntry(symbol surface.Symbol, milestone string) (catalog.Entry, error) {
	mapping, ok := issue32ConstructorMappings[symbol.Name]
	if !ok {
		return catalog.Entry{}, fmt.Errorf("no reviewed Coding Agent constructor mapping for %s", symbol.ID)
	}
	id := "constructor:" + strings.TrimPrefix(symbol.ID, "symbol:")
	target := issue32GoPackage + "." + mapping.target
	entry := catalog.Entry{
		SchemaVersion:  catalog.SchemaVersion,
		ID:             id,
		Upstream:       catalog.Upstream{Module: "coding-agent", Repository: symbol.Upstream.Repository, Commit: symbol.Upstream.Commit, Reference: symbol.Upstream.Reference + ".constructor"},
		Mapping:        catalog.Mapping{Module: "codingagent", Target: target, Kind: "contract"},
		Status:         mapping.status,
		Milestone:      milestone,
		Classification: "public-api",
	}
	if mapping.status == catalog.StatusScaffolded {
		entry.Evidence = []catalog.Evidence{{
			Kind: "go-test", Ref: issue32ConstructorTestRef, Baseline: issue32BaselineCommit,
			CaseID: id, InputHash: issue32SurfaceHash, ExecutionMethod: issue32ConstructorTestRun,
			Expected: fmt.Sprintf("the pinned Pi %s constructor maps to a declared Go package factory", symbol.Name),
			Actual:   fmt.Sprintf("PASS; %s resolved to %s", id, target), Platform: "any", CatalogID: id,
		}}
		entry.Notes = fmt.Sprintf("Issue #32 maps the public %s constructor to the compile-usable Go factory %s. Behavioral status remains on its owning capability contract.", symbol.Name, target)
		return entry, nil
	}
	reason := mapping.reason
	if reason == "" {
		reason = "no standalone Go factory is selected at the M0 scaffold boundary"
	}
	entry.Notes = fmt.Sprintf("Issue #32 inventories the public %s constructor on carrier %s because %s; no callable constructor behavior is claimed.", symbol.Name, target, reason)
	return entry, nil
}

func TestIssue32ConstructorMappingsMatchLockedSurface(t *testing.T) {
	constructible := map[string]surface.Symbol{}
	for _, symbol := range issue32Symbols(t) {
		if symbol.Constructible {
			constructible[symbol.Name] = symbol
		}
	}
	if len(constructible) != 38 {
		t.Fatalf("constructible Coding Agent classes = %d, want 38", len(constructible))
	}
	if len(issue32ConstructorMappings) != len(constructible) {
		t.Fatalf("reviewed constructor mappings = %d, want %d", len(issue32ConstructorMappings), len(constructible))
	}
	for name := range issue32ConstructorMappings {
		if _, ok := constructible[name]; !ok {
			t.Errorf("constructor mapping %s is not a constructible pinned class", name)
		}
	}
}

func TestIssue32ConstructorTargetsResolve(t *testing.T) {
	index, err := issue32LoadDecls(filepath.Join(issue32RepoRoot(t), "codingagent"))
	if err != nil {
		t.Fatal(err)
	}
	var scaffolded, inventoried int
	var names []string
	for name, mapping := range issue32ConstructorMappings {
		names = append(names, name)
		decl := index[mapping.target]
		if decl == nil {
			t.Errorf("%s constructor maps to missing Go declaration %s", name, mapping.target)
			continue
		}
		switch mapping.status {
		case catalog.StatusScaffolded:
			scaffolded++
			if !decl.packageFunction {
				t.Errorf("%s constructor target %s is not a package function", name, mapping.target)
			}
		case catalog.StatusInventoried:
			inventoried++
		default:
			t.Errorf("%s constructor status = %q", name, mapping.status)
		}
	}
	sort.Strings(names)
	if scaffolded != 8 || inventoried != 30 {
		t.Fatalf("constructor status counts = scaffolded %d, inventoried %d; want 8, 30; names=%v", scaffolded, inventoried, names)
	}
	if constructor, static := issue32ConstructorMappings["KeybindingsManager"].target, issue32StaticMemberTargetExceptions["KeybindingsManager.create"]; constructor == static {
		t.Fatalf("KeybindingsManager constructor target %q conflates the distinct static create target", constructor)
	}
}
