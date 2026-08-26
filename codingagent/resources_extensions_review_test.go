package codingagent_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/nankedr/pig/codingagent"
)

var _ codingagent.ResourceLoader = (*codingagent.DefaultResourceLoader)(nil)

func TestResourceLoadResultsUseCodingAgentCarriers(t *testing.T) {
	t.Run("collision sources", func(t *testing.T) {
		carrier := reflect.TypeOf(codingagent.ResourceCollision{})
		want := reflect.TypeOf((*string)(nil))
		for _, fieldName := range []string{"WinnerSource", "LoserSource"} {
			field, ok := carrier.FieldByName(fieldName)
			if !ok {
				t.Errorf("ResourceCollision.%s is missing", fieldName)
				continue
			}
			if field.Type != want {
				t.Errorf("ResourceCollision.%s type = %v, want %v", fieldName, field.Type, want)
			}
		}
	})

	t.Run("extensions", func(t *testing.T) {
		canonical := reflect.TypeOf(codingagent.LoadExtensionsResult{})
		wantFields := map[string]reflect.Type{
			"Extensions": reflect.TypeOf([]codingagent.Extension(nil)),
			"Runtime":    reflect.TypeOf((*codingagent.ExtensionRuntime)(nil)),
		}
		for fieldName, want := range wantFields {
			field, ok := canonical.FieldByName(fieldName)
			if !ok {
				t.Errorf("LoadExtensionsResult.%s is missing", fieldName)
				continue
			}
			if field.Type != want {
				t.Errorf("LoadExtensionsResult.%s type = %v, want %v", fieldName, field.Type, want)
			}
		}
		errorsField, ok := canonical.FieldByName("Errors")
		if !ok {
			t.Fatal("LoadExtensionsResult.Errors is missing")
		}
		if errorsField.Type.Kind() != reflect.Slice || errorsField.Type.Elem().Kind() != reflect.Struct {
			t.Fatalf("LoadExtensionsResult.Errors type = %v, want slice of anonymous records", errorsField.Type)
		}

		loaderMethod, ok := reflect.TypeOf((*codingagent.ResourceLoader)(nil)).Elem().MethodByName("GetExtensions")
		if !ok {
			t.Fatal("ResourceLoader.GetExtensions is missing")
		}
		if got := loaderMethod.Type.Out(0); got != canonical {
			t.Errorf("ResourceLoader.GetExtensions result = %v, want %v", got, canonical)
		}

		trustMethod := reflect.TypeOf((*codingagent.DefaultResourceLoader).LoadProjectTrustExtensions)
		if got := trustMethod.Out(0); got != canonical {
			t.Errorf("DefaultResourceLoader.LoadProjectTrustExtensions result = %v, want %v", got, canonical)
		}
	})

	t.Run("skills", func(t *testing.T) {
		field, ok := reflect.TypeOf(codingagent.SkillLoadResult{}).FieldByName("Skills")
		if !ok {
			t.Fatal("SkillLoadResult.Skills is missing")
		}
		want := reflect.TypeOf([]codingagent.Skill(nil))
		if field.Type != want {
			t.Fatalf("SkillLoadResult.Skills type = %v, want %v", field.Type, want)
		}
	})

	t.Run("prompts", func(t *testing.T) {
		field, ok := reflect.TypeOf(codingagent.PromptTemplateLoadResult{}).FieldByName("Prompts")
		if !ok {
			t.Fatal("PromptTemplateLoadResult.Prompts is missing")
		}
		want := reflect.TypeOf([]codingagent.PromptTemplate(nil))
		if field.Type != want {
			t.Fatalf("PromptTemplateLoadResult.Prompts type = %v, want %v", field.Type, want)
		}
	})

	t.Run("themes", func(t *testing.T) {
		field, ok := reflect.TypeOf(codingagent.ThemeLoadResult{}).FieldByName("Themes")
		if !ok {
			t.Fatal("ThemeLoadResult.Themes is missing")
		}
		want := reflect.TypeOf([]*codingagent.Theme(nil))
		if field.Type != want {
			t.Fatalf("ThemeLoadResult.Themes type = %v, want %v", field.Type, want)
		}
	})
}

