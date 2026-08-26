package codingagent_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/tui"
)

func TestCustomEditorRetainsTextWithoutActivatingDeferredRuntime(t *testing.T) {
	editor := new(codingagent.CustomEditor)
	var component tui.EditorComponent = editor

	if got := component.GetText(); got != "" {
		t.Fatalf("zero-value GetText() = %q, want empty text", got)
	}
	const text = "first line\nsecond line"
	if err := component.SetText(text); err != nil {
		t.Fatalf("SetText() error = %v, want nil", err)
	}
	if got := component.GetText(); got != text {
		t.Fatalf("GetText() = %q, want %q", got, text)
	}

	if err := component.HandleInput("x"); !errors.Is(err, codingagent.ErrNotImplemented) {
		t.Fatalf("HandleInput() error = %v, want ErrNotImplemented", err)
	}
	if lines, err := component.Render(80); lines != nil || !errors.Is(err, codingagent.ErrNotImplemented) {
		t.Fatalf("Render() = (%v, %v), want nil, ErrNotImplemented", lines, err)
	}
	if err := component.Invalidate(); err != nil {
		t.Fatalf("inherited Invalidate() error = %v, want nil", err)
	}
}

func TestCustomEditorCallbackSlotsRemainOpaqueUntilM7(t *testing.T) {
	editorType := reflect.TypeOf(codingagent.CustomEditor{})
	handlerType := reflect.TypeOf((*codingagent.ExtensionHandler)(nil)).Elem()
	actions, ok := editorType.FieldByName("ActionHandlers")
	if !ok {
		t.Fatal("CustomEditor.ActionHandlers is missing")
	}
	wantActions := reflect.MapOf(reflect.TypeOf(codingagent.AppKeybinding("")), handlerType)
	if actions.Type != wantActions {
		t.Fatalf("CustomEditor.ActionHandlers type = %v, want %v", actions.Type, wantActions)
	}

	pointerType := reflect.TypeOf((*codingagent.CustomEditor)(nil))
	for _, methodName := range []string{"OnAction", "OnCtrlD", "OnEscape", "OnExtensionShortcut", "OnPasteImage"} {
		method, ok := pointerType.MethodByName(methodName)
		if !ok {
			t.Errorf("CustomEditor.%s is missing", methodName)
			continue
		}
		for index := 1; index < method.Type.NumIn(); index++ {
			parameter := method.Type.In(index)
			if parameter.Kind() == reflect.Func {
				t.Errorf("CustomEditor.%s parameter %d remains executable before M7", methodName, index)
			}
		}
		lastInput := method.Type.In(method.Type.NumIn() - 1)
		if lastInput != handlerType {
			t.Errorf("CustomEditor.%s callback type = %v, want opaque %v", methodName, lastInput, handlerType)
		}
	}

	editor := &codingagent.CustomEditor{
		ActionHandlers: make(map[codingagent.AppKeybinding]codingagent.ExtensionHandler),
	}
	if err := editor.OnAction(codingagent.AppKeybindingExit, nil); !errors.Is(err, codingagent.ErrNotImplemented) {
		t.Fatalf("OnAction() error = %v, want ErrNotImplemented", err)
	}
	if len(editor.ActionHandlers) != 0 {
		t.Fatalf("OnAction() registered %d handlers before M7, want zero", len(editor.ActionHandlers))
	}
}

