package codingagent_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/tui"
)

type inheritedContainerSurface interface {
	tui.Component
	Children() []tui.Component
	AddChild(tui.Component)
	RemoveChild(tui.Component)
	Clear()
}

type inheritedBoxSurface interface {
	tui.Component
	AddChild(tui.Component) error
	RemoveChild(tui.Component) error
	Clear() error
	SetBGFunc(tui.TextStyleFunc) error
}

type inheritedEditorSurface interface {
	tui.EditorComponent
	tui.EditorSubmitSetter
	tui.EditorChangeSetter
	tui.EditorBorderColorSetter
	tui.EditorHistoryComponent
	tui.EditorInsertionComponent
	tui.EditorExpandedTextComponent
	tui.AutocompleteEditorComponent
	tui.PaddedEditorComponent
	tui.AutocompleteSizeEditorComponent
	SetFocusState(bool)
	GetAutocompleteMaxVisible() int
	GetCursor() tui.EditorCursor
	GetLines() []string
	GetPaddingX() int
	IsShowingAutocomplete() bool
}

var (
	_ inheritedContainerSurface = (*codingagent.AssistantMessageComponent)(nil)
	_ inheritedContainerSurface = (*codingagent.BashExecutionComponent)(nil)
	_ inheritedContainerSurface = (*codingagent.BorderedLoader)(nil)
	_ inheritedContainerSurface = (*codingagent.CustomMessageComponent)(nil)
	_ inheritedContainerSurface = (*codingagent.ExtensionEditorComponent)(nil)
	_ inheritedContainerSurface = (*codingagent.ExtensionInputComponent)(nil)
	_ inheritedContainerSurface = (*codingagent.ExtensionSelectorComponent)(nil)
	_ inheritedContainerSurface = (*codingagent.LoginDialogComponent)(nil)
	_ inheritedContainerSurface = (*codingagent.ModelSelectorComponent)(nil)
	_ inheritedContainerSurface = (*codingagent.OAuthSelectorComponent)(nil)
	_ inheritedContainerSurface = (*codingagent.SessionSelectorComponent)(nil)
	_ inheritedContainerSurface = (*codingagent.SettingsSelectorComponent)(nil)
	_ inheritedContainerSurface = (*codingagent.ShowImagesSelectorComponent)(nil)
	_ inheritedContainerSurface = (*codingagent.ThemeSelectorComponent)(nil)
	_ inheritedContainerSurface = (*codingagent.ThinkingSelectorComponent)(nil)
	_ inheritedContainerSurface = (*codingagent.ToolExecutionComponent)(nil)
	_ inheritedContainerSurface = (*codingagent.TreeSelectorComponent)(nil)
	_ inheritedContainerSurface = (*codingagent.UserMessageSelectorComponent)(nil)
	_ inheritedContainerSurface = (*codingagent.UserMessageComponent)(nil)

	_ inheritedBoxSurface = (*codingagent.BranchSummaryMessageComponent)(nil)
	_ inheritedBoxSurface = (*codingagent.CompactionSummaryMessageComponent)(nil)
	_ inheritedBoxSurface = (*codingagent.SkillInvocationMessageComponent)(nil)

	_ inheritedEditorSurface = (*codingagent.CustomEditor)(nil)
)