func TestResourceLoaderCallbackSlotsRemainOpaqueUntilM7(t *testing.T) {
	want := reflect.TypeOf((*codingagent.ExtensionHandler)(nil)).Elem()
	fieldsByCarrier := map[reflect.Type][]string{
		reflect.TypeOf(codingagent.ResourceLoaderReloadOptions{}): {
			"ResolveProjectTrust",
		},
		reflect.TypeOf(codingagent.DefaultResourceLoaderOptions{}): {
			"ExtensionsOverride", "SkillsOverride", "PromptsOverride",
			"ThemesOverride", "AgentsFilesOverride", "SystemPromptOverride",
			"AppendSystemPromptOverride",
		},
	}

	for carrier, fieldNames := range fieldsByCarrier {
		for _, fieldName := range fieldNames {
			field, ok := carrier.FieldByName(fieldName)
			if !ok {
				t.Errorf("%s.%s is missing", carrier.Name(), fieldName)
				continue
			}
			if field.Type != want {
				t.Errorf("%s.%s type = %v, want opaque %v", carrier.Name(), fieldName, field.Type, want)
			}
			if field.Type.Kind() == reflect.Func {
				t.Errorf("%s.%s remains callable before the M7 ABI decision", carrier.Name(), fieldName)
			}
		}
	}
}

func TestDefaultResourceLoaderUnavailableBoundary(t *testing.T) {
	constructor := reflect.TypeOf(codingagent.NewDefaultResourceLoader)
	if constructor.NumIn() != 1 || constructor.In(0).Name() != "DefaultResourceLoaderOptions" {
		t.Errorf("NewDefaultResourceLoader input = %v, want DefaultResourceLoaderOptions", constructor)
	}
	if constructor.NumOut() != 2 || !constructor.Out(1).Implements(reflect.TypeOf((*error)(nil)).Elem()) {
		t.Errorf("NewDefaultResourceLoader outputs = %v, want (*DefaultResourceLoader, error)", constructor)
	}

	root := t.TempDir()
	cwd := filepath.Join(root, "missing-project")
	agentDir := filepath.Join(root, "missing-agent-dir")
	created, err := codingagent.NewDefaultResourceLoader(codingagent.DefaultResourceLoaderOptions{
		CWD:      cwd,
		AgentDir: agentDir,
		EventBus: codingagent.EventBus{
			Emit: nil,
			On:   nil,
		},
		ExtensionsOverride:         nil,
		SkillsOverride:             nil,
		PromptsOverride:            nil,
		ThemesOverride:             nil,
		AgentsFilesOverride:        nil,
		SystemPromptOverride:       nil,
		AppendSystemPromptOverride: nil,
	})
	assertReviewNotImplemented(t, err, "NewDefaultResourceLoader")
	if created != nil {
		t.Fatalf("NewDefaultResourceLoader result = %#v, want nil", created)
	}
	for _, path := range []string{cwd, agentDir} {
		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			t.Fatalf("NewDefaultResourceLoader touched %s: Stat error = %v", path, statErr)
		}
	}

	loader := &codingagent.DefaultResourceLoader{}
	for _, method := range []string{
		"GetExtensions",
		"GetSkills",
		"GetPrompts",
		"GetThemes",
		"GetAgentsFiles",
		"GetSystemPrompt",
		"GetSystemPromptSource",
		"GetAppendSystemPrompt",
		"GetAppendSystemPromptSources",
	} {
		t.Run(method, func(t *testing.T) {
			assertReflectiveUnavailableQuery(t, loader, method, "DefaultResourceLoader."+method)
		})
	}
	assertReviewNotImplemented(t, loader.Reload(context.Background(), codingagent.ResourceLoaderReloadOptions{
		ResolveProjectTrust: nil,
	}), "DefaultResourceLoader.Reload")
}

