package tui_test

import (
	"errors"
	"testing"

	"github.com/nankedr/pig/tui"
)

type recordingRenderRequester struct{ calls int }

func (requester *recordingRenderRequester) RequestRender(...bool) error {
	requester.calls++
	return nil
}

type recordingEditorHost struct {
	recordingRenderRequester
}

func (*recordingEditorHost) Terminal() tui.Terminal { return nil }

func TestComponentInteractiveStubsAreExplicitAndInert(t *testing.T) {
	t.Parallel()

	renderRequests := 0
	flash := tui.NewAltScreenFlashContainer(func() { renderRequests++ })
	assertTUIStub(t, flash.Flash("saved"), "AltScreenFlashContainer.flash")
	if renderRequests != 0 {
		t.Fatalf("Flash invoked requestRender %d times, want 0", renderRequests)
	}

	loader := tui.NewLoader(nil, nil, nil, "Loading...")
	assertTUIStub(t, loader.Start(), "Loader.start")
	assertTUIStub(t, loader.Stop(), "Loader.stop")

	input := tui.NewInput()
	inputCalls := 0
	input.OnSubmit = func(string) { inputCalls++ }
	input.OnEscape = func() { inputCalls++ }
	assertTUIStub(t, input.HandleInput("\n"), "Input.handleInput")
	if inputCalls != 0 {
		t.Fatalf("Input.HandleInput invoked %d callbacks, want 0", inputCalls)
	}
}

func TestEditorStateAccessDoesNotInvokeRuntime(t *testing.T) {
	t.Parallel()

	padding := 2
	visible := 7
	editor := tui.NewEditor(nil, tui.EditorTheme{}, tui.EditorOptions{
		PaddingX:               &padding,
		AutocompleteMaxVisible: &visible,
	})
	if got := editor.GetPaddingX(); got != padding {
		t.Fatalf("GetPaddingX() = %d, want %d", got, padding)
	}
	if got := editor.GetAutocompleteMaxVisible(); got != visible {
		t.Fatalf("GetAutocompleteMaxVisible() = %d, want %d", got, visible)
	}
	if err := editor.SetText("one\ntwo"); err != nil {
		t.Fatalf("SetText: %v", err)
	}
	if got := editor.GetText(); got != "one\ntwo" {
		t.Fatalf("GetText() = %q, want %q", got, "one\ntwo")
	}
	wantLines := []string{"one", "two"}
	gotLines := editor.GetLines()
	if len(gotLines) != len(wantLines) || gotLines[0] != wantLines[0] || gotLines[1] != wantLines[1] {
		t.Fatalf("GetLines() = %#v, want %#v", gotLines, wantLines)
	}

	assertTUIStub(t, editor.HandleInput("x"), "Editor.handleInput")
	if got := editor.GetText(); got != "one\ntwo" {
		t.Fatalf("failed HandleInput mutated text to %q", got)
	}
}

func TestEditorComponentOptionalWiringStoresCallbacksWithoutInvokingThem(t *testing.T) {
	t.Parallel()

	var component tui.EditorComponent = tui.NewEditor(nil, tui.EditorTheme{})
	submitCalls := 0
	changeCalls := 0
	styleCalls := 0

	submitSetter, ok := component.(tui.EditorSubmitSetter)
	if !ok {
		t.Fatal("EditorComponent does not expose the optional submit-callback seam")
	}
	changeSetter, ok := component.(tui.EditorChangeSetter)
	if !ok {
		t.Fatal("EditorComponent does not expose the optional change-callback seam")
	}
	borderSetter, ok := component.(tui.EditorBorderColorSetter)
	if !ok {
		t.Fatal("EditorComponent does not expose the optional border-color seam")
	}

	submitSetter.SetOnSubmit(func(string) { submitCalls++ })
	changeSetter.SetOnChange(func(string) { changeCalls++ })
	borderSetter.SetBorderColor(func(value string) string {
		styleCalls++
		return value
	})

	if submitCalls != 0 || changeCalls != 0 || styleCalls != 0 {
		t.Fatalf("wiring invoked callbacks: submit=%d change=%d style=%d", submitCalls, changeCalls, styleCalls)
	}
	editor := component.(*tui.Editor)
	if editor.OnSubmit == nil || editor.OnChange == nil || editor.BorderColor == nil {
		t.Fatal("wiring did not retain all optional editor callbacks")
	}
}

func TestComponentConstructorsAcceptNarrowRuntimeSeamsWithoutInvokingThem(t *testing.T) {
	t.Parallel()

	loaderHost := &recordingRenderRequester{}
	editorHost := &recordingEditorHost{}
	_ = tui.NewLoader(loaderHost, nil, nil, "Loading...")
	_ = tui.NewCancellableLoader(loaderHost, nil, nil, "Loading...")
	_ = tui.NewEditor(editorHost, tui.EditorTheme{})
	if loaderHost.calls != 0 || editorHost.calls != 0 {
		t.Fatalf("constructors requested rendering: loader=%d editor=%d", loaderHost.calls, editorHost.calls)
	}
}

func assertTUIStub(t *testing.T, err error, operation string) {
	t.Helper()
	if !errors.Is(err, tui.ErrNotImplemented) {
		t.Fatalf("errors.Is(%v, ErrNotImplemented) = false", err)
	}
	var target *tui.NotImplementedError
	if !errors.As(err, &target) {
		t.Fatalf("errors.As(%v, *NotImplementedError) = false", err)
	}
	if target.Module != "tui" || target.Operation != operation {
		t.Fatalf("NotImplementedError = %+v, want module tui operation %s", target, operation)
	}
}
