package codingagent_test

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/nankedr/pig/codingagent"
)

func TestExtensionBoundaryDeclaresNoExecutableFunctionFields(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve extension review test path")
	}
	path := filepath.Join(filepath.Dir(currentFile), "extensions.go")
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	forbiddenFunctions := map[string]bool{
		"CreateExtensionRuntime": true,
		"NewExtensionRunner":     true,
		"WrapRegisteredTool":     true,
		"WrapRegisteredTools":    true,
	}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok {
			continue
		}
		if function.Recv == nil {
			if forbiddenFunctions[function.Name.Name] {
				t.Errorf("extension runtime function %s freezes an executable ABI before M7", function.Name.Name)
			}
			continue
		}
		receiver := function.Recv.List[0].Type
		if pointer, ok := receiver.(*ast.StarExpr); ok {
			receiver = pointer.X
		}
		name, ok := receiver.(*ast.Ident)
		if ok && (name.Name == "ExtensionRunner" || name.Name == "ExtensionRuntime") {
			t.Errorf("%s.%s freezes an executable runtime method before M7", name.Name, function.Name.Name)
		}
	}

	ast.Inspect(file, func(node ast.Node) bool {
		declaration, ok := node.(*ast.TypeSpec)
		if !ok {
			return true
		}
		if _, executable := declaration.Type.(*ast.FuncType); executable {
			t.Errorf("extension ABI type %s is executable before M7", declaration.Name.Name)
		}
		carrier, ok := declaration.Type.(*ast.StructType)
		if ok {
			for _, field := range carrier.Fields.List {
				if _, executable := field.Type.(*ast.FuncType); !executable {
					continue
				}
				for _, name := range field.Names {
					t.Errorf("extension field %s.%s is executable before M7", declaration.Name.Name, name.Name)
				}
			}
		}

		contract, ok := declaration.Type.(*ast.InterfaceType)
		if ok {
			for _, method := range contract.Methods.List {
				methodType, ok := method.Type.(*ast.FuncType)
				if !ok || len(method.Names) == 0 {
					continue
				}
				if declaration.Name.Name == "EventBus" || declaration.Name.Name == "EventBusController" {
					t.Errorf("extension event bus %s.%s is callable before M7", declaration.Name.Name, method.Names[0].Name)
				}
				for _, fields := range []*ast.FieldList{methodType.Params, methodType.Results} {
					if fields == nil {
						continue
					}
					for _, field := range fields.List {
						ast.Inspect(field.Type, func(node ast.Node) bool {
							if _, executable := node.(*ast.FuncType); executable {
								t.Errorf("extension interface method %s.%s exposes a Go callback type before M7", declaration.Name.Name, method.Names[0].Name)
								return false
							}
							return true
						})
					}
				}
			}
		}
		return false
	})
}

func TestExtensionInterfaceCallbackSlotsUseOpaqueCarrier(t *testing.T) {
	handler := reflect.TypeOf((*codingagent.ExtensionHandler)(nil)).Elem()
	errorType := reflect.TypeOf((*error)(nil)).Elem()
	intType := reflect.TypeOf(0)
	stringMapType := reflect.TypeOf(map[string]string(nil))
	stringPointerType := reflect.TypeOf((*string)(nil))

	for carrier, fields := range map[reflect.Type][]string{
		reflect.TypeOf(codingagent.EventBus{}):           {"Emit", "On"},
		reflect.TypeOf(codingagent.EventBusController{}): {"Clear", "Emit", "On"},
	} {
		for _, name := range fields {
			field, ok := carrier.FieldByName(name)
			if !ok || field.Type != handler {
				t.Errorf("%s.%s = %v, want opaque %v", carrier.Name(), name, field.Type, handler)
			}
		}
	}
	assertInterfaceMethods(t, reflect.TypeOf((*codingagent.ReadonlyFooterDataProvider)(nil)).Elem(), map[string]reflect.Type{
		"GetAvailableProviderCount": reflect.FuncOf(nil, []reflect.Type{intType}, false),
		"GetExtensionStatuses":      reflect.FuncOf(nil, []reflect.Type{stringMapType}, false),
		"GetGitBranch":              reflect.FuncOf(nil, []reflect.Type{stringPointerType}, false),
		"OnBranchChange":            reflect.FuncOf([]reflect.Type{handler}, []reflect.Type{handler, errorType}, false),
	})
}