func TestExtensionRuntimeBoundaryRemainsOpaque(t *testing.T) {
	for name, carrier := range map[string]reflect.Type{
		"ExtensionRunner":  reflect.TypeOf(codingagent.ExtensionRunner{}),
		"ExtensionRuntime": reflect.TypeOf(codingagent.ExtensionRuntime{}),
	} {
		if carrier.NumField() != 0 {
			t.Errorf("%s has %d fields, want an opaque carrier", name, carrier.NumField())
		}
		if methods := reflect.PointerTo(carrier).NumMethod(); methods != 0 {
			t.Errorf("%s exposes %d executable methods before M7", name, methods)
		}
	}
}

func TestExtensionFiniteUnionValues(t *testing.T) {
	appKeybindings := []codingagent.AppKeybinding{
		codingagent.AppKeybindingInterrupt,
		codingagent.AppKeybindingClear,
		codingagent.AppKeybindingExit,
		codingagent.AppKeybindingSuspend,
		codingagent.AppKeybindingThinkingCycle,
		codingagent.AppKeybindingModelCycleForward,
		codingagent.AppKeybindingModelCycleBackward,
		codingagent.AppKeybindingModelSelect,
		codingagent.AppKeybindingToolsExpand,
		codingagent.AppKeybindingThinkingToggle,
		codingagent.AppKeybindingSessionToggleNamedFilter,
		codingagent.AppKeybindingEditorExternal,
		codingagent.AppKeybindingMessageCopy,
		codingagent.AppKeybindingMessageFollowUp,
		codingagent.AppKeybindingMessageDequeue,
		codingagent.AppKeybindingClipboardPasteImage,
		codingagent.AppKeybindingSessionNew,
		codingagent.AppKeybindingSessionTree,
		codingagent.AppKeybindingSessionFork,
		codingagent.AppKeybindingSessionResume,
		codingagent.AppKeybindingTreeFoldOrUp,
		codingagent.AppKeybindingTreeUnfoldOrDown,
		codingagent.AppKeybindingTreeEditLabel,
		codingagent.AppKeybindingTreeToggleLabelTimestamp,
		codingagent.AppKeybindingSessionTogglePath,
		codingagent.AppKeybindingSessionToggleSort,
		codingagent.AppKeybindingSessionRename,
		codingagent.AppKeybindingSessionDelete,
		codingagent.AppKeybindingSessionDeleteNoninvasive,
		codingagent.AppKeybindingModelsSave,
		codingagent.AppKeybindingModelsEnableAll,
		codingagent.AppKeybindingModelsClearAll,
		codingagent.AppKeybindingModelsToggleProvider,
		codingagent.AppKeybindingModelsReorderUp,
		codingagent.AppKeybindingModelsReorderDown,
		codingagent.AppKeybindingTreeFilterDefault,
		codingagent.AppKeybindingTreeFilterNoTools,
		codingagent.AppKeybindingTreeFilterUserOnly,
		codingagent.AppKeybindingTreeFilterLabeledOnly,
		codingagent.AppKeybindingTreeFilterAll,
		codingagent.AppKeybindingTreeFilterCycleForward,
		codingagent.AppKeybindingTreeFilterCycleBackward,
	}
	wantAppKeybindings := []codingagent.AppKeybinding{
		"app.interrupt", "app.clear", "app.exit", "app.suspend",
		"app.thinking.cycle", "app.model.cycleForward", "app.model.cycleBackward",
		"app.model.select", "app.tools.expand", "app.thinking.toggle",
		"app.session.toggleNamedFilter", "app.editor.external", "app.message.copy",
		"app.message.followUp", "app.message.dequeue", "app.clipboard.pasteImage",
		"app.session.new", "app.session.tree", "app.session.fork", "app.session.resume",
		"app.tree.foldOrUp", "app.tree.unfoldOrDown", "app.tree.editLabel",
		"app.tree.toggleLabelTimestamp", "app.session.togglePath", "app.session.toggleSort",
		"app.session.rename", "app.session.delete", "app.session.deleteNoninvasive",
		"app.models.save", "app.models.enableAll", "app.models.clearAll",
		"app.models.toggleProvider", "app.models.reorderUp", "app.models.reorderDown",
		"app.tree.filter.default", "app.tree.filter.noTools", "app.tree.filter.userOnly",
		"app.tree.filter.labeledOnly", "app.tree.filter.all",
		"app.tree.filter.cycleForward", "app.tree.filter.cycleBackward",
	}
	if !reflect.DeepEqual(appKeybindings, wantAppKeybindings) {
		t.Fatalf("AppKeybinding values = %#v, want %#v", appKeybindings, wantAppKeybindings)
	}

	if got, want := []codingagent.WidgetPlacement{
		codingagent.WidgetPlacementAboveEditor,
		codingagent.WidgetPlacementBelowEditor,
	}, []codingagent.WidgetPlacement{"aboveEditor", "belowEditor"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("WidgetPlacement values = %#v, want %#v", got, want)
	}
	if got, want := []codingagent.ProjectTrustEventDecision{
		codingagent.ProjectTrustEventDecisionYes,
		codingagent.ProjectTrustEventDecisionNo,
		codingagent.ProjectTrustEventDecisionUndecided,
	}, []codingagent.ProjectTrustEventDecision{"yes", "no", "undecided"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("ProjectTrustEventDecision values = %#v, want %#v", got, want)
	}
	if got, want := []codingagent.InputSource{
		codingagent.InputSourceInteractive,
		codingagent.InputSourceRPC,
		codingagent.InputSourceExtension,
	}, []codingagent.InputSource{"interactive", "rpc", "extension"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("InputSource values = %#v, want %#v", got, want)
	}
}