func TestPinnedCoreUIContractsUseConcreteCarriers(t *testing.T) {
	anyType := reflect.TypeOf((*any)(nil)).Elem()
	assistantMessageType := reflect.TypeOf(ai.AssistantMessage{})
	boolType := reflect.TypeOf(false)
	contextType := reflect.TypeOf((*context.Context)(nil)).Elem()
	errorType := reflect.TypeOf((*error)(nil)).Elem()
	intPointerType := reflect.TypeOf((*int)(nil))
	stringType := reflect.TypeOf("")
	stringPointerType := reflect.TypeOf((*string)(nil))
	stringSliceType := reflect.TypeOf([]string(nil))

	assertMethodType(t, (*codingagent.AssistantMessageComponent)(nil), "UpdateContent", reflect.FuncOf(
		[]reflect.Type{reflect.TypeOf((*codingagent.AssistantMessageComponent)(nil)), assistantMessageType, reflect.SliceOf(boolType)},
		[]reflect.Type{errorType},
		true,
	))
	assertMethodType(t, (*codingagent.BashExecutionComponent)(nil), "SetComplete", reflect.FuncOf(
		[]reflect.Type{
			reflect.TypeOf((*codingagent.BashExecutionComponent)(nil)),
			intPointerType,
			boolType,
			reflect.TypeOf((*codingagent.TruncationResult)(nil)),
			stringPointerType,
		},
		[]reflect.Type{errorType},
		false,
	))
	assertMethodType(t, (*codingagent.BorderedLoader)(nil), "Signal", reflect.FuncOf(
		[]reflect.Type{reflect.TypeOf((*codingagent.BorderedLoader)(nil))},
		[]reflect.Type{contextType, errorType},
		false,
	))

	resultType := reflect.TypeOf(codingagent.ToolExecutionResult{})
	wantResultFields := []struct {
		name   string
		typeOf reflect.Type
	}{
		{name: "Content", typeOf: reflect.TypeOf([]ai.ToolResultContent(nil))},
		{name: "Details", typeOf: reflect.TypeOf(ai.Optional[ai.JSONValue]{})},
		{name: "IsError", typeOf: boolType},
	}
	if resultType.NumField() != len(wantResultFields) {
		t.Fatalf("ToolExecutionResult has %d fields, want %d", resultType.NumField(), len(wantResultFields))
	}
	for index, wantField := range wantResultFields {
		field := resultType.Field(index)
		if field.Name != wantField.name || field.Type != wantField.typeOf {
			t.Errorf("ToolExecutionResult field %d = %s %v, want %s %v", index, field.Name, field.Type, wantField.name, wantField.typeOf)
		}
	}

	toolExecutionType := reflect.TypeOf((*codingagent.ToolExecutionComponent)(nil))
	assertMethodType(t, (*codingagent.ToolExecutionComponent)(nil), "UpdateArgs", reflect.FuncOf(
		[]reflect.Type{toolExecutionType, anyType}, []reflect.Type{errorType}, false,
	))
	assertMethodType(t, (*codingagent.ToolExecutionComponent)(nil), "UpdateResult", reflect.FuncOf(
		[]reflect.Type{toolExecutionType, resultType, reflect.SliceOf(boolType)}, []reflect.Type{errorType}, true,
	))

	dialogType := reflect.TypeOf((*codingagent.LoginDialogComponent)(nil))
	assertMethodType(t, (*codingagent.LoginDialogComponent)(nil), "Signal", reflect.FuncOf(
		[]reflect.Type{dialogType}, []reflect.Type{contextType, errorType}, false,
	))
	assertMethodType(t, (*codingagent.LoginDialogComponent)(nil), "ShowAuth", reflect.FuncOf(
		[]reflect.Type{dialogType, stringType, reflect.SliceOf(stringType)}, []reflect.Type{errorType}, true,
	))
	assertMethodType(t, (*codingagent.LoginDialogComponent)(nil), "ShowDetails", reflect.FuncOf(
		[]reflect.Type{dialogType, stringSliceType}, []reflect.Type{errorType}, false,
	))
	assertMethodType(t, (*codingagent.LoginDialogComponent)(nil), "ShowDeviceCode", reflect.FuncOf(
		[]reflect.Type{dialogType, reflect.TypeOf(ai.OAuthDeviceCodeInfo{})}, []reflect.Type{errorType}, false,
	))
	assertMethodType(t, (*codingagent.LoginDialogComponent)(nil), "ShowInfo", reflect.FuncOf(
		[]reflect.Type{dialogType, stringType, reflect.TypeOf([]ai.AuthInfoLink(nil)), reflect.SliceOf(boolType)}, []reflect.Type{errorType}, true,
	))
	for _, methodName := range []string{"ShowManualInput", "ShowPrompt"} {
		inputs := []reflect.Type{dialogType, stringType}
		variadic := false
		if methodName == "ShowPrompt" {
			inputs = append(inputs, reflect.SliceOf(stringType))
			variadic = true
		}
		assertMethodType(t, (*codingagent.LoginDialogComponent)(nil), methodName, reflect.FuncOf(
			inputs, []reflect.Type{stringType, errorType}, variadic,
		))
	}
	for _, methodName := range []string{"ShowProgress", "ShowWaiting"} {
		assertMethodType(t, (*codingagent.LoginDialogComponent)(nil), methodName, reflect.FuncOf(
			[]reflect.Type{dialogType, stringType}, []reflect.Type{errorType}, false,
		))
	}
}