func TestExtensionErrorAndLoadErrorCarriersMatchPinnedData(t *testing.T) {
	for name, carrier := range map[string]reflect.Type{
		"ExtensionRunner":  reflect.TypeOf(codingagent.ExtensionRunner{}),
		"ExtensionRuntime": reflect.TypeOf(codingagent.ExtensionRuntime{}),
	} {
		if carrier.NumField() != 0 {
			t.Errorf("%s has %d fields, want an opaque carrier", name, carrier.NumField())
		}
		if reflect.PointerTo(carrier).NumMethod() != 0 {
			t.Errorf("%s exposes %d methods before the M7 ABI decision", name, reflect.PointerTo(carrier).NumMethod())
		}
	}

	extensionError := reflect.TypeOf(codingagent.ExtensionError{})
	wantExtensionFields := map[string]reflect.Type{
		"ExtensionPath": reflect.TypeOf(""),
		"Event":         reflect.TypeOf(""),
		"Error":         reflect.TypeOf(""),
		"Stack":         reflect.TypeOf((*string)(nil)),
	}
	for name, want := range wantExtensionFields {
		field, ok := extensionError.FieldByName(name)
		if !ok {
			t.Errorf("ExtensionError.%s is missing", name)
			continue
		}
		if field.Type != want {
			t.Errorf("ExtensionError.%s type = %v, want %v", name, field.Type, want)
		}
	}

	errorsField, ok := reflect.TypeOf(codingagent.LoadExtensionsResult{}).FieldByName("Errors")
	if !ok {
		t.Fatal("LoadExtensionsResult.Errors is missing")
	}
	if errorsField.Type.Kind() != reflect.Slice || errorsField.Type.Elem().Name() != "" {
		t.Fatalf("LoadExtensionsResult.Errors type = %v, want slice of anonymous records", errorsField.Type)
	}
	loadError := errorsField.Type.Elem()
	if loadError.NumField() != 2 {
		t.Fatalf("LoadExtensionsResult.Errors element has %d fields, want 2", loadError.NumField())
	}
	for _, name := range []string{"Path", "Error"} {
		field, ok := loadError.FieldByName(name)
		if !ok || field.Type != reflect.TypeOf("") {
			t.Errorf("LoadExtensionsResult.Errors element %s = %v, want string field", name, field.Type)
		}
	}
}

func TestDiscoverAndLoadExtensionsPreservesOnlyTheDeferredEntry(t *testing.T) {
	entry := reflect.TypeOf(codingagent.DiscoverAndLoadExtensions)
	if entry.IsVariadic() || entry.NumIn() != 0 {
		t.Fatalf("DiscoverAndLoadExtensions type = %v, want a zero-input deferred entry", entry)
	}
	if entry.NumOut() != 2 || entry.Out(0) != reflect.TypeOf(codingagent.LoadExtensionsResult{}) || entry.Out(1) != reflect.TypeOf((*error)(nil)).Elem() {
		t.Fatalf("DiscoverAndLoadExtensions type = %v, want func() (LoadExtensionsResult, error)", entry)
	}

	result, err := codingagent.DiscoverAndLoadExtensions()
	if !reflect.DeepEqual(result, codingagent.LoadExtensionsResult{}) {
		t.Fatalf("DiscoverAndLoadExtensions result = %#v, want zero value", result)
	}
	if !errors.Is(err, codingagent.ErrNotImplemented) {
		t.Fatalf("DiscoverAndLoadExtensions error = %v, want ErrNotImplemented", err)
	}
	var unavailable *codingagent.NotImplementedError
	if !errors.As(err, &unavailable) || unavailable.Module != "codingagent" || unavailable.Operation != "DiscoverAndLoadExtensions" {
		t.Fatalf("DiscoverAndLoadExtensions error = %#v, want codingagent.DiscoverAndLoadExtensions", unavailable)
	}
}

