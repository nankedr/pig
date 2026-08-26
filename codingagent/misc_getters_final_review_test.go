package codingagent

import (
	"errors"
	"reflect"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/client"
	"github.com/nankedr/pig/protocol"
	"github.com/nankedr/pig/tui"
)

var (
	_ tui.EditorComponent = (*CustomEditor)(nil)

	_ func(*BashExecutionComponent) (string, error)                = (*BashExecutionComponent).GetCommand
	_ func(*BashExecutionComponent) (string, error)                = (*BashExecutionComponent).GetOutput
	_ func(*CustomEditor) string                                   = (*CustomEditor).GetText
	_ func(*ModelSelectorComponent) (*tui.Input, error)            = (*ModelSelectorComponent).GetSearchInput
	_ func(*SessionSelectorComponent) (*tui.SelectList, error)     = (*SessionSelectorComponent).GetSessionList
	_ func(*SettingsSelectorComponent) (*tui.SettingsList, error)  = (*SettingsSelectorComponent).GetSettingsList
	_ func(*ShowImagesSelectorComponent) (*tui.SelectList, error)  = (*ShowImagesSelectorComponent).GetSelectList
	_ func(*ThemeSelectorComponent) (*tui.SelectList, error)       = (*ThemeSelectorComponent).GetSelectList
	_ func(*ThinkingSelectorComponent) (*tui.SelectList, error)    = (*ThinkingSelectorComponent).GetSelectList
	_ func(*TreeSelectorComponent) (*tui.SelectList, error)        = (*TreeSelectorComponent).GetTreeList
	_ func(*UserMessageSelectorComponent) (*tui.SelectList, error) = (*UserMessageSelectorComponent).GetMessageList
	_ func(*Theme, agent.ThinkingLevel) (tui.TextStyleFunc, error) = (*Theme).GetThinkingBorderColor
	_ func() (tui.MarkdownTheme, error)                            = GetMarkdownTheme
	_ func() (tui.SelectListTheme, error)                          = GetSelectListTheme
	_ func() (tui.SettingsListTheme, error)                        = GetSettingsListTheme
	_ func(string, ...string) ([]string, error)                    = HighlightCode
	_ func(*RPCClient) (string, error)                             = (*RPCClient).GetStderr
	_ func(*RemoteSession) (*string, error)                        = (*RemoteSession).ID
	_ func(*RemoteSession) (RemoteSessionState, error)             = (*RemoteSession).State
	_ func(*RemoteSession) (*protocol.SessionSnapshot, error)      = (*RemoteSession).Snapshot
	_ func(*RemoteSession) (*protocol.SessionPhase, error)         = (*RemoteSession).Phase
	_ func(*RemoteSession) (*RemoteSessionOperation, error)        = (*RemoteSession).Operation
	_ func(*RemoteSession) ([]protocol.ModelMetadata, error)       = (*RemoteSession).Models
	_ func(*RemoteSession) ([]protocol.SessionMetadata, error)     = (*RemoteSession).Sessions
	_ func(*RemoteSession) (client.ConnectionState, error)         = (*RemoteSession).ConnectionState
	_ func(*RemoteSession) (bool, error)                           = (*RemoteSession).Disposed
)

func TestDeferredUIGettersReturnStructuredErrors(t *testing.T) {
	assertDeferredGetter(t, "BashExecutionComponent.GetCommand", new(BashExecutionComponent).GetCommand)
	assertDeferredGetter(t, "BashExecutionComponent.GetOutput", new(BashExecutionComponent).GetOutput)
	assertDeferredGetter(t, "ModelSelectorComponent.GetSearchInput", new(ModelSelectorComponent).GetSearchInput)
	assertDeferredGetter(t, "SessionSelectorComponent.GetSessionList", new(SessionSelectorComponent).GetSessionList)
	assertDeferredGetter(t, "SettingsSelectorComponent.GetSettingsList", new(SettingsSelectorComponent).GetSettingsList)
	assertDeferredGetter(t, "ShowImagesSelectorComponent.GetSelectList", new(ShowImagesSelectorComponent).GetSelectList)
	assertDeferredGetter(t, "ThemeSelectorComponent.GetSelectList", new(ThemeSelectorComponent).GetSelectList)
	assertDeferredGetter(t, "ThinkingSelectorComponent.GetSelectList", new(ThinkingSelectorComponent).GetSelectList)
	assertDeferredGetter(t, "TreeSelectorComponent.GetTreeList", new(TreeSelectorComponent).GetTreeList)
	assertDeferredGetter(t, "UserMessageSelectorComponent.GetMessageList", new(UserMessageSelectorComponent).GetMessageList)
}

func TestDeferredThemeHelpersReturnStructuredErrors(t *testing.T) {
	assertDeferredGetter(t, "Theme.GetThinkingBorderColor", func() (tui.TextStyleFunc, error) {
		return new(Theme).GetThinkingBorderColor(agent.ThinkingLevel("high"))
	})
	assertDeferredGetter(t, "GetMarkdownTheme", GetMarkdownTheme)
	assertDeferredGetter(t, "GetSelectListTheme", GetSelectListTheme)
	assertDeferredGetter(t, "GetSettingsListTheme", GetSettingsListTheme)
	assertDeferredGetter(t, "HighlightCode", func() ([]string, error) {
		return HighlightCode("package main", "go")
	})
}

func TestRPCClientGetStderrReturnsStructuredError(t *testing.T) {
	assertDeferredGetter(t, "RPCClient.GetStderr", new(RPCClient).GetStderr)
}

func TestRemoteSessionGettersReturnStructuredErrors(t *testing.T) {
	session := new(RemoteSession)
	assertDeferredGetter(t, "RemoteSession.ID", session.ID)
	assertDeferredGetter(t, "RemoteSession.State", session.State)
	assertDeferredGetter(t, "RemoteSession.Snapshot", session.Snapshot)
	assertDeferredGetter(t, "RemoteSession.Phase", session.Phase)
	assertDeferredGetter(t, "RemoteSession.Operation", session.Operation)
	assertDeferredGetter(t, "RemoteSession.Models", session.Models)
	assertDeferredGetter(t, "RemoteSession.Sessions", session.Sessions)
	assertDeferredGetter(t, "RemoteSession.ConnectionState", session.ConnectionState)
	assertDeferredGetter(t, "RemoteSession.Disposed", session.Disposed)
}

func assertDeferredGetter[T any](t *testing.T, operation string, getter func() (T, error)) {
	t.Helper()

	got, err := getter()
	var zero T
	if !reflect.DeepEqual(got, zero) {
		t.Fatalf("%s result = %#v, want zero value %#v", operation, got, zero)
	}
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("%s error = %v, want ErrNotImplemented", operation, err)
	}
	var unavailable *NotImplementedError
	if !errors.As(err, &unavailable) {
		t.Fatalf("%s error = %T, want *codingagent.NotImplementedError", operation, err)
	}
	if unavailable.Module != "codingagent" || unavailable.Operation != operation {
		t.Fatalf("%s error = %#v, want codingagent.%s", operation, unavailable, operation)
	}
}
