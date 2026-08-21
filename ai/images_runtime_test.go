package ai_test

import (
	"context"
	"errors"
	"testing"

	"github.com/nankedr/pig/ai"
)

var (
	_ ai.ImagesAPIFunction   = nil                                  // upstream: ImagesApiFunction
	_                        = ai.ImagesAPIProvider{}               // upstream: ImagesApiProvider
	_                        = ai.RegisterImagesAPIProvider         // upstream: registerImagesApiProvider
	_                        = ai.GetImagesAPIProvider              // upstream: getImagesApiProvider
	_                        = ai.CreateImagesProviderOptions{}     // upstream: CreateImagesProviderOptions
	_ ai.ImagesModels        = nil                                  // upstream: ImagesModels
	_ ai.ImagesProvider      = nil                                  // upstream: ImagesProvider
	_ ai.MutableImagesModels = nil                                  // upstream: MutableImagesModels
	_                        = ai.CreateImagesModels                // upstream: createImagesModels
	_                        = ai.CreateImagesProvider              // upstream: createImagesProvider
	_                        = ai.GenerateImages                    // upstream: ./images#generateImages
	_                        = ai.GetImageProviders                 // upstream: getImageProviders
	_                        = ai.GetImageModels                    // upstream: getImageModels
	_                        = ai.GetImageModel                     // upstream: getImageModel
	_                        = ai.OpenRouterImagesAPI               // upstream: openrouterImagesApi
	_                        = ai.GenerateImagesOpenRouter          // upstream: ./api/openrouter-images#generateImages
	_                        = ai.GenerateImagesOpenRouter          // upstream: generateImagesOpenRouter
	_                        = ai.RegisterBuiltinImagesAPIProviders // upstream: registerBuiltInImagesApiProviders
	_                        = ai.OpenRouterImagesProvider          // upstream: openrouterImagesProvider
	_                        = ai.BuiltinImagesProviders            // upstream: builtinImagesProviders
	_                        = ai.BuiltinImagesModels               // upstream: builtinImagesModels
)

func TestCreateImagesProviderKeepsCloneSafeMetadataAndStubsExternalWork(t *testing.T) {
	t.Parallel()

	input := ai.ImagesModel{
		ID:       "image-1",
		Name:     "Image One",
		API:      ai.ImagesAPIOpenRouter,
		Provider: ai.ImagesProviderIDOpenRouter,
		Input:    []ai.ModelInput{ai.ModelInputText},
		Output:   []ai.ModelInput{ai.ModelInputImage},
		Headers:  map[string]string{"X-Model": "original"},
		Cost: ai.ModelCost{Tiers: []ai.ModelCostTier{{
			InputTokensAbove: 1_000,
		}}},
	}
	apiCalls := 0
	refreshCalls := 0
	provider := ai.CreateImagesProvider(ai.CreateImagesProviderOptions{
		ID:     ai.ImagesProviderIDOpenRouter,
		Name:   "OpenRouter",
		Auth:   ai.ProviderAuth{APIKey: ptr(ai.NewStubAPIKeyAuth("OpenRouter API key"))},
		Models: []ai.ImagesModel{input},
		API: imagesFunction(func(context.Context, ai.ImagesModel, ai.ImagesContext, ai.ImagesOptions) (ai.AssistantImages, error) {
			apiCalls++
			return ai.AssistantImages{}, nil
		}),
		RefreshModels: func(context.Context) ([]ai.ImagesModel, error) {
			refreshCalls++
			return nil, nil
		},
	})

	input.Name = "mutated"
	input.Input[0] = ai.ModelInputImage
	input.Headers["X-Model"] = "mutated"
	input.Cost.Tiers[0].InputTokensAbove = 2_000
	first := provider.GetModels()
	if len(first) != 1 || first[0].Name != "Image One" || first[0].Input[0] != ai.ModelInputText || first[0].Headers["X-Model"] != "original" || first[0].Cost.Tiers[0].InputTokensAbove != 1_000 {
		t.Fatalf("provider observed mutated input: %#v", first)
	}
	first[0].Name = "mutated output"
	first[0].Headers["X-Model"] = "mutated output"
	first[0].Cost.Tiers[0].InputTokensAbove = 3_000
	if second := provider.GetModels(); second[0].Name != "Image One" || second[0].Headers["X-Model"] != "original" || second[0].Cost.Tiers[0].InputTokensAbove != 1_000 {
		t.Fatalf("provider leaked model snapshot: %#v", second)
	}
	if provider.SupportsRefreshModels() {
		t.Fatal("stub image provider advertises refresh support")
	}

	invoked := 0
	_, err := provider.GenerateImages(context.Background(), first[0], ai.ImagesContext{}, ai.ImagesOptions{
		ProviderRequestOptions: poisonedRequestOptions(&invoked),
	})
	if !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("GenerateImages error = %v, want ErrNotImplemented", err)
	}
	if err := provider.RefreshModels(context.Background()); !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("RefreshModels error = %v, want ErrNotImplemented", err)
	}
	if apiCalls != 0 || refreshCalls != 0 || invoked != 0 {
		t.Fatalf("stub side effects = api %d/refresh %d/hooks %d, want zero", apiCalls, refreshCalls, invoked)
	}
}