func assertInterfaceMethods(t *testing.T, contract reflect.Type, want map[string]reflect.Type) {
	t.Helper()
	if contract.Kind() != reflect.Interface {
		t.Fatalf("%v kind = %v, want interface", contract, contract.Kind())
	}
	if contract.NumMethod() != len(want) {
		t.Errorf("%v has %d methods, want %d", contract, contract.NumMethod(), len(want))
	}
	for name, wantType := range want {
		method, ok := contract.MethodByName(name)
		if !ok {
			t.Errorf("%v.%s is missing", contract, name)
			continue
		}
		if method.Type != wantType {
			t.Errorf("%v.%s type = %v, want %v", contract, name, method.Type, wantType)
		}
	}
}

func TestExtensionExecutableTypesRemainOpaqueUntilM7(t *testing.T) {
	carriers := map[string]reflect.Type{
		"AutocompleteProviderFactory": reflect.TypeOf((*codingagent.AutocompleteProviderFactory)(nil)).Elem(),
		"CreateExtensionRuntime":      reflect.TypeOf((*codingagent.CreateExtensionRuntime)(nil)).Elem(),
		"EntryRenderer":               reflect.TypeOf((*codingagent.EntryRenderer)(nil)).Elem(),
		"ExtensionFactory":            reflect.TypeOf((*codingagent.ExtensionFactory)(nil)).Elem(),
		"ExtensionHandler":            reflect.TypeOf((*codingagent.ExtensionHandler)(nil)).Elem(),
		"InlineExtension":             reflect.TypeOf((*codingagent.InlineExtension)(nil)).Elem(),
		"MarkdownTransformer":         reflect.TypeOf((*codingagent.MarkdownTransformer)(nil)).Elem(),
		"MessageRenderer":             reflect.TypeOf((*codingagent.MessageRenderer)(nil)).Elem(),
		"ProjectTrustHandler":         reflect.TypeOf((*codingagent.ProjectTrustHandler)(nil)).Elem(),
		"TerminalInputHandler":        reflect.TypeOf((*codingagent.TerminalInputHandler)(nil)).Elem(),
		"WrapRegisteredTool":          reflect.TypeOf((*codingagent.WrapRegisteredTool)(nil)).Elem(),
		"WrapRegisteredTools":         reflect.TypeOf((*codingagent.WrapRegisteredTools)(nil)).Elem(),
	}

	for name, carrier := range carriers {
		t.Run(name, func(t *testing.T) {
			if carrier.Kind() != reflect.Pointer {
				t.Fatalf("%s kind = %v, want an opaque, non-callable pointer carrier", name, carrier.Kind())
			}
			if carrier.NumMethod() != 0 {
				t.Fatalf("%s exposes %d methods before the M7 ABI decision", name, carrier.NumMethod())
			}
		})
	}
}

