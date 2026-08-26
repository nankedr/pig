package codingagent_test

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"testing"

	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/protocol"
)

func TestSDKSessionPlumbingUsesConcreteTypes(t *testing.T) {
	requireModelRuntime := func(*codingagent.ModelRuntime) {}
	requireResourceLoader := func(codingagent.ResourceLoader) {}
	requireSettingsManager := func(*codingagent.SettingsManager) {}
	requireSessionStartEvent := func(*codingagent.SessionStartEvent) {}
	requireExtensionsResult := func(codingagent.LoadExtensionsResult) {}
	requireLoaderOptions := func(codingagent.DefaultResourceLoaderOptions) {}
	requireReloadOptions := func(codingagent.ResourceLoaderReloadOptions) {}
	requireProjectTrustContext := func(*codingagent.ProjectTrustContext) {}
	requireProjectTrustFactory := func(func(string) codingagent.ProjectTrustContext) {}

	sdkOptions := codingagent.CreateAgentSessionOptions{}
	requireModelRuntime(sdkOptions.ModelRuntime)
	requireResourceLoader(sdkOptions.ResourceLoader)
	requireSettingsManager(sdkOptions.SettingsManager)
	requireSessionStartEvent(sdkOptions.SessionStartEvent)
	requireExtensionsResult(codingagent.CreateAgentSessionResult{}.ExtensionsResult)

	services := codingagent.AgentSessionServices{}
	requireModelRuntime(services.ModelRuntime)
	requireResourceLoader(services.ResourceLoader)
	requireSettingsManager(services.SettingsManager)

	serviceOptions := codingagent.CreateAgentSessionServicesOptions{}
	requireModelRuntime(serviceOptions.ModelRuntime)
	requireSettingsManager(serviceOptions.SettingsManager)
	requireLoaderOptions(serviceOptions.ResourceLoaderOptions)
	requireReloadOptions(serviceOptions.ResourceLoaderReloadOptions)
	requireSessionStartEvent(codingagent.CreateAgentSessionFromServicesOptions{}.SessionStartEvent)

	runtimeOptions := codingagent.CreateAgentSessionRuntimeOptions{}
	requireSessionStartEvent(runtimeOptions.SessionStartEvent)
	requireProjectTrustContext(runtimeOptions.ProjectTrustContext)
	requireProjectTrustFactory(codingagent.SwitchSessionOptions{}.ProjectTrustContextFactory)
}

func TestStatefulModelFacadesUsePointerReceivers(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "models.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}

	methodCounts := map[string]int{
		"ModelRuntime":  0,
		"ModelRegistry": 0,
	}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Recv == nil || len(function.Recv.List) != 1 {
			continue
		}
		receiver, pointer := function.Recv.List[0].Type.(*ast.StarExpr)
		if !pointer {
			if identifier, ok := function.Recv.List[0].Type.(*ast.Ident); ok {
				if _, tracked := methodCounts[identifier.Name]; tracked {
					t.Errorf("%s.%s has a value receiver", identifier.Name, function.Name.Name)
				}
			}
			continue
		}
		identifier, ok := receiver.X.(*ast.Ident)
		if !ok {
			continue
		}
		if _, tracked := methodCounts[identifier.Name]; tracked {
			methodCounts[identifier.Name]++
		}
	}
	for receiver, count := range methodCounts {
		if count == 0 {
			t.Errorf("no pointer-receiver methods found for %s", receiver)
		}
	}

	if got := reflect.TypeOf(codingagent.ModelRuntime{}).NumMethod(); got != 0 {
		t.Errorf("ModelRuntime value method count = %d, want 0", got)
	}
	if got := reflect.TypeOf(codingagent.ModelRegistry{}).NumMethod(); got != 0 {
		t.Errorf("ModelRegistry value method count = %d, want 0", got)
	}
	if got := reflect.TypeOf((*codingagent.ModelRuntime)(nil)).NumMethod(); got != methodCounts["ModelRuntime"] {
		t.Errorf("*ModelRuntime method count = %d, want %d", got, methodCounts["ModelRuntime"])
	}
	if got := reflect.TypeOf((*codingagent.ModelRegistry)(nil)).NumMethod(); got != methodCounts["ModelRegistry"] {
		t.Errorf("*ModelRegistry method count = %d, want %d", got, methodCounts["ModelRegistry"])
	}
}

func TestTUIModeHasOneCanonicalExport(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "settings.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}

	var canonical *ast.TypeSpec
	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.TYPE {
			continue
		}
		for _, specification := range general.Specs {
			typeSpec := specification.(*ast.TypeSpec)
			switch typeSpec.Name.Name {
			case "TUIMode":
				canonical = typeSpec
			case "TuiMode":
				t.Error("settings.go exports duplicate TuiMode; TUIMode is the canonical mapping")
			}
		}
	}
	if canonical == nil || !canonical.Assign.IsValid() {
		t.Fatal("TUIMode must remain an alias of tui.TUIMode")
	}
}