func TestPinnedCoreUIOperationsRemainStructuredStubs(t *testing.T) {
	assertCodingAgentUIStub(t, "AssistantMessageComponent.UpdateContent", new(codingagent.AssistantMessageComponent).UpdateContent(ai.AssistantMessage{}))
	assertCodingAgentUIStub(t, "BashExecutionComponent.SetComplete", new(codingagent.BashExecutionComponent).SetComplete(nil, false, nil, nil))

	toolResult := codingagent.ToolExecutionResult{
		Content: []ai.ToolResultContent{ai.TextContent{Type: ai.ContentTypeText, Text: "partial output"}},
		Details: ai.Some[ai.JSONValue](map[string]any{"status": "pending"}),
		IsError: true,
	}
	wantToolResult := codingagent.ToolExecutionResult{
		Content: append([]ai.ToolResultContent(nil), toolResult.Content...),
		Details: ai.Some[ai.JSONValue](map[string]any{"status": "pending"}),
		IsError: true,
	}
	toolExecution := new(codingagent.ToolExecutionComponent)
	assertCodingAgentUIStub(t, "ToolExecutionComponent.UpdateResult", toolExecution.UpdateResult(toolResult))
	assertCodingAgentUIStub(t, "ToolExecutionComponent.UpdateResult", toolExecution.UpdateResult(toolResult, true))
	if !reflect.DeepEqual(toolResult, wantToolResult) {
		t.Errorf("UpdateResult() mutated input to %#v, want %#v", toolResult, wantToolResult)
	}

	dialog := new(codingagent.LoginDialogComponent)
	assertCodingAgentUIStub(t, "LoginDialogComponent.ShowAuth", dialog.ShowAuth("https://example.invalid"))
	assertCodingAgentUIStub(t, "LoginDialogComponent.ShowDetails", dialog.ShowDetails(nil))
	assertCodingAgentUIStub(t, "LoginDialogComponent.ShowDeviceCode", dialog.ShowDeviceCode(ai.OAuthDeviceCodeInfo{}))
	assertCodingAgentUIStub(t, "LoginDialogComponent.ShowInfo", dialog.ShowInfo("message", nil))
	manual, err := dialog.ShowManualInput("prompt")
	if manual != "" {
		t.Errorf("ShowManualInput() result = %q, want empty string", manual)
	}
	assertCodingAgentUIStub(t, "LoginDialogComponent.ShowManualInput", err)
	assertCodingAgentUIStub(t, "LoginDialogComponent.ShowProgress", dialog.ShowProgress("progress"))
	prompt, err := dialog.ShowPrompt("prompt")
	if prompt != "" {
		t.Errorf("ShowPrompt() result = %q, want empty string", prompt)
	}
	assertCodingAgentUIStub(t, "LoginDialogComponent.ShowPrompt", err)
	assertCodingAgentUIStub(t, "LoginDialogComponent.ShowWaiting", dialog.ShowWaiting("waiting"))
}