func TestUIComponentsEmbedInheritedCarriersByValue(t *testing.T) {
	containerTypes := []reflect.Type{
		reflect.TypeOf(codingagent.AssistantMessageComponent{}),
		reflect.TypeOf(codingagent.BashExecutionComponent{}),
		reflect.TypeOf(codingagent.BorderedLoader{}),
		reflect.TypeOf(codingagent.CustomMessageComponent{}),
		reflect.TypeOf(codingagent.ExtensionEditorComponent{}),
		reflect.TypeOf(codingagent.ExtensionInputComponent{}),
		reflect.TypeOf(codingagent.ExtensionSelectorComponent{}),
		reflect.TypeOf(codingagent.LoginDialogComponent{}),
		reflect.TypeOf(codingagent.ModelSelectorComponent{}),
		reflect.TypeOf(codingagent.OAuthSelectorComponent{}),
		reflect.TypeOf(codingagent.SessionSelectorComponent{}),
		reflect.TypeOf(codingagent.SettingsSelectorComponent{}),
		reflect.TypeOf(codingagent.ShowImagesSelectorComponent{}),
		reflect.TypeOf(codingagent.ThemeSelectorComponent{}),
		reflect.TypeOf(codingagent.ThinkingSelectorComponent{}),
		reflect.TypeOf(codingagent.ToolExecutionComponent{}),
		reflect.TypeOf(codingagent.TreeSelectorComponent{}),
		reflect.TypeOf(codingagent.UserMessageSelectorComponent{}),
		reflect.TypeOf(codingagent.UserMessageComponent{}),
	}
	for _, componentType := range containerTypes {
		assertAnonymousValueCarrier(t, componentType, "Container", reflect.TypeOf(tui.Container{}))
	}

	boxTypes := []reflect.Type{
		reflect.TypeOf(codingagent.BranchSummaryMessageComponent{}),
		reflect.TypeOf(codingagent.CompactionSummaryMessageComponent{}),
		reflect.TypeOf(codingagent.SkillInvocationMessageComponent{}),
	}
	for _, componentType := range boxTypes {
		assertAnonymousValueCarrier(t, componentType, "Box", reflect.TypeOf(tui.Box{}))
	}

	editorType := reflect.TypeOf(codingagent.CustomEditor{})
	assertAnonymousValueCarrier(t, editorType, "Editor", reflect.TypeOf(tui.Editor{}))
	if _, duplicate := editorType.FieldByName("editor"); duplicate {
		t.Error("CustomEditor retains a duplicate private editor field")
	}

	assertPointerOnlyMethod(t, reflect.TypeOf(codingagent.AssistantMessageComponent{}), "AddChild", reflect.TypeOf((func(*codingagent.AssistantMessageComponent, tui.Component))(nil)))
	assertPointerOnlyMethod(t, reflect.TypeOf(codingagent.BranchSummaryMessageComponent{}), "AddChild", reflect.TypeOf((func(*codingagent.BranchSummaryMessageComponent, tui.Component) error)(nil)))
	assertPointerOnlyMethod(t, editorType, "GetLines", reflect.TypeOf((func(*codingagent.CustomEditor) []string)(nil)))
}

func TestContainerDerivedUIComponentsPromoteLocalComposition(t *testing.T) {
	components := []struct {
		name      string
		component inheritedContainerSurface
	}{
		{name: "AssistantMessageComponent", component: new(codingagent.AssistantMessageComponent)},
		{name: "BashExecutionComponent", component: new(codingagent.BashExecutionComponent)},
		{name: "BorderedLoader", component: new(codingagent.BorderedLoader)},
		{name: "CustomMessageComponent", component: new(codingagent.CustomMessageComponent)},
		{name: "ExtensionEditorComponent", component: new(codingagent.ExtensionEditorComponent)},
		{name: "ExtensionInputComponent", component: new(codingagent.ExtensionInputComponent)},
		{name: "ExtensionSelectorComponent", component: new(codingagent.ExtensionSelectorComponent)},
		{name: "LoginDialogComponent", component: new(codingagent.LoginDialogComponent)},
		{name: "ModelSelectorComponent", component: new(codingagent.ModelSelectorComponent)},
		{name: "OAuthSelectorComponent", component: new(codingagent.OAuthSelectorComponent)},
		{name: "SessionSelectorComponent", component: new(codingagent.SessionSelectorComponent)},
		{name: "SettingsSelectorComponent", component: new(codingagent.SettingsSelectorComponent)},
		{name: "ShowImagesSelectorComponent", component: new(codingagent.ShowImagesSelectorComponent)},
		{name: "ThemeSelectorComponent", component: new(codingagent.ThemeSelectorComponent)},
		{name: "ThinkingSelectorComponent", component: new(codingagent.ThinkingSelectorComponent)},
		{name: "ToolExecutionComponent", component: new(codingagent.ToolExecutionComponent)},
		{name: "TreeSelectorComponent", component: new(codingagent.TreeSelectorComponent)},
		{name: "UserMessageSelectorComponent", component: new(codingagent.UserMessageSelectorComponent)},
		{name: "UserMessageComponent", component: new(codingagent.UserMessageComponent)},
	}

	child := &inheritanceReviewComponent{}
	for _, testCase := range components {
		t.Run(testCase.name, func(t *testing.T) {
			if children := testCase.component.Children(); children != nil {
				t.Fatalf("zero-value Children() = %#v, want nil", children)
			}
			testCase.component.AddChild(child)
			children := testCase.component.Children()
			if len(children) != 1 || children[0] != child {
				t.Fatalf("Children() after AddChild = %#v, want [%T]", children, child)
			}
			testCase.component.RemoveChild(child)
			if children := testCase.component.Children(); len(children) != 0 {
				t.Fatalf("Children() after RemoveChild = %#v, want empty", children)
			}
			testCase.component.AddChild(child)
			testCase.component.Clear()
			if children := testCase.component.Children(); children != nil {
				t.Fatalf("Children() after Clear = %#v, want nil", children)
			}
		})
	}
}