func TestImagesModelsProvidesPureInMemoryRegistry(t *testing.T) {
	t.Parallel()

	models := ai.CreateImagesModels()
	provider := ai.OpenRouterImagesProvider()
	models.SetProvider(provider)
	if got, ok := models.GetProvider(ai.ImagesProviderIDOpenRouter); !ok || got.ID() != ai.ImagesProviderIDOpenRouter {
		t.Fatalf("GetProvider() = (%v, %t)", got, ok)
	}
	if providers := models.GetProviders(); len(providers) != 1 || providers[0].ID() != ai.ImagesProviderIDOpenRouter {
		t.Fatalf("GetProviders() = %#v", providers)
	}
	if got := models.GetModels(); got == nil || len(got) != 0 {
		t.Fatalf("GetModels() = %#v, want pending-capture empty snapshot", got)
	}
	if _, err := models.GetProviderAuth(context.Background(), ai.ImagesProviderIDOpenRouter); !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("GetProviderAuth error = %v, want ErrNotImplemented", err)
	}
	if _, err := models.GetModelAuth(context.Background(), ai.ImagesModel{Provider: ai.ImagesProviderIDOpenRouter}); !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("GetModelAuth error = %v, want ErrNotImplemented", err)
	}
	if err := models.Refresh(context.Background(), ai.ImagesProviderIDOpenRouter); !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("Refresh error = %v, want ErrNotImplemented", err)
	}
	if _, err := models.GenerateImages(context.Background(), ai.ImagesModel{}, ai.ImagesContext{}); !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("GenerateImages error = %v, want ErrNotImplemented", err)
	}
	models.DeleteProvider(ai.ImagesProviderIDOpenRouter)
	if len(models.GetProviders()) != 0 {
		t.Fatal("DeleteProvider did not remove provider")
	}
	models.SetProvider(provider)
	models.ClearProviders()
	if len(models.GetProviders()) != 0 {
		t.Fatal("ClearProviders did not empty registry")
	}
}

func TestImageAPIRegistryIsInMemoryButGenerationRemainsStubbed(t *testing.T) {
	apiCalls := 0
	registered := ai.ImagesAPIProvider{
		API: ai.ImagesAPI("test-images"),
		GenerateImages: func(context.Context, ai.ImagesModel, ai.ImagesContext, ai.ImagesOptions) (ai.AssistantImages, error) {
			apiCalls++
			return ai.AssistantImages{}, nil
		},
	}
	if err := ai.RegisterImagesAPIProvider(registered); err != nil {
		t.Fatalf("RegisterImagesAPIProvider error = %v", err)
	}
	got, ok := ai.GetImagesAPIProvider(registered.API)
	if !ok || got.API != registered.API || got.GenerateImages == nil {
		t.Fatalf("GetImagesAPIProvider() = (%#v, %t)", got, ok)
	}
	if _, err := got.GenerateImages(context.Background(), ai.ImagesModel{API: "other-images"}, ai.ImagesContext{}, ai.ImagesOptions{}); !errors.Is(err, ai.ErrEventStreamInvariant) {
		t.Fatalf("registered image API mismatch error = %v, want ErrEventStreamInvariant", err)
	}

	_, err := ai.GenerateImages(context.Background(), ai.ImagesModel{API: registered.API}, ai.ImagesContext{})
	if !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("GenerateImages error = %v, want ErrNotImplemented", err)
	}
	if apiCalls != 0 {
		t.Fatalf("GenerateImages invoked registered API %d times, want zero", apiCalls)
	}
}