func TestSettingsCallbacksMatchPinnedCoreUITypes(t *testing.T) {
	callbacks := reflect.TypeOf(codingagent.SettingsCallbacks{})
	want := map[string]reflect.Type{
		"OnAutoCompactChange":            reflect.TypeOf((func(bool))(nil)),
		"OnAutoResizeImagesChange":       reflect.TypeOf((func(bool))(nil)),
		"OnAutocompleteMaxVisibleChange": reflect.TypeOf((func(int))(nil)),
		"OnBlockImagesChange":            reflect.TypeOf((func(bool))(nil)),
		"OnCancel":                       reflect.TypeOf((func())(nil)),
		"OnClearOnShrinkChange":          reflect.TypeOf((func(bool))(nil)),
		"OnCollapseChangelogChange":      reflect.TypeOf((func(bool))(nil)),
		"OnDefaultProjectTrustChange":    reflect.TypeOf((func(codingagent.DefaultProjectTrust))(nil)),
		"OnDoubleEscapeActionChange":     reflect.TypeOf((func(codingagent.DoubleEscapeAction))(nil)),
		"OnEditorPaddingXChange":         reflect.TypeOf((func(int))(nil)),
		"OnEnableInstallTelemetryChange": reflect.TypeOf((func(bool))(nil)),
		"OnEnableSkillCommandsChange":    reflect.TypeOf((func(bool))(nil)),
		"OnFollowUpModeChange":           reflect.TypeOf((func(agent.QueueMode))(nil)),
		"OnFullscreenExitOutputChange":   reflect.TypeOf((func(codingagent.FullscreenExitOutput))(nil)),
		"OnFullscreenScrollbarChange":    reflect.TypeOf((func(tui.ScrollViewScrollbar))(nil)),
		"OnHideThinkingBlockChange":      reflect.TypeOf((func(bool))(nil)),
		"OnHTTPIdleTimeoutMSChange":      reflect.TypeOf((func(int64))(nil)),
		"OnImageWidthCellsChange":        reflect.TypeOf((func(int))(nil)),
		"OnMermaidRenderingModeChange":   reflect.TypeOf((func(codingagent.MermaidRenderingMode))(nil)),
		"OnOutputPadChange":              reflect.TypeOf((func(int))(nil)),
		"OnQuietStartupChange":           reflect.TypeOf((func(bool))(nil)),
		"OnShowCacheMissNoticesChange":   reflect.TypeOf((func(bool))(nil)),
		"OnShowHardwareCursorChange":     reflect.TypeOf((func(bool))(nil)),
		"OnShowImagesChange":             reflect.TypeOf((func(bool))(nil)),
		"OnShowTerminalProgressChange":   reflect.TypeOf((func(bool))(nil)),
		"OnSteeringModeChange":           reflect.TypeOf((func(agent.QueueMode))(nil)),
		"OnThemeChange":                  reflect.TypeOf((func(string))(nil)),
		"OnThemePreview":                 reflect.TypeOf((func(string))(nil)),
		"OnThinkingLevelChange":          reflect.TypeOf((func(agent.ThinkingLevel))(nil)),
		"OnTransportChange":              reflect.TypeOf((func(ai.Transport))(nil)),
		"OnTreeFilterModeChange":         reflect.TypeOf((func(codingagent.TreeFilterMode))(nil)),
		"OnTUIModeChange":                reflect.TypeOf((func(tui.TUIMode))(nil)),
		"OnWarningsChange":               reflect.TypeOf((func(codingagent.WarningSettings))(nil)),
	}
	if callbacks.NumField() != len(want) {
		t.Errorf("SettingsCallbacks has %d fields, want %d", callbacks.NumField(), len(want))
	}
	for name, wantType := range want {
		field, ok := callbacks.FieldByName(name)
		if !ok {
			t.Errorf("SettingsCallbacks.%s is missing", name)
			continue
		}
		if field.Type != wantType {
			t.Errorf("SettingsCallbacks.%s type = %v, want %v", name, field.Type, wantType)
		}
	}
	if _, typo := callbacks.FieldByName("OnHTTPIDleTimeoutMSChange"); typo {
		t.Error("SettingsCallbacks retains duplicate OnHTTPIDleTimeoutMSChange typo")
	}

	config := reflect.TypeOf(codingagent.SettingsConfig{})
	if field, ok := config.FieldByName("HTTPIdleTimeoutMS"); !ok || field.Type != reflect.TypeOf(int64(0)) {
		t.Errorf("SettingsConfig.HTTPIdleTimeoutMS = %v, want int64", field.Type)
	}
	if _, typo := config.FieldByName("HTTPIDleTimeoutMS"); typo {
		t.Error("SettingsConfig retains duplicate HTTPIDleTimeoutMS typo")
	}
	wantConfigFields := map[string]reflect.Type{
		"DoubleEscapeAction":   reflect.TypeOf(codingagent.DoubleEscapeAction("")),
		"MermaidRenderingMode": reflect.TypeOf(codingagent.MermaidRenderingMode("")),
		"TreeFilterMode":       reflect.TypeOf(codingagent.TreeFilterMode("")),
		"Warnings":             reflect.TypeOf(codingagent.WarningSettings{}),
	}
	for name, wantType := range wantConfigFields {
		field, ok := config.FieldByName(name)
		if !ok {
			t.Errorf("SettingsConfig.%s is missing", name)
			continue
		}
		if field.Type != wantType {
			t.Errorf("SettingsConfig.%s type = %v, want %v", name, field.Type, wantType)
		}
	}
	if field, ok := config.FieldByName("OutputPad"); !ok || field.Type != reflect.TypeOf(0) {
		t.Errorf("SettingsConfig.OutputPad = %v, want int", field.Type)
	}

	warnings := reflect.TypeOf(codingagent.WarningSettings{})
	if warnings.NumField() != 1 {
		t.Fatalf("WarningSettings has %d fields, want 1", warnings.NumField())
	}
	warningField := warnings.Field(0)
	if warningField.Name != "AnthropicExtraUsage" || warningField.Type != reflect.TypeOf((*bool)(nil)) {
		t.Errorf("WarningSettings field = %s %v, want AnthropicExtraUsage *bool", warningField.Name, warningField.Type)
	}
}