func TestExtensionOperationSlotsUseOpaqueCarrier(t *testing.T) {
	want := reflect.TypeOf((*codingagent.ExtensionHandler)(nil)).Elem()
	fieldsByCarrier := map[reflect.Type][]string{
		reflect.TypeOf(codingagent.CompactOptions{}): {
			"OnComplete", "OnError",
		},
		reflect.TypeOf(codingagent.ExtensionUIContext{}): {
			"AddAutocompleteProvider", "Confirm", "Custom", "Editor",
			"GetAllThemes", "GetEditorComponent", "GetEditorText", "GetTheme",
			"GetToolsExpanded", "Input", "Notify", "OnTerminalInput",
			"PasteToEditor", "Select", "SetEditorComponent", "SetEditorText",
			"SetFooter", "SetHeader", "SetHiddenThinkingLabel", "SetStatus",
			"SetTheme", "SetTitle", "SetToolsExpanded", "SetWidget",
			"SetWorkingIndicator", "SetWorkingMessage", "SetWorkingVisible",
		},
		reflect.TypeOf(codingagent.ExtensionContext{}): {
			"Abort", "Compact", "GetContextUsage", "GetSystemPrompt",
			"HasPendingMessages", "IsIdle", "IsProjectTrusted", "Shutdown",
		},
		reflect.TypeOf(codingagent.ExtensionCommandContext{}): {
			"Fork", "GetSystemPromptOptions", "NavigateTree", "NewSession",
			"Reload", "SwitchSession", "WaitForIdle",
		},
		reflect.TypeOf(codingagent.ExtensionContextActions{}): {
			"Abort", "Compact", "GetContextUsage", "GetModel", "GetScopedModels",
			"GetSignal", "GetSystemPrompt", "GetSystemPromptOptions", "HasPendingMessages",
			"IsIdle", "IsProjectTrusted", "Shutdown",
		},
		reflect.TypeOf(codingagent.ExtensionCommandContextActions{}): {
			"Fork", "NavigateTree", "NewSession", "Reload", "SwitchSession",
			"WaitForIdle",
		},
		reflect.TypeOf(codingagent.ExtensionActions{}): {
			"AppendEntry", "GetActiveTools", "GetAllTools", "GetCommands",
			"GetSessionName", "GetThinkingLevel", "RefreshTools", "SendMessage",
			"SendUserMessage", "SetActiveTools", "SetLabel", "SetModel",
			"SetSessionName", "SetThinkingLevel",
		},
		reflect.TypeOf(codingagent.ExtensionAPI{}): {
			"AppendEntry", "Exec", "GetActiveTools", "GetAllTools", "GetCommands",
			"GetFlag", "GetSessionName", "GetThinkingLevel", "On", "RegisterCommand",
			"RegisterEntryRenderer", "RegisterFlag", "RegisterMarkdownTransformer",
			"RegisterMessageRenderer", "RegisterProvider", "RegisterShortcut",
			"RegisterTool", "SendMessage", "SendUserMessage", "SetActiveTools",
			"SetLabel", "SetModel", "SetSessionName", "SetThinkingLevel",
			"UnregisterProvider",
		},
		reflect.TypeOf(codingagent.ExtensionShortcut{}): {"Handler"},
		reflect.TypeOf(codingagent.RegisteredCommand{}): {"GetArgumentCompletions", "Handler"},
		reflect.TypeOf(codingagent.ResolvedCommand{}):   {"GetArgumentCompletions", "Handler"},
		reflect.TypeOf(codingagent.ProviderConfig{}):    {"OAuth", "RefreshModels", "StreamSimple"},
	}

	for carrier, fieldNames := range fieldsByCarrier {
		for _, fieldName := range fieldNames {
			field, ok := carrier.FieldByName(fieldName)
			if !ok {
				t.Errorf("%s.%s is missing from the frozen member inventory", carrier.Name(), fieldName)
				continue
			}
			if field.Type != want {
				t.Errorf("%s.%s type = %v, want opaque %v", carrier.Name(), fieldName, field.Type, want)
			}
		}
	}
}

func TestKeybindingsManagerEffectiveConfigIsExplicitlyUnavailable(t *testing.T) {
	config, err := (&codingagent.KeybindingsManager{}).GetEffectiveConfig()
	if config != nil {
		t.Fatalf("GetEffectiveConfig result = %#v, want nil", config)
	}
	if !errors.Is(err, codingagent.ErrNotImplemented) {
		t.Fatalf("GetEffectiveConfig error = %v, want ErrNotImplemented", err)
	}
	var unavailable *codingagent.NotImplementedError
	if !errors.As(err, &unavailable) {
		t.Fatalf("GetEffectiveConfig error = %T, want *codingagent.NotImplementedError", err)
	}
	if unavailable.Module != "codingagent" || unavailable.Operation != "KeybindingsManager.GetEffectiveConfig" {
		t.Fatalf("NotImplementedError = %#v, want codingagent.KeybindingsManager.GetEffectiveConfig", unavailable)
	}
}