func assertReflectiveUnavailableQuery(t *testing.T, receiver any, method, operation string) {
	t.Helper()
	call := reflect.ValueOf(receiver).MethodByName(method)
	if !call.IsValid() {
		t.Fatalf("%T.%s is missing", receiver, method)
	}
	arguments := make([]reflect.Value, call.Type().NumIn())
	for index := range arguments {
		arguments[index] = reflect.Zero(call.Type().In(index))
	}
	outputs := call.Call(arguments)
	if len(outputs) < 2 {
		t.Fatalf("%s returned %d values, want a trailing structured error", method, len(outputs))
	}
	errValue := outputs[len(outputs)-1]
	if !errValue.Type().Implements(reflect.TypeOf((*error)(nil)).Elem()) {
		t.Fatalf("%s final result type = %v, want error", method, errValue.Type())
	}
	err, _ := errValue.Interface().(error)
	assertReviewNotImplemented(t, err, operation)
	for _, output := range outputs[:len(outputs)-1] {
		if !output.IsZero() {
			t.Errorf("%s result = %#v, want zero value with ErrNotImplemented", method, output.Interface())
		}
	}
}

func assertReviewNotImplemented(t *testing.T, err error, operation string) {
	t.Helper()
	if !errors.Is(err, codingagent.ErrNotImplemented) {
		t.Fatalf("%s error = %v, want ErrNotImplemented", operation, err)
	}
	var unavailable *codingagent.NotImplementedError
	if !errors.As(err, &unavailable) {
		t.Fatalf("%s error = %T, want *codingagent.NotImplementedError", operation, err)
	}
	if unavailable.Module != "codingagent" || unavailable.Operation != operation {
		t.Fatalf("NotImplementedError = %#v, want codingagent.%s", unavailable, operation)
	}
}
