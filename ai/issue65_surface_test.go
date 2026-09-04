package ai_test

import (
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nankedr/pig/ai"
)

var updateIssue65Surface = flag.Bool("update-issue65-surface", false, "regenerate issue #65 API snapshot")

func TestIssue65LockedGoAPISnapshot(t *testing.T) {
	sections := []string{}
	sections = append(sections, issue60TypeSnapshot("APIProvider", reflect.TypeOf(ai.APIProvider{})))
	sections = append(sections, issue60TypeSnapshot("FauxCore", reflect.TypeOf(ai.FauxCore{})))
	sections = append(sections, issue60TypeSnapshot("FauxProviderHandle", reflect.TypeOf(ai.FauxProviderHandle{})))
	sections = append(sections, issue60TypeSnapshot("FauxProviderRegistration", reflect.TypeOf(ai.FauxProviderRegistration{})))
	sections = append(sections, issue60TypeSnapshot("FauxProviderState", reflect.TypeOf(ai.FauxProviderState{})))
	sections = append(sections, issue60TypeSnapshot("RegisterFauxProviderOptions", reflect.TypeOf(ai.RegisterFauxProviderOptions{})))
	sections = append(sections, issue60TypeSnapshot("FauxModelDefinition", reflect.TypeOf(ai.FauxModelDefinition{})))
	sections = append(sections, issue60TypeSnapshot("FauxAssistantMessageOptions", reflect.TypeOf(ai.FauxAssistantMessageOptions{})))
	sections = append(sections, issue60TypeSnapshot("FauxToolCallOptions", reflect.TypeOf(ai.FauxToolCallOptions{})))
	sections = append(sections, issue60TypeSnapshot("FauxTokenSize", reflect.TypeOf(ai.FauxTokenSize{})))
	sections = append(sections, issue60TypeSnapshot("FauxDeferredOptions", reflect.TypeOf(ai.FauxDeferredOptions{})))
	sections = append(sections, "type ai.SessionResourceCleanup "+issue60FuncSignature(reflect.TypeOf((*ai.SessionResourceCleanup)(nil)).Elem())+" variadic")
	sections = append(sections, "type ai.CompatAPIStreamFunction "+issue60FuncSignature(reflect.TypeOf((*ai.CompatAPIStreamFunction)(nil)).Elem()))
	sections = append(sections, "type ai.CompatAPISimpleStreamFunction "+issue60FuncSignature(reflect.TypeOf((*ai.CompatAPISimpleStreamFunction)(nil)).Elem()))
	sections = append(sections, "type ai.FauxResponseFactory "+issue60FuncSignature(reflect.TypeOf((*ai.FauxResponseFactory)(nil)).Elem()))
	sections = append(sections, "type ai.FauxResponseStep "+reflect.TypeOf((*ai.FauxResponseStep)(nil)).Elem().String())
	sections = append(sections, "type ai.FauxContentBlock "+reflect.TypeOf((*ai.FauxContentBlock)(nil)).Elem().String())
	functions := []struct {
		name  string
		value any
	}{
		{"CleanupSessionResources", ai.CleanupSessionResources},
		{"Complete", ai.Complete},
		{"CompleteSimple", ai.CompleteSimple},
		{"CreateFauxCore", ai.CreateFauxCore},
		{"FauxAssistantBlocks", ai.FauxAssistantBlocks},
		{"FauxAssistantMessage", ai.FauxAssistantMessage},
		{"FauxAssistantText", ai.FauxAssistantText},
		{"FauxText", ai.FauxText},
		{"FauxThinking", ai.FauxThinking},
		{"FauxToolCall", ai.FauxToolCall},
		{"GetAPIProvider", ai.GetAPIProvider},
		{"GetAPIProviders", ai.GetAPIProviders},
		{"GetModel", ai.GetModel},
		{"GetModels", ai.GetModels},
		{"GetProviders", ai.GetProviders},
		{"NewFauxProvider", ai.NewFauxProvider},
		{"RegisterAPIProvider", ai.RegisterAPIProvider},
		{"RegisterBuiltinAPIProviders", ai.RegisterBuiltinAPIProviders},
		{"RegisterFauxProvider", ai.RegisterFauxProvider},
		{"RegisterSessionResourceCleanup", ai.RegisterSessionResourceCleanup},
		{"ResetAPIProviders", ai.ResetAPIProviders},
		{"Stream", ai.Stream},
		{"StreamAnthropic", ai.StreamAnthropic},
		{"StreamAzureOpenAIResponses", ai.StreamAzureOpenAIResponses},
		{"StreamGoogle", ai.StreamGoogle},
		{"StreamGoogleVertex", ai.StreamGoogleVertex},
		{"StreamMistral", ai.StreamMistral},
		{"StreamOpenAICodexResponses", ai.StreamOpenAICodexResponses},
		{"StreamOpenAICompletions", ai.StreamOpenAICompletions},
		{"StreamOpenAIResponses", ai.StreamOpenAIResponses},
		{"StreamSimple", ai.StreamSimple},
		{"StreamSimpleAnthropic", ai.StreamSimpleAnthropic},
		{"StreamSimpleAzureOpenAIResponses", ai.StreamSimpleAzureOpenAIResponses},
		{"StreamSimpleGoogle", ai.StreamSimpleGoogle},
		{"StreamSimpleGoogleVertex", ai.StreamSimpleGoogleVertex},
		{"StreamSimpleMistral", ai.StreamSimpleMistral},
		{"StreamSimpleOpenAICodexResponses", ai.StreamSimpleOpenAICodexResponses},
		{"StreamSimpleOpenAICompletions", ai.StreamSimpleOpenAICompletions},
		{"StreamSimpleOpenAIResponses", ai.StreamSimpleOpenAIResponses},
		{"UnregisterAPIProviders", ai.UnregisterAPIProviders},
	}
	for _, function := range functions {
		sections = append(sections, "func ai."+function.name+" "+reflect.TypeOf(function.value).String())
	}
	got := strings.Join(sections, "\n\n") + "\n"
	path := filepath.Join("testdata", "issue65_surface_golden.txt")
	if *updateIssue65Surface {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatal("issue #65 API snapshot drifted; regenerate with -update-issue65-surface")
	}
}