func TestSettingsCarrierFiniteValues(t *testing.T) {
	assertFiniteStringValues(t,
		[]codingagent.MermaidRenderingMode{
			codingagent.MermaidRenderingModeOff,
			codingagent.MermaidRenderingModeFinal,
			codingagent.MermaidRenderingModeStreaming,
		},
		[]codingagent.MermaidRenderingMode{"off", "final", "streaming"},
	)
	assertFiniteStringValues(t,
		[]codingagent.DoubleEscapeAction{
			codingagent.DoubleEscapeActionFork,
			codingagent.DoubleEscapeActionTree,
			codingagent.DoubleEscapeActionNone,
		},
		[]codingagent.DoubleEscapeAction{"fork", "tree", "none"},
	)
	assertFiniteStringValues(t,
		[]codingagent.TreeFilterMode{
			codingagent.TreeFilterModeDefault,
			codingagent.TreeFilterModeNoTools,
			codingagent.TreeFilterModeUserOnly,
			codingagent.TreeFilterModeLabeledOnly,
			codingagent.TreeFilterModeAll,
		},
		[]codingagent.TreeFilterMode{"default", "no-tools", "user-only", "labeled-only", "all"},
	)
}

func assertFiniteStringValues[T ~string](t *testing.T, got, want []T) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("finite values = %q, want %q", got, want)
	}
}

func assertMethodType(t *testing.T, receiver any, name string, want reflect.Type) {
	t.Helper()
	method, ok := reflect.TypeOf(receiver).MethodByName(name)
	if !ok {
		t.Errorf("%v.%s is missing", reflect.TypeOf(receiver), name)
		return
	}
	if method.Type != want {
		t.Errorf("%v.%s type = %v, want %v", reflect.TypeOf(receiver), name, method.Type, want)
	}
}

func assertCodingAgentUIStub(t *testing.T, operation string, err error) {
	t.Helper()
	if !errors.Is(err, codingagent.ErrNotImplemented) {
		t.Errorf("%s error = %v, want ErrNotImplemented", operation, err)
		return
	}
	var unavailable *codingagent.NotImplementedError
	if !errors.As(err, &unavailable) {
		t.Errorf("%s error = %T, want *codingagent.NotImplementedError", operation, err)
		return
	}
	if unavailable.Module != "codingagent" || unavailable.Operation != operation {
		t.Errorf("%s error = %#v, want codingagent.%s", operation, unavailable, operation)
	}
}

