package parity_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
)

type usageParityInput struct {
	API       ai.API             `json:"api"`
	Provider  ai.ProviderID      `json:"provider"`
	ModelID   string             `json:"model_id"`
	Response  string             `json:"response"`
	Context   usageParityContext `json:"context"`
	ModelCost ai.ModelCostRates  `json:"model_cost"`
	Requests  []struct {
		ID             string             `json:"id"`
		Entrypoint     string             `json:"entrypoint"`
		SessionID      string             `json:"sessionId"`
		CacheRetention *ai.CacheRetention `json:"cacheRetention"`
	} `json:"requests"`
	CostCases []struct {
		ID    string   `json:"id"`
		Model ai.Model `json:"model"`
		Usage ai.Usage `json:"usage"`
	} `json:"cost_cases"`
	OpenAI struct {
		Model ai.Model `json:"model"`
		Cases []struct {
			ID  string `json:"id"`
			SSE string `json:"sse"`
		} `json:"cases"`
	} `json:"openai"`
}

type usageParityContext struct {
	SystemPrompt string `json:"systemPrompt"`
	Messages     []struct {
		Role      ai.MessageRole `json:"role"`
		Content   string         `json:"content"`
		Timestamp int64          `json:"timestamp"`
	} `json:"messages"`
}

func TestUsageCostCacheParity(t *testing.T) {
	root := parityRepoRoot(t)
	baselineDir := filepath.Join(root, "parity", "baseline")
	if err := baseline.Verify(baselineDir); err != nil {
		t.Fatal(err)
	}
	lock, _, err := baseline.Load(baselineDir)
	if err != nil {
		t.Fatal(err)
	}
	locked := parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository}
	fixture, err := parity.LoadFixture(filepath.Join(root, "parity", "oracle", "fixtures", "usage-cost-cache.json"), locked)
	if err != nil {
		t.Fatal(err)
	}
	oracle, err := parity.NewFixtureDriver(fixture, locked)
	if err != nil {
		t.Fatal(err)
	}
	pig := parity.DriverFunc{SurfaceName: parity.SurfaceGoSDK, ObserveFunc: observeUsageCostCache}
	result, err := parity.RunCase(context.Background(), fixture.Case, oracle, pig)
	if err != nil {
		t.Fatalf("RunCase() = %v; differences=%+v", err, result.Differences)
	}
	if !result.Match || len(result.Normalizations) != 0 {
		t.Fatalf("parity result = %+v", result)
	}
}

func observeUsageCostCache(ctx context.Context, declaration parity.Case) (parity.Observation, error) {
	var input usageParityInput
	if err := json.Unmarshal(declaration.Input, &input); err != nil {
		return parity.Observation{}, err
	}
	message, err := ai.FauxAssistantMessage(ai.FauxAssistantText(input.Response), ai.FauxAssistantMessageOptions{Timestamp: ai.Some[int64](1)})
	if err != nil {
		return parity.Observation{}, err
	}
	handle, err := ai.NewFauxProvider(ai.RegisterFauxProviderOptions{
		API: input.API, Provider: input.Provider,
		Models: []ai.FauxModelDefinition{{ID: input.ModelID, Cost: ai.Some(input.ModelCost)}},
	})
	if err != nil {
		return parity.Observation{}, err
	}
	responses := make([]ai.FauxResponseStep, len(input.Requests))
	for i := range responses {
		responses[i] = message
	}
	handle.SetResponses(responses)
	model, ok := handle.GetModel(input.ModelID)
	if !ok {
		return parity.Observation{}, fmt.Errorf("Faux model %q not found", input.ModelID)
	}
	models := ai.CreateModels()
	models.SetProvider(handle.Provider)
	modelContext := ai.Context{SystemPrompt: ai.Some(input.Context.SystemPrompt), Messages: make([]ai.Message, len(input.Context.Messages))}
	for i, message := range input.Context.Messages {
		modelContext.Messages[i] = ai.UserMessage{Role: message.Role, Content: ai.UserText(message.Content), Timestamp: message.Timestamp}
	}

	requests := make(map[string]any, len(input.Requests))
	for _, request := range input.Requests {
		options := ai.StreamOptions{SessionID: &request.SessionID, CacheRetention: request.CacheRetention}
		if request.Entrypoint == "complete" {
			message, err := models.Complete(ctx, model, modelContext, ai.ModelsStreamOptions{StreamOptions: options})
			if err != nil {
				return parity.Observation{}, err
			}
			requests[request.ID] = map[string]any{"usage": message.Usage}
			continue
		}
		observed, err := observeUsageStream(ctx, handle.Provider.Stream(ctx, model, modelContext, options))
		if err != nil {
			return parity.Observation{}, err
		}
		requests[request.ID] = observed
	}
	costs := make(map[string]any, len(input.CostCases))
	for _, item := range input.CostCases {
		usage := item.Usage
		cost := ai.CalculateCost(item.Model, &usage)
		costs[item.ID] = map[string]any{"usage": usage, "cost": cost}
	}
	key := "test-key"
	openAI := make(map[string]any, len(input.OpenAI.Cases))
	for _, item := range input.OpenAI.Cases {
		observed, err := observeUsageStream(ctx, ai.StreamOpenAICompletions(ctx, input.OpenAI.Model, ai.Context{}, ai.OpenAICompletionsOptions{
			StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{
				APIKey: &key, Fetch: func(context.Context, ai.FetchRequest) (ai.FetchResponse, error) {
					return ai.FetchResponse{Status: http.StatusOK, BodyReader: io.NopCloser(strings.NewReader(item.SSE))}, nil
				},
			}},
		}))
		if err != nil {
			return parity.Observation{}, err
		}
		openAI[item.ID] = observed
	}
	outcome, err := json.Marshal(map[string]any{"requests": requests, "costs": costs, "openai": openAI})
	if err != nil {
		return parity.Observation{}, err
	}
	sideEffects := []parity.SideEffect{}
	return parity.Observation{Outcome: outcome, SideEffects: &sideEffects}, nil
}

func observeUsageStream(ctx context.Context, stream *ai.AssistantMessageEventStream) (map[string]any, error) {
	var terminal ai.Usage
	for {
		event, ok, err := stream.Next(ctx)
		if err != nil {
			return nil, err
		}
		if !ok {
			break
		}
		if done, ok := event.(ai.AssistantMessageDoneEvent); ok {
			terminal = done.Message.Usage
		}
	}
	result, err := stream.Result(ctx)
	if err != nil {
		return nil, err
	}
	repeated, err := stream.Result(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"usage": result.Usage, "terminal_equal": reflect.DeepEqual(terminal, result.Usage),
		"repeat_equal": reflect.DeepEqual(repeated.Usage, result.Usage),
	}, nil
}
