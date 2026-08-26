package codingagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
)

var (
	_ func(SettingsManager) (BranchSummarySettings, error)                   = SettingsManager.GetBranchSummarySettings
	_ func(SettingsManager) (int64, error)                                   = SettingsManager.GetHTTPIdleTimeoutMS
	_ func(SettingsManager, int64) error                                     = SettingsManager.SetHTTPIdleTimeoutMS
	_ func(*InteractiveMode, LatestRelease) error                            = (*InteractiveMode).ShowNewVersionNotification
	_ func(*RPCClient, context.Context, string) (BashResult, error)          = (*RPCClient).Bash
	_ func(*RPCClient, context.Context, ...string) (CompactionResult, error) = (*RPCClient).Compact
	_ func(*RPCClient, context.Context) ([]ForkMessage, error)               = (*RPCClient).GetForkMessages
	_ func(*RPCClient, context.Context) error                                = (*RPCClient).WaitForIdle
)

func TestBranchSummarySettingsCarrier(t *testing.T) {
	assertExactStructFields(t, BranchSummarySettings{}, []structField{
		{name: "ReserveTokens", typ: reflect.TypeFor[int64]()},
		{name: "SkipPrompt", typ: reflect.TypeFor[bool]()},
	})

	result, err := (SettingsManager{}).GetBranchSummarySettings()
	if result != (BranchSummarySettings{}) {
		t.Fatalf("GetBranchSummarySettings result = %#v, want zero BranchSummarySettings", result)
	}
	assertExactResultsNotImplemented(t, err, "SettingsManager.GetBranchSummarySettings")
}

func TestSettingsManagerHTTPIdleTimeoutMSUsesCanonicalGoSpelling(t *testing.T) {
	managerType := reflect.TypeFor[SettingsManager]()
	if _, ok := managerType.MethodByName("GetHTTPIDleTimeoutMS"); ok {
		t.Fatal("SettingsManager exposes misspelled GetHTTPIDleTimeoutMS")
	}
	if _, ok := managerType.MethodByName("SetHTTPIDleTimeoutMS"); ok {
		t.Fatal("SettingsManager exposes misspelled SetHTTPIDleTimeoutMS")
	}
	_, err := (SettingsManager{}).GetHTTPIdleTimeoutMS()
	assertExactResultsNotImplemented(t, err, "SettingsManager.GetHTTPIdleTimeoutMS")
	assertExactResultsNotImplemented(t, (SettingsManager{}).SetHTTPIdleTimeoutMS(0), "SettingsManager.SetHTTPIdleTimeoutMS")
}

func TestLatestReleaseCarrier(t *testing.T) {
	assertExactStructFields(t, LatestRelease{}, []structField{
		{name: "Version", typ: reflect.TypeFor[string]()},
		{name: "PackageName", typ: reflect.TypeFor[*string]()},
		{name: "Note", typ: reflect.TypeFor[*string]()},
	})

	packageName, note := "@mariozechner/pi-coding-agent", "release notes"
	release := LatestRelease{Version: "1.2.3", PackageName: &packageName, Note: &note}
	err := (*InteractiveMode)(nil).ShowNewVersionNotification(release)
	assertExactResultsNotImplemented(t, err, "InteractiveMode.ShowNewVersionNotification")
	if *release.PackageName != packageName || *release.Note != note {
		t.Fatalf("ShowNewVersionNotification mutated release = %#v", release)
	}
}

func TestRPCClientGetForkMessagesResult(t *testing.T) {
	assertExactStructFields(t, ForkMessage{}, []structField{
		{name: "EntryID", typ: reflect.TypeFor[string]()},
		{name: "Text", typ: reflect.TypeFor[string]()},
	})

	result, err := (*RPCClient)(nil).GetForkMessages(context.Background())
	if result != nil {
		t.Fatalf("GetForkMessages result = %#v, want nil []ForkMessage", result)
	}
	assertExactResultsNotImplemented(t, err, "RPCClient.GetForkMessages")
}

func TestRPCClientWaitForIdleContract(t *testing.T) {
	clientType := reflect.TypeFor[*RPCClient]()
	if _, ok := clientType.MethodByName("WaitForIdle"); !ok {
		t.Fatal("*RPCClient is missing WaitForIdle")
	}
	if _, ok := clientType.MethodByName("WaitForIDle"); ok {
		t.Fatal("*RPCClient exposes redundant WaitForIDle")
	}

	err := (*RPCClient)(nil).WaitForIdle(context.Background())
	assertExactResultsNotImplemented(t, err, "RPCClient.WaitForIdle")
}