func TestBorderedLoaderSignal(t *testing.T) {
	signal, err := new(codingagent.BorderedLoader).Signal()
	if signal != nil {
		t.Fatalf("Signal() = %v, want nil while unavailable", signal)
	}
	assertCodingAgentUIStub(t, "BorderedLoader.Signal", err)
}

func TestLoginDialogComponentSignal(t *testing.T) {
	signal, err := new(codingagent.LoginDialogComponent).Signal()
	if signal != nil {
		t.Fatalf("Signal() = %v, want nil while unavailable", signal)
	}
	assertCodingAgentUIStub(t, "LoginDialogComponent.Signal", err)
}

func TestTruncateToVisualLines(t *testing.T) {
	t.Run("surfaces deferred text wrapping", func(t *testing.T) {
		result, err := codingagent.TruncateToVisualLines("alpha beta", 2, 8)
		if !errors.Is(err, tui.ErrNotImplemented) {
			t.Fatalf("error = %v, want tui.ErrNotImplemented", err)
		}
		if result.SkippedCount != 0 || result.VisualLines != nil {
			t.Fatalf("result = %#v, want zero result alongside error", result)
		}
	})

	t.Run("empty text is an empty result", func(t *testing.T) {
		result, err := codingagent.TruncateToVisualLines("", 1, 1)
		if err != nil {
			t.Fatalf("error = %v, want nil", err)
		}
		if result.SkippedCount != 0 || result.VisualLines != nil {
			t.Fatalf("result = %#v, want zero result", result)
		}
	})

	t.Run("valid padding keeps one content column", func(t *testing.T) {
		_, err := codingagent.TruncateToVisualLines("x", 1, 5, 2)
		if !errors.Is(err, tui.ErrNotImplemented) {
			t.Fatalf("error = %v, want tui.ErrNotImplemented", err)
		}
	})

	validationCases := []struct {
		name string
		call func() (codingagent.VisualTruncateResult, error)
	}{
		{
			name: "non-positive maximum lines",
			call: func() (codingagent.VisualTruncateResult, error) {
				return codingagent.TruncateToVisualLines("x", 0, 1)
			},
		},
		{
			name: "non-positive width",
			call: func() (codingagent.VisualTruncateResult, error) {
				return codingagent.TruncateToVisualLines("x", 1, 0)
			},
		},
		{
			name: "negative padding",
			call: func() (codingagent.VisualTruncateResult, error) {
				return codingagent.TruncateToVisualLines("x", 1, 3, -1)
			},
		},
		{
			name: "padding consumes width",
			call: func() (codingagent.VisualTruncateResult, error) {
				return codingagent.TruncateToVisualLines("x", 1, 4, 2)
			},
		},
		{
			name: "multiple padding values",
			call: func() (codingagent.VisualTruncateResult, error) {
				return codingagent.TruncateToVisualLines("x", 1, 5, 1, 1)
			},
		},
	}
	for _, testCase := range validationCases {
		t.Run(testCase.name, func(t *testing.T) {
			result, err := testCase.call()
			if err == nil {
				t.Fatal("error = nil, want validation error")
			}
			if errors.Is(err, tui.ErrNotImplemented) {
				t.Fatalf("error = %v, want validation before deferred wrapping", err)
			}
			if result.SkippedCount != 0 || result.VisualLines != nil {
				t.Fatalf("result = %#v, want zero result alongside error", result)
			}
		})
	}
}

