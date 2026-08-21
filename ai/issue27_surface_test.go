package ai_test

import (
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/internal/catalog"
	"github.com/nankedr/pig/internal/surface"
)

type issue27FactoryMapping struct {
	surfaceID string
	catalogID string
	goName    string
	value     any
}

var issue27FactoryMappings = []issue27FactoryMapping{
	{"symbol:ai/src/api/anthropic-messages.lazy.ts#anthropicMessagesApi", "contract:ai/api-entrypoints", "AnthropicMessagesAPI", ai.AnthropicMessagesAPI},
	{"symbol:ai/src/api/azure-openai-responses.lazy.ts#azureOpenAIResponsesApi", "contract:ai/api-entrypoints", "AzureOpenAIResponsesAPI", ai.AzureOpenAIResponsesAPI},
	{"symbol:ai/src/api/bedrock-converse-stream.lazy.ts#bedrockConverseStreamApi", "contract:ai/api-entrypoints", "BedrockConverseStreamAPI", ai.BedrockConverseStreamAPI},
	{"symbol:ai/src/api/google-generative-ai.lazy.ts#googleGenerativeAIApi", "contract:ai/api-entrypoints", "GoogleGenerativeAIAPI", ai.GoogleGenerativeAIAPI},
	{"symbol:ai/src/api/google-vertex.lazy.ts#googleVertexApi", "contract:ai/api-entrypoints", "GoogleVertexAPI", ai.GoogleVertexAPI},
	{"symbol:ai/src/api/mistral-conversations.lazy.ts#mistralConversationsApi", "contract:ai/api-entrypoints", "MistralConversationsAPI", ai.MistralConversationsAPI},
	{"symbol:ai/src/api/openai-codex-responses.lazy.ts#openAICodexResponsesApi", "contract:ai/api-entrypoints", "OpenAICodexResponsesAPI", ai.OpenAICodexResponsesAPI},
	{"symbol:ai/src/api/openai-completions.lazy.ts#openAICompletionsApi", "contract:ai/api-entrypoints", "OpenAICompletionsAPI", ai.OpenAICompletionsAPI},
	{"symbol:ai/src/api/openai-responses.lazy.ts#openAIResponsesApi", "contract:ai/api-entrypoints", "OpenAIResponsesAPI", ai.OpenAIResponsesAPI},
	{"symbol:ai/src/api/openrouter-images.lazy.ts#openrouterImagesApi", "contract:ai/images", "OpenRouterImagesAPI", ai.OpenRouterImagesAPI},
	{"symbol:ai/src/api/pi-messages.lazy.ts#piMessagesApi", "contract:ai/api-entrypoints", "PiMessagesAPI", ai.PiMessagesAPI},

	{"symbol:ai/src/providers/amazon-bedrock.ts#amazonBedrockProvider", "contract:ai/provider-factories", "AmazonBedrockProvider", ai.AmazonBedrockProvider},
	{"symbol:ai/src/providers/ant-ling.ts#antLingProvider", "contract:ai/provider-factories", "AntLingProvider", ai.AntLingProvider},
	{"symbol:ai/src/providers/anthropic.ts#anthropicProvider", "contract:ai/provider-factories", "AnthropicProvider", ai.AnthropicProvider},
	{"symbol:ai/src/providers/azure-openai-responses.ts#azureOpenAIResponsesProvider", "contract:ai/provider-factories", "AzureOpenAIResponsesProvider", ai.AzureOpenAIResponsesProvider},
	{"symbol:ai/src/providers/baseten.ts#basetenProvider", "contract:ai/provider-factories", "BasetenProvider", ai.BasetenProvider},
	{"symbol:ai/src/providers/cerebras.ts#cerebrasProvider", "contract:ai/provider-factories", "CerebrasProvider", ai.CerebrasProvider},
	{"symbol:ai/src/providers/cloudflare-ai-gateway.ts#cloudflareAIGatewayProvider", "contract:ai/provider-factories", "CloudflareAIGatewayProvider", ai.CloudflareAIGatewayProvider},
	{"symbol:ai/src/providers/cloudflare-workers-ai.ts#cloudflareWorkersAIProvider", "contract:ai/provider-factories", "CloudflareWorkersAIProvider", ai.CloudflareWorkersAIProvider},
	{"symbol:ai/src/providers/deepseek.ts#deepseekProvider", "contract:ai/provider-factories", "DeepSeekProvider", ai.DeepSeekProvider},
	{"symbol:ai/src/providers/faux.ts#fauxProvider", "contract:ai/faux-provider", "NewFauxProvider", ai.NewFauxProvider},
	{"symbol:ai/src/providers/fireworks.ts#fireworksProvider", "contract:ai/provider-factories", "FireworksProvider", ai.FireworksProvider},
	{"symbol:ai/src/providers/github-copilot.ts#githubCopilotProvider", "contract:ai/provider-factories", "GitHubCopilotProvider", ai.GitHubCopilotProvider},
	{"symbol:ai/src/providers/google.ts#googleProvider", "contract:ai/provider-factories", "GoogleProvider", ai.GoogleProvider},
	{"symbol:ai/src/providers/google-vertex.ts#googleVertexProvider", "contract:ai/provider-factories", "GoogleVertexProvider", ai.GoogleVertexProvider},
	{"symbol:ai/src/providers/groq.ts#groqProvider", "contract:ai/provider-factories", "GroqProvider", ai.GroqProvider},
	{"symbol:ai/src/providers/huggingface.ts#huggingfaceProvider", "contract:ai/provider-factories", "HuggingFaceProvider", ai.HuggingFaceProvider},
	{"symbol:ai/src/providers/kimi-coding.ts#kimiCodingProvider", "contract:ai/provider-factories", "KimiCodingProvider", ai.KimiCodingProvider},
	{"symbol:ai/src/providers/minimax-cn.ts#minimaxCnProvider", "contract:ai/provider-factories", "MiniMaxCNProvider", ai.MiniMaxCNProvider},
	{"symbol:ai/src/providers/minimax.ts#minimaxProvider", "contract:ai/provider-factories", "MiniMaxProvider", ai.MiniMaxProvider},
	{"symbol:ai/src/providers/mistral.ts#mistralProvider", "contract:ai/provider-factories", "MistralProvider", ai.MistralProvider},
	{"symbol:ai/src/providers/moonshotai-cn.ts#moonshotaiCnProvider", "contract:ai/provider-factories", "MoonshotAICNProvider", ai.MoonshotAICNProvider},
	{"symbol:ai/src/providers/moonshotai.ts#moonshotaiProvider", "contract:ai/provider-factories", "MoonshotAIProvider", ai.MoonshotAIProvider},
	{"symbol:ai/src/providers/nvidia.ts#nvidiaProvider", "contract:ai/provider-factories", "NVIDIAProvider", ai.NVIDIAProvider},
	{"symbol:ai/src/providers/openai-codex.ts#openaiCodexProvider", "contract:ai/provider-factories", "OpenAICodexProvider", ai.OpenAICodexProvider},
	{"symbol:ai/src/providers/openai.ts#openaiProvider", "contract:ai/provider-factories", "OpenAIProvider", ai.OpenAIProvider},
	{"symbol:ai/src/providers/opencode-go.ts#opencodeGoProvider", "contract:ai/provider-factories", "OpenCodeGoProvider", ai.OpenCodeGoProvider},
	{"symbol:ai/src/providers/opencode.ts#opencodeProvider", "contract:ai/provider-factories", "OpenCodeProvider", ai.OpenCodeProvider},
	{"symbol:ai/src/providers/openrouter-images.ts#openrouterImagesProvider", "contract:ai/images", "OpenRouterImagesProvider", ai.OpenRouterImagesProvider},
	{"symbol:ai/src/providers/openrouter.ts#openrouterProvider", "contract:ai/provider-factories", "OpenRouterProvider", ai.OpenRouterProvider},
	{"symbol:ai/src/providers/qwen-token-plan-cn.ts#qwenTokenPlanCnProvider", "contract:ai/provider-factories", "QwenTokenPlanCNProvider", ai.QwenTokenPlanCNProvider},
	{"symbol:ai/src/providers/qwen-token-plan-individual.ts#qwenTokenPlanIndividualProvider", "contract:ai/provider-factories", "QwenTokenPlanIndividualProvider", ai.QwenTokenPlanIndividualProvider},
	{"symbol:ai/src/providers/qwen-token-plan.ts#qwenTokenPlanProvider", "contract:ai/provider-factories", "QwenTokenPlanProvider", ai.QwenTokenPlanProvider},
	{"symbol:ai/src/providers/radius.ts#radiusProvider", "contract:ai/provider-factories", "RadiusProvider", ai.RadiusProvider},
	{"symbol:ai/src/providers/together.ts#togetherProvider", "contract:ai/provider-factories", "TogetherProvider", ai.TogetherProvider},
	{"symbol:ai/src/providers/vercel-ai-gateway.ts#vercelAIGatewayProvider", "contract:ai/provider-factories", "VercelAIGatewayProvider", ai.VercelAIGatewayProvider},
	{"symbol:ai/src/providers/xai.ts#xaiProvider", "contract:ai/provider-factories", "XAIProvider", ai.XAIProvider},
	{"symbol:ai/src/providers/xiaomi-token-plan-ams.ts#xiaomiTokenPlanAmsProvider", "contract:ai/provider-factories", "XiaomiTokenPlanAMSProvider", ai.XiaomiTokenPlanAMSProvider},
	{"symbol:ai/src/providers/xiaomi-token-plan-cn.ts#xiaomiTokenPlanCnProvider", "contract:ai/provider-factories", "XiaomiTokenPlanCNProvider", ai.XiaomiTokenPlanCNProvider},
	{"symbol:ai/src/providers/xiaomi-token-plan-sgp.ts#xiaomiTokenPlanSgpProvider", "contract:ai/provider-factories", "XiaomiTokenPlanSGPProvider", ai.XiaomiTokenPlanSGPProvider},
	{"symbol:ai/src/providers/xiaomi.ts#xiaomiProvider", "contract:ai/provider-factories", "XiaomiProvider", ai.XiaomiProvider},
	{"symbol:ai/src/providers/zai-coding-cn.ts#zaiCodingCnProvider", "contract:ai/provider-factories", "ZAICodingCNProvider", ai.ZAICodingCNProvider},
	{"symbol:ai/src/providers/zai.ts#zaiProvider", "contract:ai/provider-factories", "ZAIProvider", ai.ZAIProvider},
}