func TestNewAgentSessionRuntimeOwnsConstructorInputs(t *testing.T) {
	serviceDiagnostics := []codingagent.AgentSessionRuntimeDiagnostic{{Type: "warning", Message: "service warning"}}
	runtimeDiagnostics := []codingagent.AgentSessionRuntimeDiagnostic{{Type: "error", Message: "runtime error"}}
	fallback := "fallback warning"
	runtime := codingagent.NewAgentSessionRuntime(
		nil,
		codingagent.AgentSessionServices{Diagnostics: serviceDiagnostics},
		nil,
		runtimeDiagnostics,
		&fallback,
	)

	serviceDiagnostics[0].Message = "caller mutation"
	runtimeDiagnostics[0].Message = "caller mutation"
	fallback = "caller mutation"

	services := runtime.Services()
	if got := services.Diagnostics[0].Message; got != "service warning" {
		t.Fatalf("Services diagnostics after caller mutation = %q, want %q", got, "service warning")
	}
	if got := runtime.Diagnostics()[0].Message; got != "runtime error" {
		t.Fatalf("runtime diagnostics after caller mutation = %q, want %q", got, "runtime error")
	}
	if got := runtime.ModelFallbackMessage(); got == nil || *got != "fallback warning" {
		t.Fatalf("fallback after caller mutation = %v, want fallback warning", got)
	}

	services.Diagnostics[0].Message = "getter mutation"
	diagnostics := runtime.Diagnostics()
	diagnostics[0].Message = "getter mutation"
	returnedFallback := runtime.ModelFallbackMessage()
	*returnedFallback = "getter mutation"

	if got := runtime.Services().Diagnostics[0].Message; got != "service warning" {
		t.Fatalf("Services diagnostics alias getter result: %q", got)
	}
	if got := runtime.Diagnostics()[0].Message; got != "runtime error" {
		t.Fatalf("runtime diagnostics alias getter result: %q", got)
	}
	if got := runtime.ModelFallbackMessage(); got == nil || *got != "fallback warning" {
		t.Fatalf("fallback aliases getter result: %v", got)
	}
}

type namedJSONBytes []byte
type namedJSONArray []any
type namedJSONObject map[string]any
type namedStrings []string
type namedStringMap map[string]namedStrings

func TestTranscriptStateOwnsJSONCompositeAliases(t *testing.T) {
	raw := json.RawMessage(`{"ok":true}`)
	plainBytes := []byte{1, 2, 3}
	namedBytes := namedJSONBytes{4, 5, 6}
	array := namedJSONArray{namedJSONObject{"leaf": "kept"}}
	stringMap := namedStringMap{"items": {"first", "second"}}
	input := namedJSONObject{
		"raw":        raw,
		"plainBytes": plainBytes,
		"namedBytes": namedBytes,
		"array":      array,
		"stringMap":  stringMap,
	}
	details := json.RawMessage(`{"detail":true}`)
	snapshot := protocol.SessionSnapshot{
		ID: "session-1",
		Transcript: []protocol.TranscriptItem{protocol.CompleteToolTranscriptItem{
			ID:      "tool-1",
			Input:   input,
			Details: protocol.Some[protocol.JSONValue](details),
		}},
	}
	state := codingagent.CreateTranscriptState(snapshot)

	raw[0] = '!'
	plainBytes[0] = 9
	namedBytes[0] = 9
	array[0].(namedJSONObject)["leaf"] = "caller mutation"
	stringMap["items"][0] = "caller mutation"
	details[0] = '!'
	delete(input, "raw")

	first := transcriptToolJSON(t, codingagent.SelectTranscript(state))
	assertOwnedJSONCompositeValues(t, first.input, first.details)

	first.input["raw"].(json.RawMessage)[1] = '!'
	first.input["plainBytes"].([]byte)[1] = 9
	first.input["namedBytes"].(namedJSONBytes)[1] = 9
	first.input["array"].(namedJSONArray)[0].(namedJSONObject)["leaf"] = "getter mutation"
	first.input["stringMap"].(namedStringMap)["items"][0] = "getter mutation"
	first.details[1] = '!'
	first.input["new"] = true

	second := transcriptToolJSON(t, codingagent.SelectTranscript(state))
	assertOwnedJSONCompositeValues(t, second.input, second.details)
}

type toolJSON struct {
	input   namedJSONObject
	details json.RawMessage
}

func transcriptToolJSON(t *testing.T, transcript []protocol.TranscriptItem) toolJSON {
	t.Helper()
	if len(transcript) != 1 {
		t.Fatalf("transcript length = %d, want 1", len(transcript))
	}
	item, ok := transcript[0].(protocol.CompleteToolTranscriptItem)
	if !ok {
		t.Fatalf("transcript item type = %T, want protocol.CompleteToolTranscriptItem", transcript[0])
	}
	input, ok := item.Input.(namedJSONObject)
	if !ok {
		t.Fatalf("input type = %T, want namedJSONObject", item.Input)
	}
	details, ok := item.Details.Value.(json.RawMessage)
	if !item.Details.Present || !ok {
		t.Fatalf("details = %#v (%T), want present json.RawMessage", item.Details, item.Details.Value)
	}
	return toolJSON{input: input, details: details}
}

func assertOwnedJSONCompositeValues(t *testing.T, input namedJSONObject, details json.RawMessage) {
	t.Helper()
	if got := string(input["raw"].(json.RawMessage)); got != `{"ok":true}` {
		t.Errorf("raw message = %q, want original bytes", got)
	}
	if got := input["plainBytes"].([]byte); !reflect.DeepEqual(got, []byte{1, 2, 3}) {
		t.Errorf("plain bytes = %v, want [1 2 3]", got)
	}
	if got := input["namedBytes"].(namedJSONBytes); !reflect.DeepEqual(got, namedJSONBytes{4, 5, 6}) {
		t.Errorf("named bytes = %v, want [4 5 6]", got)
	}
	if got := input["array"].(namedJSONArray)[0].(namedJSONObject)["leaf"]; got != "kept" {
		t.Errorf("named array/map leaf = %v, want kept", got)
	}
	if got := input["stringMap"].(namedStringMap)["items"]; !reflect.DeepEqual(got, namedStrings{"first", "second"}) {
		t.Errorf("typed named map = %v, want [first second]", got)
	}
	if _, exists := input["new"]; exists {
		t.Error("getter mutation added a key to stored input")
	}
	if got := string(details); got != `{"detail":true}` {
		t.Errorf("details = %q, want original bytes", got)
	}
}