func TestThemeColorConstants(t *testing.T) {
	want := []codingagent.ThemeColor{
		"accent",
		"border",
		"borderAccent",
		"borderMuted",
		"success",
		"error",
		"warning",
		"muted",
		"dim",
		"text",
		"thinkingText",
		"userMessageText",
		"customMessageText",
		"customMessageLabel",
		"toolTitle",
		"toolOutput",
		"mdHeading",
		"mdLink",
		"mdLinkUrl",
		"mdCode",
		"mdCodeBlock",
		"mdCodeBlockBorder",
		"mdQuote",
		"mdQuoteBorder",
		"mdHr",
		"mdListBullet",
		"toolDiffAdded",
		"toolDiffRemoved",
		"toolDiffContext",
		"syntaxComment",
		"syntaxKeyword",
		"syntaxFunction",
		"syntaxVariable",
		"syntaxString",
		"syntaxNumber",
		"syntaxType",
		"syntaxOperator",
		"syntaxPunctuation",
		"thinkingOff",
		"thinkingMinimal",
		"thinkingLow",
		"thinkingMedium",
		"thinkingHigh",
		"thinkingXhigh",
		"thinkingMax",
		"bashMode",
	}
	got := []codingagent.ThemeColor{
		codingagent.ThemeColorAccent,
		codingagent.ThemeColorBorder,
		codingagent.ThemeColorBorderAccent,
		codingagent.ThemeColorBorderMuted,
		codingagent.ThemeColorSuccess,
		codingagent.ThemeColorError,
		codingagent.ThemeColorWarning,
		codingagent.ThemeColorMuted,
		codingagent.ThemeColorDim,
		codingagent.ThemeColorText,
		codingagent.ThemeColorThinkingText,
		codingagent.ThemeColorUserMessageText,
		codingagent.ThemeColorCustomMessageText,
		codingagent.ThemeColorCustomMessageLabel,
		codingagent.ThemeColorToolTitle,
		codingagent.ThemeColorToolOutput,
		codingagent.ThemeColorMDHeading,
		codingagent.ThemeColorMDLink,
		codingagent.ThemeColorMDLinkURL,
		codingagent.ThemeColorMDCode,
		codingagent.ThemeColorMDCodeBlock,
		codingagent.ThemeColorMDCodeBlockBorder,
		codingagent.ThemeColorMDQuote,
		codingagent.ThemeColorMDQuoteBorder,
		codingagent.ThemeColorMDHR,
		codingagent.ThemeColorMDListBullet,
		codingagent.ThemeColorToolDiffAdded,
		codingagent.ThemeColorToolDiffRemoved,
		codingagent.ThemeColorToolDiffContext,
		codingagent.ThemeColorSyntaxComment,
		codingagent.ThemeColorSyntaxKeyword,
		codingagent.ThemeColorSyntaxFunction,
		codingagent.ThemeColorSyntaxVariable,
		codingagent.ThemeColorSyntaxString,
		codingagent.ThemeColorSyntaxNumber,
		codingagent.ThemeColorSyntaxType,
		codingagent.ThemeColorSyntaxOperator,
		codingagent.ThemeColorSyntaxPunctuation,
		codingagent.ThemeColorThinkingOff,
		codingagent.ThemeColorThinkingMinimal,
		codingagent.ThemeColorThinkingLow,
		codingagent.ThemeColorThinkingMedium,
		codingagent.ThemeColorThinkingHigh,
		codingagent.ThemeColorThinkingXHigh,
		codingagent.ThemeColorThinkingMax,
		codingagent.ThemeColorBashMode,
	}
	assertThemeValues(t, got, want)
}

func TestThemeBGConstants(t *testing.T) {
	want := []codingagent.ThemeBG{
		"selectedBg",
		"scrollbarThumb",
		"userMessageBg",
		"customMessageBg",
		"toolPendingBg",
		"toolSuccessBg",
		"toolErrorBg",
	}
	got := []codingagent.ThemeBG{
		codingagent.ThemeBGSelected,
		codingagent.ThemeBGScrollbarThumb,
		codingagent.ThemeBGUserMessage,
		codingagent.ThemeBGCustomMessage,
		codingagent.ThemeBGToolPending,
		codingagent.ThemeBGToolSuccess,
		codingagent.ThemeBGToolError,
	}
	assertThemeValues(t, got, want)
}

func assertThemeValues[T ~string](t *testing.T, got, want []T) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d constants, want %d", len(got), len(want))
	}
	seen := make(map[T]bool, len(got))
	for index := range want {
		if got[index] != want[index] {
			t.Errorf("constant %d = %q, want %q", index, got[index], want[index])
		}
		if seen[got[index]] {
			t.Errorf("constant %d duplicates value %q", index, got[index])
		}
		seen[got[index]] = true
	}
}
