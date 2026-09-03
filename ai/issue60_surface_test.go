package ai_test

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/nankedr/pig/ai"
)

var updateIssue60Surface = flag.Bool("update-issue60-surface", false, "regenerate the issue #60 Go API snapshot")

func TestIssue60LockedGoAPISnapshot(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	path := filepath.Join(filepath.Dir(file), "testdata", "issue60_surface_golden.txt")
	got := issue60SurfaceSnapshot()
	if *updateIssue60Surface {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatalf("write golden: %v", err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if got != string(want) {
		t.Fatalf("issue #60 Go API snapshot drifted; regenerate with -update-issue60-surface\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func issue60SurfaceSnapshot() string {
	types := []struct {
		name   string
		typeOf reflect.Type
	}{
		{"AssistantMessageThinkingDeltaEvent", reflect.TypeOf(ai.AssistantMessageThinkingDeltaEvent{})},
		{"AssistantMessageThinkingEndEvent", reflect.TypeOf(ai.AssistantMessageThinkingEndEvent{})},
		{"AssistantMessageThinkingStartEvent", reflect.TypeOf(ai.AssistantMessageThinkingStartEvent{})},
		{"FauxModelDefinition", reflect.TypeOf(ai.FauxModelDefinition{})},
		{"Model", reflect.TypeOf(ai.Model{})},
		{"OpenAICompletionsCompat", reflect.TypeOf(ai.OpenAICompletionsCompat{})},
		{"OpenAICompletionsOptions", reflect.TypeOf(ai.OpenAICompletionsOptions{})},
		{"SimpleStreamOptions", reflect.TypeOf(ai.SimpleStreamOptions{})},
		{"TextContent", reflect.TypeOf(ai.TextContent{})},
		{"TextSignatureV1", reflect.TypeOf(ai.TextSignatureV1{})},
		{"ThinkingBudgets", reflect.TypeOf(ai.ThinkingBudgets{})},
		{"ThinkingContent", reflect.TypeOf(ai.ThinkingContent{})},
		{"ToolCall", reflect.TypeOf(ai.ToolCall{})},
		{"Usage", reflect.TypeOf(ai.Usage{})},
	}
	functions := []struct {
		name  string
		value any
	}{
		{"ConvertOpenAICompletionsMessages", ai.ConvertOpenAICompletionsMessages},
		{"FauxThinking", ai.FauxThinking},
		{"StreamOpenAICompletions", ai.StreamOpenAICompletions},
		{"StreamSimpleOpenAICompletions", ai.StreamSimpleOpenAICompletions},
	}
	var sections []string
	for _, item := range types {
		sections = append(sections, issue60TypeSnapshot(item.name, item.typeOf))
	}
	for _, item := range functions {
		sections = append(sections, "func ai."+item.name+" "+issue60FuncSignature(reflect.TypeOf(item.value)))
	}
	sections = append(sections, strings.Join([]string{
		"constants:",
		"- ContentTypeThinking=" + string(ai.ContentTypeThinking),
		"- AssistantMessageEventTypeThinkingStart=" + string(ai.AssistantMessageEventTypeThinkingStart),
		"- AssistantMessageEventTypeThinkingDelta=" + string(ai.AssistantMessageEventTypeThinkingDelta),
		"- AssistantMessageEventTypeThinkingEnd=" + string(ai.AssistantMessageEventTypeThinkingEnd),
		"- TextSignaturePhaseCommentary=" + string(ai.TextSignaturePhaseCommentary),
		"- TextSignaturePhaseFinalAnswer=" + string(ai.TextSignaturePhaseFinalAnswer),
	}, "\n"))
	return strings.Join(sections, "\n\n") + "\n"
}

func issue60TypeSnapshot(name string, typ reflect.Type) string {
	lines := []string{"type ai." + name + " " + typ.Kind().String()}
	if typ.Kind() == reflect.Struct {
		lines = append(lines, "fields:")
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			if field.PkgPath == "" {
				lines = append(lines, fmt.Sprintf("- %s %s tag=%q", field.Name, field.Type, field.Tag))
			}
		}
	}
	methods := make([]string, typ.NumMethod())
	for i := 0; i < typ.NumMethod(); i++ {
		method := typ.Method(i)
		methods[i] = "- " + method.Name + " " + issue60FuncSignature(method.Type)
	}
	if len(methods) > 0 {
		sort.Strings(methods)
		lines = append(lines, "methods:")
		lines = append(lines, methods...)
	}
	return strings.Join(lines, "\n")
}

func issue60FuncSignature(typ reflect.Type) string {
	inputs := make([]string, typ.NumIn())
	for i := range inputs {
		inputs[i] = typ.In(i).String()
	}
	outputs := make([]string, typ.NumOut())
	for i := range outputs {
		outputs[i] = typ.Out(i).String()
	}
	result := "func(" + strings.Join(inputs, ", ") + ")"
	if len(outputs) == 1 {
		return result + " " + outputs[0]
	}
	if len(outputs) > 1 {
		return result + " (" + strings.Join(outputs, ", ") + ")"
	}
	return result
}