func TestIssue27FactoryMappingsMatchLockedSurface(t *testing.T) {
	root := issue27RepoRoot(t)
	symbols, err := surface.LoadSymbols(filepath.Join(root, "parity", "surface", "symbols.jsonl"))
	if err != nil {
		t.Fatalf("LoadSymbols: %v", err)
	}
	want := make(map[string]bool)
	for _, symbol := range symbols {
		if issue27FactorySymbol(symbol) {
			want[symbol.ID] = true
		}
	}
	if len(want) != 53 {
		t.Fatalf("locked factory surface count = %d, want 53", len(want))
	}

	entries, err := catalog.LoadCatalog(filepath.Join(root, "parity", "catalog.jsonl"))
	if err != nil {
		t.Fatalf("LoadCatalog: %v", err)
	}
	catalogEntries := make(map[string]catalog.Entry, len(entries))
	for _, entry := range entries {
		catalogEntries[entry.ID] = entry
	}

	got := make(map[string]bool, len(issue27FactoryMappings))
	for _, mapping := range issue27FactoryMappings {
		if got[mapping.surfaceID] {
			t.Fatalf("duplicate mapping for %s", mapping.surfaceID)
		}
		got[mapping.surfaceID] = true
		if !want[mapping.surfaceID] {
			t.Errorf("mapping is outside locked factory surface: %s", mapping.surfaceID)
		}
		if name := runtime.FuncForPC(reflect.ValueOf(mapping.value).Pointer()).Name(); !strings.HasSuffix(name, "/ai."+mapping.goName) {
			t.Errorf("%s maps to %q, want Go symbol %q", mapping.surfaceID, name, mapping.goName)
		}
		entry, ok := catalogEntries[mapping.catalogID]
		if !ok || entry.Mapping.Module != "ai" || entry.Mapping.Target != "github.com/nankedr/pig/ai" {
			t.Errorf("%s references invalid Catalog mapping %q", mapping.surfaceID, mapping.catalogID)
			continue
		}
		switch entry.Status {
		case catalog.StatusScaffolded, catalog.StatusPartial, catalog.StatusImplemented, catalog.StatusVerified:
		default:
			t.Errorf("%s references missing or inactive Capability Status %q", mapping.surfaceID, mapping.catalogID)
		}
	}
	for id := range want {
		if !got[id] {
			t.Errorf("locked factory has no Go mapping: %s", id)
		}
	}
}

func issue27FactorySymbol(symbol surface.Symbol) bool {
	ref := symbol.Upstream.Reference
	if strings.HasPrefix(ref, "packages/ai/src/api/") && strings.Contains(ref, ".lazy.ts#") && strings.HasSuffix(symbol.Name, "Api") {
		return true
	}
	return symbol.Kind == "function" && strings.HasPrefix(ref, "packages/ai/src/providers/") && strings.HasSuffix(symbol.Name, "Provider")
}

func issue27RepoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), ".."))
}