func TestInheritedUIOverrideBehavior(t *testing.T) {
	containerCases := []struct {
		name               string
		component          tui.Component
		invalidateOverride bool
		renderOverride     bool
	}{
		{name: "AssistantMessageComponent", component: new(codingagent.AssistantMessageComponent), invalidateOverride: true, renderOverride: true},
		{name: "BashExecutionComponent", component: new(codingagent.BashExecutionComponent), invalidateOverride: true},
		{name: "BorderedLoader", component: new(codingagent.BorderedLoader)},
		{name: "CustomMessageComponent", component: new(codingagent.CustomMessageComponent), invalidateOverride: true},
		{name: "ExtensionEditorComponent", component: new(codingagent.ExtensionEditorComponent)},
		{name: "ExtensionInputComponent", component: new(codingagent.ExtensionInputComponent)},
		{name: "ExtensionSelectorComponent", component: new(codingagent.ExtensionSelectorComponent)},
		{name: "LoginDialogComponent", component: new(codingagent.LoginDialogComponent)},
		{name: "ModelSelectorComponent", component: new(codingagent.ModelSelectorComponent)},
		{name: "OAuthSelectorComponent", component: new(codingagent.OAuthSelectorComponent)},
		{name: "SessionSelectorComponent", component: new(codingagent.SessionSelectorComponent)},
		{name: "SettingsSelectorComponent", component: new(codingagent.SettingsSelectorComponent)},
		{name: "ShowImagesSelectorComponent", component: new(codingagent.ShowImagesSelectorComponent)},
		{name: "ThemeSelectorComponent", component: new(codingagent.ThemeSelectorComponent)},
		{name: "ThinkingSelectorComponent", component: new(codingagent.ThinkingSelectorComponent)},
		{name: "ToolExecutionComponent", component: new(codingagent.ToolExecutionComponent), invalidateOverride: true, renderOverride: true},
		{name: "TreeSelectorComponent", component: new(codingagent.TreeSelectorComponent)},
		{name: "UserMessageSelectorComponent", component: new(codingagent.UserMessageSelectorComponent)},
		{name: "UserMessageComponent", component: new(codingagent.UserMessageComponent), renderOverride: true},
	}

	for _, testCase := range containerCases {
		t.Run(testCase.name, func(t *testing.T) {
			err := testCase.component.Invalidate()
			if testCase.invalidateOverride {
				assertInheritanceStub(t, err, "codingagent", testCase.name+".Invalidate")
			} else if err != nil {
				t.Fatalf("inherited Invalidate() error = %v, want nil", err)
			}

			lines, err := testCase.component.Render(80)
			if lines != nil {
				t.Fatalf("Render() lines = %#v, want nil", lines)
			}
			if testCase.renderOverride {
				assertInheritanceStub(t, err, "codingagent", testCase.name+".Render")
			} else if err != nil {
				t.Fatalf("inherited Render() error = %v, want nil", err)
			}
		})
	}

	boxCases := []struct {
		name      string
		component inheritedBoxSurface
	}{
		{name: "BranchSummaryMessageComponent", component: new(codingagent.BranchSummaryMessageComponent)},
		{name: "CompactionSummaryMessageComponent", component: new(codingagent.CompactionSummaryMessageComponent)},
		{name: "SkillInvocationMessageComponent", component: new(codingagent.SkillInvocationMessageComponent)},
	}
	for _, testCase := range boxCases {
		t.Run(testCase.name, func(t *testing.T) {
			assertInheritanceStub(t, testCase.component.Invalidate(), "codingagent", testCase.name+".Invalidate")
			assertInheritanceStub(t, testCase.component.AddChild(nil), "tui", "Box.addChild")
			lines, err := testCase.component.Render(80)
			if lines != nil {
				t.Fatalf("Render() lines = %#v, want nil", lines)
			}
			assertInheritanceStub(t, err, "tui", "Box.render")
		})
	}
}

