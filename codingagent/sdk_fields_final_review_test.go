package codingagent_test

import (
	"reflect"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/codingagent"
)

func TestSDKCustomToolsUseToolDefinitions(t *testing.T) {
	want := reflect.TypeOf([]codingagent.ToolDefinition(nil))
	for _, tc := range []struct {
		name    string
		options any
	}{
		{name: "CreateAgentSessionOptions", options: codingagent.CreateAgentSessionOptions{}},
		{name: "CreateAgentSessionFromServicesOptions", options: codingagent.CreateAgentSessionFromServicesOptions{}},
	} {
		field, ok := reflect.TypeOf(tc.options).FieldByName("CustomTools")
		if !ok {
			t.Fatalf("%s.CustomTools is missing", tc.name)
		}
		if field.Type != want {
			t.Errorf("%s.CustomTools type = %s, want %s", tc.name, field.Type, want)
		}
	}
}

func TestSDKInMemorySessionInjectionFields(t *testing.T) {
	typeOf := reflect.TypeOf(codingagent.CreateAgentSessionOptions{})
	want := map[string]reflect.Type{
		"Provider":       reflect.TypeOf((*codingagent.AgentSessionProvider)(nil)).Elem(),
		"StreamFunction": reflect.TypeOf((agent.StreamFunction)(nil)),
		"AgentTools":     reflect.TypeOf([]agent.ErasedAgentTool(nil)),
	}
	for name, wantType := range want {
		field, ok := typeOf.FieldByName(name)
		if !ok || field.Type != wantType {
			t.Errorf("CreateAgentSessionOptions.%s = %v, want %v", name, field.Type, wantType)
		}
	}
}

func TestSDKSessionCarrierFields(t *testing.T) {
	if _, ok := reflect.TypeOf(codingagent.CreateAgentSessionServicesOptions{}).FieldByName("ModelRuntimeSignal"); ok {
		t.Fatal("CreateAgentSessionServicesOptions.ModelRuntimeSignal duplicates the leading context.Context cancellation carrier")
	}
	for _, carrier := range []struct {
		name   string
		typeOf reflect.Type
	}{
		{name: "CreateModelRuntimeOptions", typeOf: reflect.TypeOf(codingagent.CreateModelRuntimeOptions{})},
		{name: "ExecOptions", typeOf: reflect.TypeOf(codingagent.ExecOptions{})},
		{name: "ExtensionUIDialogOptions", typeOf: reflect.TypeOf(codingagent.ExtensionUIDialogOptions{})},
		{name: "ExtensionContext", typeOf: reflect.TypeOf(codingagent.ExtensionContext{})},
		{name: "SessionBeforeCompactEvent", typeOf: reflect.TypeOf(codingagent.SessionBeforeCompactEvent{})},
		{name: "SessionBeforeTreeEvent", typeOf: reflect.TypeOf(codingagent.SessionBeforeTreeEvent{})},
		{name: "GenerateBranchSummaryOptions", typeOf: reflect.TypeOf(codingagent.GenerateBranchSummaryOptions{})},
	} {
		if _, ok := carrier.typeOf.FieldByName("Signal"); ok {
			t.Errorf("%s.Signal duplicates operation context cancellation", carrier.name)
		}
	}

	extensionHandlerType := reflect.TypeOf((*codingagent.ExtensionHandler)(nil)).Elem()
	tests := []struct {
		name      string
		container reflect.Type
		field     string
		want      reflect.Type
	}{
		{"prompt preflight result", reflect.TypeOf(codingagent.PromptOptions{}), "PreflightResult", reflect.TypeOf((func(bool))(nil))},
		{"extension runner ref", reflect.TypeOf(codingagent.AgentSessionConfig{}), "ExtensionRunnerRef", reflect.TypeOf((*codingagent.ExtensionRunner)(nil))},
		{"switch cwd override", reflect.TypeOf(codingagent.SwitchSessionOptions{}), "CWDOverride", reflect.TypeOf((*string)(nil))},
		{"switch with session", reflect.TypeOf(codingagent.SwitchSessionOptions{}), "WithSession", extensionHandlerType},
		{"new parent session", reflect.TypeOf(codingagent.NewRuntimeSessionOptions{}), "ParentSession", reflect.TypeOf((*string)(nil))},
		{"new setup", reflect.TypeOf(codingagent.NewRuntimeSessionOptions{}), "Setup", extensionHandlerType},
		{"new with session", reflect.TypeOf(codingagent.NewRuntimeSessionOptions{}), "WithSession", extensionHandlerType},
		{"fork position", reflect.TypeOf(codingagent.ForkSessionOptions{}), "Position", reflect.TypeOf((*string)(nil))},
		{"fork with session", reflect.TypeOf(codingagent.ForkSessionOptions{}), "WithSession", extensionHandlerType},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			field, ok := test.container.FieldByName(test.field)
			if !ok {
				t.Fatalf("%s.%s is missing", test.container, test.field)
			}
			if field.Type != test.want {
				t.Fatalf("%s.%s type = %s, want %s", test.container, test.field, field.Type, test.want)
			}
		})
	}
}