func TestRPCClientBashResult(t *testing.T) {
	assertExactStructFields(t, BashResult{}, []structField{
		{name: "Output", typ: reflect.TypeFor[string]()},
		{name: "ExitCode", typ: reflect.TypeFor[*int]()},
		{name: "Cancelled", typ: reflect.TypeFor[bool]()},
		{name: "Truncated", typ: reflect.TypeFor[bool]()},
		{name: "FullOutputPath", typ: reflect.TypeFor[*string]()},
	})

	sentinel := filepath.Join(t.TempDir(), "rpc-bash-ran")
	result, err := (*RPCClient)(nil).Bash(context.Background(), fmt.Sprintf("touch %q", sentinel))
	if result != (BashResult{}) {
		t.Fatalf("Bash result = %#v, want zero BashResult", result)
	}
	assertExactResultsNotImplemented(t, err, "RPCClient.Bash")
	if _, statErr := os.Stat(sentinel); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("Bash capability stub touched sentinel: %v", statErr)
	}
}

func TestRPCClientCompactResult(t *testing.T) {
	result, err := (*RPCClient)(nil).Compact(context.Background(), "preserve decisions")
	if !reflect.DeepEqual(result, CompactionResult{}) {
		t.Fatalf("Compact result = %#v, want zero CompactionResult", result)
	}
	assertExactResultsNotImplemented(t, err, "RPCClient.Compact")
}

func TestProjectJSONAgentSessionEventRemovesCumulativeMessageState(t *testing.T) {
	partial := ai.AssistantMessage{Role: ai.MessageRoleAssistant, Content: []ai.AssistantContent{
		ai.TextContent{Type: ai.ContentTypeText, Text: "cumulative"},
	}}
	event := AgentSessionMessageUpdateEvent{MessageUpdateEvent: agent.MessageUpdateEvent{
		Type:    agent.AgentEventTypeMessageUpdate,
		Message: partial,
		AssistantMessageEvent: ai.AssistantMessageTextDeltaEvent{
			Type: ai.AssistantMessageEventTypeTextDelta, ContentIndex: 0, Delta: "delta", Partial: partial,
		},
	}}

	projected, err := projectJSONAgentSessionEvent(event)
	if err != nil {
		t.Fatalf("projectJSONAgentSessionEvent: %v", err)
	}
	if projected.AgentSessionEventType() != AgentSessionEventTypeMessageUpdate {
		t.Fatalf("projected type = %q", projected.AgentSessionEventType())
	}
	encoded, err := json.Marshal(projected)
	if err != nil {
		t.Fatalf("Marshal(projected): %v", err)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &object); err != nil {
		t.Fatal(err)
	}
	if _, ok := object["message"]; ok {
		t.Fatalf("JSON message_update retained top-level message: %s", encoded)
	}
	var nested map[string]json.RawMessage
	if err := json.Unmarshal(object["assistantMessageEvent"], &nested); err != nil {
		t.Fatal(err)
	}
	if _, ok := nested["partial"]; ok {
		t.Fatalf("JSON message_update retained nested partial: %s", encoded)
	}
	if string(nested["delta"]) != `"delta"` {
		t.Fatalf("projected delta = %s, want delta", nested["delta"])
	}

	settled := AgentSessionAgentSettledEvent{Type: AgentSessionEventTypeAgentSettled}
	unchanged, err := projectJSONAgentSessionEvent(settled)
	if err != nil || !reflect.DeepEqual(unchanged, settled) {
		t.Fatalf("non-update projection = (%#v, %v), want unchanged %#v", unchanged, err, settled)
	}

	var listener RPCEventListener = func(event JSONAgentSessionEvent) {
		if event.AgentSessionEventType() == "" {
			t.Error("RPC listener received empty discriminator")
		}
	}
	listener(projected)
}

type structField struct {
	name string
	typ  reflect.Type
}

func assertExactStructFields(t *testing.T, value any, want []structField) {
	t.Helper()
	typeOf := reflect.TypeOf(value)
	if typeOf.NumField() != len(want) {
		t.Fatalf("%s has %d fields, want %d", typeOf, typeOf.NumField(), len(want))
	}
	for index, wantField := range want {
		got := typeOf.Field(index)
		if got.Name != wantField.name || got.Type != wantField.typ || !got.IsExported() {
			t.Errorf("%s field %d = %s %s (exported=%t), want %s %s exported", typeOf, index, got.Name, got.Type, got.IsExported(), wantField.name, wantField.typ)
		}
	}
}

func assertExactResultsNotImplemented(t *testing.T, err error, operation string) {
	t.Helper()
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("%s error = %v, want ErrNotImplemented", operation, err)
	}
	var unavailable *NotImplementedError
	if !errors.As(err, &unavailable) {
		t.Fatalf("%s error = %T, want *NotImplementedError", operation, err)
	}
	if unavailable.Module != "codingagent" || unavailable.Operation != operation {
		t.Fatalf("%s error = %#v, want module codingagent and exact operation", operation, unavailable)
	}
}