func TestCustomEditorPromotesEditorStateAndCapabilities(t *testing.T) {
	editor := new(codingagent.CustomEditor)
	if got := editor.GetLines(); !reflect.DeepEqual(got, []string{""}) {
		t.Fatalf("zero-value GetLines() = %#v, want one empty line", got)
	}
	if err := editor.SetText("first\nsecond"); err != nil {
		t.Fatalf("SetText() error = %v, want nil", err)
	}
	if got := editor.GetText(); got != "first\nsecond" {
		t.Fatalf("GetText() = %q, want %q", got, "first\nsecond")
	}
	if got := editor.GetLines(); !reflect.DeepEqual(got, []string{"first", "second"}) {
		t.Fatalf("GetLines() = %#v, want split editor text", got)
	}

	called := false
	editor.SetOnSubmit(func(string) { called = true })
	editor.SetFocusState(true)
	if called {
		t.Fatal("SetOnSubmit invoked the callback")
	}
	if !editor.Focused || editor.OnSubmit == nil {
		t.Fatalf("promoted editor state = Focused %t, OnSubmit nil %t", editor.Focused, editor.OnSubmit == nil)
	}
	if err := editor.Invalidate(); err != nil {
		t.Fatalf("inherited Invalidate() error = %v, want nil", err)
	}
	lines, err := editor.Render(80)
	if lines != nil {
		t.Fatalf("Render() lines = %#v, want nil", lines)
	}
	assertInheritanceStub(t, err, "tui", "Editor.render")
	assertInheritanceStub(t, editor.AddToHistory("history"), "tui", "Editor.addToHistory")
	assertInheritanceStub(t, editor.HandleInput("x"), "codingagent", "CustomEditor.HandleInput")
}

type inheritanceReviewComponent struct{}

func (*inheritanceReviewComponent) Invalidate() error            { return nil }
func (*inheritanceReviewComponent) Render(int) ([]string, error) { return []string{"child"}, nil }

func assertAnonymousValueCarrier(t *testing.T, outer reflect.Type, fieldName string, want reflect.Type) {
	t.Helper()
	field, ok := outer.FieldByName(fieldName)
	if !ok {
		t.Errorf("%s.%s is missing", outer, fieldName)
		return
	}
	if !field.Anonymous || field.Type != want || len(field.Index) != 1 {
		t.Errorf("%s.%s = anonymous %t, type %v, index %v; want direct anonymous value %v", outer, fieldName, field.Anonymous, field.Type, field.Index, want)
	}
}

func assertPointerOnlyMethod(t *testing.T, valueType reflect.Type, name string, want reflect.Type) {
	t.Helper()
	if _, ok := valueType.MethodByName(name); ok {
		t.Errorf("%s value method set unexpectedly contains %s; carrier may be pointer-embedded", valueType, name)
	}
	method, ok := reflect.PointerTo(valueType).MethodByName(name)
	if !ok {
		t.Errorf("*%s.%s is missing", valueType, name)
		return
	}
	if method.Type != want {
		t.Errorf("*%s.%s type = %v, want %v", valueType, name, method.Type, want)
	}
}

func assertInheritanceStub(t *testing.T, err error, module, operation string) {
	t.Helper()
	if !errors.Is(err, codingagent.ErrNotImplemented) {
		t.Fatalf("%s error = %v, want ErrNotImplemented", operation, err)
	}
	var unavailable *codingagent.NotImplementedError
	if !errors.As(err, &unavailable) {
		t.Fatalf("%s error = %T, want *NotImplementedError", operation, err)
	}
	if unavailable.Module != module || unavailable.Operation != operation {
		t.Errorf("error = %#v, want module %q operation %q", unavailable, module, operation)
	}
}