func TestImagesModelsIsolatesProviderModelPanics(t *testing.T) {
	t.Parallel()

	models := ai.CreateImagesModels()
	models.SetProvider(panicImagesProvider{})
	if got := models.GetModels(); len(got) != 0 {
		t.Fatalf("GetModels after provider panic = %#v, want empty", got)
	}
	if got := models.GetModels("panic-images"); got == nil || len(got) != 0 {
		t.Fatalf("scoped GetModels after provider panic = %#v, want empty", got)
	}
}

func TestImagesModelClonePreservesPresentEmptySlices(t *testing.T) {
	t.Parallel()

	provider := ai.CreateImagesProvider(ai.CreateImagesProviderOptions{
		ID: "empty-slices",
		Models: []ai.ImagesModel{{
			ID:     "empty",
			Input:  []ai.ModelInput{},
			Output: []ai.ModelInput{},
			Cost:   ai.ModelCost{Tiers: []ai.ModelCostTier{}},
		}},
	})
	model := provider.GetModels()[0]
	if model.Input == nil || model.Output == nil || model.Cost.Tiers == nil {
		t.Fatalf("present empty slices collapsed to nil: %#v", model)
	}
}

func TestBuiltinImageEntriesRemainPendingCaptureAndSideEffectFree(t *testing.T) {
	t.Parallel()

	apiProvider, ok := ai.GetImagesAPIProvider(ai.ImagesAPIOpenRouter)
	if !ok || apiProvider.GenerateImages == nil {
		t.Fatal("built-in OpenRouter image API is not registered")
	}
	if err := ai.RegisterBuiltinImagesAPIProviders(); err != nil {
		t.Fatalf("RegisterBuiltinImagesAPIProviders error = %v", err)
	}
	if providers := ai.GetImageProviders(); len(providers) != 0 {
		t.Fatalf("GetImageProviders() = %#v, want pending-capture empty snapshot", providers)
	}
	if models := ai.GetImageModels(ai.ImagesProviderIDOpenRouter); len(models) != 0 {
		t.Fatalf("GetImageModels() = %#v, want empty", models)
	}
	if _, ok := ai.GetImageModel(ai.ImagesProviderIDOpenRouter, "missing"); ok {
		t.Fatal("GetImageModel found a model in pending-capture snapshot")
	}

	providers := ai.BuiltinImagesProviders()
	if len(providers) != 1 || providers[0].ID() != ai.ImagesProviderIDOpenRouter {
		t.Fatalf("BuiltinImagesProviders() = %#v", providers)
	}
	models := ai.BuiltinImagesModels()
	if _, ok := models.GetProvider(ai.ImagesProviderIDOpenRouter); !ok {
		t.Fatal("BuiltinImagesModels omitted OpenRouter")
	}

	invoked := 0
	_, err := ai.OpenRouterImagesAPI().GenerateImages(
		context.Background(), ai.ImagesModel{}, ai.ImagesContext{},
		ai.ImagesOptions{ProviderRequestOptions: poisonedRequestOptions(&invoked)},
	)
	if !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("OpenRouterImagesAPI error = %v, want ErrNotImplemented", err)
	}
	if invoked != 0 {
		t.Fatalf("OpenRouterImagesAPI invoked hooks %d times, want zero", invoked)
	}
}

type imagesFunction ai.ImagesFunction

func (f imagesFunction) GenerateImages(ctx context.Context, model ai.ImagesModel, input ai.ImagesContext, options ai.ImagesOptions) (ai.AssistantImages, error) {
	return f(ctx, model, input, options)
}

type panicImagesProvider struct{}

func (panicImagesProvider) ID() ai.ImagesProviderID { return "panic-images" }
func (panicImagesProvider) Name() string            { return "Panic Images" }
func (panicImagesProvider) Auth() ai.ProviderAuth   { return ai.ProviderAuth{} }
func (panicImagesProvider) GetModels() []ai.ImagesModel {
	panic("model source panic")
}
func (panicImagesProvider) SupportsRefreshModels() bool { return false }
func (panicImagesProvider) RefreshModels(context.Context) error {
	return nil
}
func (panicImagesProvider) GenerateImages(context.Context, ai.ImagesModel, ai.ImagesContext, ai.ImagesOptions) (ai.AssistantImages, error) {
	return ai.AssistantImages{}, nil
}
