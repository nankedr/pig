package ai

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

type ImagesAPIFunction = ImagesFunction

type ImagesAPIProvider struct {
	API            ImagesAPI
	GenerateImages ImagesAPIFunction
}

var imagesAPIRegistry = struct {
	sync.RWMutex
	providers map[ImagesAPI]ImagesAPIProvider
}{providers: map[ImagesAPI]ImagesAPIProvider{
	ImagesAPIOpenRouter: guardedImagesAPIProvider(ImagesAPIProvider{
		API:            ImagesAPIOpenRouter,
		GenerateImages: GenerateImagesOpenRouter,
	}),
}}

func RegisterImagesAPIProvider(provider ImagesAPIProvider, _ ...string) error {
	if provider.API == "" {
		return errors.New("image API must not be empty")
	}
	if provider.GenerateImages == nil {
		return errors.New("image API generate function must not be nil")
	}
	imagesAPIRegistry.Lock()
	imagesAPIRegistry.providers[provider.API] = guardedImagesAPIProvider(provider)
	imagesAPIRegistry.Unlock()
	return nil
}

func guardedImagesAPIProvider(provider ImagesAPIProvider) ImagesAPIProvider {
	registered := provider
	generate := provider.GenerateImages
	registered.GenerateImages = func(ctx context.Context, model ImagesModel, input ImagesContext, options ImagesOptions) (AssistantImages, error) {
		if model.API != provider.API {
			return AssistantImages{}, fmt.Errorf("%w: image API %q does not match registered API %q", ErrEventStreamInvariant, model.API, provider.API)
		}
		return generate(ctx, model, input, options)
	}
	return registered
}

func GetImagesAPIProvider(api ImagesAPI) (ImagesAPIProvider, bool) {
	imagesAPIRegistry.RLock()
	provider, ok := imagesAPIRegistry.providers[api]
	imagesAPIRegistry.RUnlock()
	return provider, ok
}

func GenerateImages(context.Context, ImagesModel, ImagesContext, ...ImagesOptions) (AssistantImages, error) {
	return AssistantImages{}, newNotImplemented("GenerateImages")
}

type ImagesRefreshFunction func(context.Context) ([]ImagesModel, error)

type ImagesProvider interface {
	ID() ImagesProviderID
	Name() string
	Auth() ProviderAuth
	GetModels() []ImagesModel
	SupportsRefreshModels() bool
	RefreshModels(context.Context) error
	GenerateImages(context.Context, ImagesModel, ImagesContext, ImagesOptions) (AssistantImages, error)
}

type CreateImagesProviderOptions struct {
	ID            ImagesProviderID
	Name          string
	Auth          ProviderAuth
	Models        []ImagesModel
	RefreshModels ImagesRefreshFunction
	API           ProviderImages
}

type CreatedImagesProvider struct {
	id     ImagesProviderID
	name   string
	auth   ProviderAuth
	models []ImagesModel
}

var _ ImagesProvider = (*CreatedImagesProvider)(nil)

func CreateImagesProvider(options CreateImagesProviderOptions) *CreatedImagesProvider {
	name := options.Name
	if name == "" {
		name = string(options.ID)
	}
	return &CreatedImagesProvider{
		id:     options.ID,
		name:   name,
		auth:   cloneProviderAuth(options.Auth),
		models: cloneImagesModels(options.Models),
	}
}

func (p *CreatedImagesProvider) ID() ImagesProviderID { return p.id }

func (p *CreatedImagesProvider) Name() string { return p.name }

func (p *CreatedImagesProvider) Auth() ProviderAuth { return cloneProviderAuth(p.auth) }

func (p *CreatedImagesProvider) GetModels() []ImagesModel { return cloneImagesModels(p.models) }

func (*CreatedImagesProvider) SupportsRefreshModels() bool { return false }

func (p *CreatedImagesProvider) RefreshModels(context.Context) error {
	return newNotImplemented("ImagesProvider.RefreshModels")
}

func (p *CreatedImagesProvider) GenerateImages(context.Context, ImagesModel, ImagesContext, ImagesOptions) (AssistantImages, error) {
	return AssistantImages{}, newNotImplemented("ImagesProvider.GenerateImages")
}

type ImagesModels interface {
	GetProviders() []ImagesProvider
	GetProvider(id ImagesProviderID) (ImagesProvider, bool)
	GetModels(provider ...ImagesProviderID) []ImagesModel
	GetModel(provider ImagesProviderID, id string) (ImagesModel, bool)
	Refresh(context.Context, ...ImagesProviderID) error
	GetProviderAuth(context.Context, ImagesProviderID, ...AuthResolutionOverrides) (Optional[AuthResult], error)
	GetModelAuth(context.Context, ImagesModel, ...AuthResolutionOverrides) (Optional[AuthResult], error)
	GenerateImages(context.Context, ImagesModel, ImagesContext, ...ImagesOptions) (AssistantImages, error)
}

type MutableImagesModels interface {
	ImagesModels
	SetProvider(ImagesProvider)
	DeleteProvider(ImagesProviderID)
	ClearProviders()
}

type imagesModels struct {
	mu        sync.RWMutex
	providers map[ImagesProviderID]ImagesProvider
	order     []ImagesProviderID
}

func CreateImagesModels(...CreateModelsOptions) MutableImagesModels {
	return &imagesModels{providers: make(map[ImagesProviderID]ImagesProvider)}
}

func (m *imagesModels) SetProvider(provider ImagesProvider) {
	if provider == nil || isNilRuntimeValue(provider) {
		return
	}
	id := provider.ID()
	m.mu.Lock()
	if _, exists := m.providers[id]; !exists {
		m.order = append(m.order, id)
	}
	m.providers[id] = provider
	m.mu.Unlock()
}

func (m *imagesModels) DeleteProvider(id ImagesProviderID) {
	m.mu.Lock()
	if _, exists := m.providers[id]; exists {
		delete(m.providers, id)
		for index, current := range m.order {
			if current == id {
				m.order = append(m.order[:index], m.order[index+1:]...)
				break
			}
		}
	}
	m.mu.Unlock()
}

func (m *imagesModels) ClearProviders() {
	m.mu.Lock()
	m.providers = make(map[ImagesProviderID]ImagesProvider)
	m.order = nil
	m.mu.Unlock()
}

func (m *imagesModels) GetProviders() []ImagesProvider {
	m.mu.RLock()
	providers := make([]ImagesProvider, 0, len(m.order))
	for _, id := range m.order {
		providers = append(providers, m.providers[id])
	}
	m.mu.RUnlock()
	return providers
}

func (m *imagesModels) GetProvider(id ImagesProviderID) (ImagesProvider, bool) {
	m.mu.RLock()
	provider, ok := m.providers[id]
	m.mu.RUnlock()
	return provider, ok
}

func (m *imagesModels) GetModels(provider ...ImagesProviderID) []ImagesModel {
	if len(provider) != 0 {
		entry, ok := m.GetProvider(provider[0])
		if !ok {
			return []ImagesModel{}
		}
		return bestEffortImagesProviderModels(entry)
	}
	models := make([]ImagesModel, 0)
	for _, entry := range m.GetProviders() {
		models = append(models, bestEffortImagesProviderModels(entry)...)
	}
	return models
}

func (m *imagesModels) GetModel(provider ImagesProviderID, id string) (ImagesModel, bool) {
	for _, model := range m.GetModels(provider) {
		if model.ID == id {
			return model, true
		}
	}
	return ImagesModel{}, false
}

func (*imagesModels) Refresh(context.Context, ...ImagesProviderID) error {
	return newNotImplemented("ImagesModels.Refresh")
}

func (*imagesModels) GetProviderAuth(context.Context, ImagesProviderID, ...AuthResolutionOverrides) (Optional[AuthResult], error) {
	return Absent[AuthResult](), newNotImplemented("ImagesModels.GetProviderAuth")
}

func (*imagesModels) GetModelAuth(context.Context, ImagesModel, ...AuthResolutionOverrides) (Optional[AuthResult], error) {
	return Absent[AuthResult](), newNotImplemented("ImagesModels.GetModelAuth")
}

func (*imagesModels) GenerateImages(context.Context, ImagesModel, ImagesContext, ...ImagesOptions) (AssistantImages, error) {
	return AssistantImages{}, newNotImplemented("ImagesModels.GenerateImages")
}

var builtinImageModelsByProvider = map[ImagesProviderID][]ImagesModel{}

func GetImageProviders() []ImagesProviderID {
	providers := make([]ImagesProviderID, 0, len(builtinImageModelsByProvider))
	for provider := range builtinImageModelsByProvider {
		providers = append(providers, provider)
	}
	return providers
}

func GetImageModels(provider ImagesProviderID) []ImagesModel {
	return cloneImagesModels(builtinImageModelsByProvider[provider])
}

func GetImageModel(provider ImagesProviderID, id string) (ImagesModel, bool) {
	for _, model := range builtinImageModelsByProvider[provider] {
		if model.ID == id {
			return cloneImagesModel(model), true
		}
	}
	return ImagesModel{}, false
}

func OpenRouterImagesAPI() ProviderImages { return NewStubProviderImages() }

func GenerateImagesOpenRouter(context.Context, ImagesModel, ImagesContext, ImagesOptions) (AssistantImages, error) {
	return AssistantImages{}, newNotImplemented("OpenRouterImages.GenerateImages")
}

func RegisterBuiltinImagesAPIProviders() error {
	return RegisterImagesAPIProvider(ImagesAPIProvider{
		API:            ImagesAPIOpenRouter,
		GenerateImages: GenerateImagesOpenRouter,
	})
}

func OpenRouterImagesProvider() ImagesProvider {
	return CreateImagesProvider(CreateImagesProviderOptions{
		ID:     ImagesProviderIDOpenRouter,
		Name:   "OpenRouter",
		Auth:   newBuiltinProviderAuth(ProviderIDOpenRouter),
		Models: GetImageModels(ImagesProviderIDOpenRouter),
		API:    OpenRouterImagesAPI(),
	})
}

func BuiltinImagesProviders() []ImagesProvider {
	return []ImagesProvider{OpenRouterImagesProvider()}
}

func BuiltinImagesModels(options ...CreateModelsOptions) MutableImagesModels {
	models := CreateImagesModels(options...)
	for _, provider := range BuiltinImagesProviders() {
		models.SetProvider(provider)
	}
	return models
}

func cloneImagesModels(models []ImagesModel) []ImagesModel {
	cloned := make([]ImagesModel, len(models))
	for index, model := range models {
		cloned[index] = cloneImagesModel(model)
	}
	return cloned
}

func cloneImagesModel(model ImagesModel) ImagesModel {
	if model.Input != nil {
		model.Input = append([]ModelInput{}, model.Input...)
	}
	if model.Output != nil {
		model.Output = append([]ModelInput{}, model.Output...)
	}
	if model.Cost.Tiers != nil {
		model.Cost.Tiers = append([]ModelCostTier{}, model.Cost.Tiers...)
	}
	model.ThinkingLevelMap = cloneThinkingLevelMap(model.ThinkingLevelMap)
	model.SamplingParams = cloneRawMessageMap(model.SamplingParams)
	model.Headers = cloneStringMap(model.Headers)
	return model
}

func bestEffortImagesProviderModels(provider ImagesProvider) (models []ImagesModel) {
	defer func() {
		if recover() != nil {
			models = []ImagesModel{}
		}
	}()
	return cloneImagesModels(provider.GetModels())
}

func cloneThinkingLevelMap(input ThinkingLevelMap) ThinkingLevelMap {
	if input == nil {
		return nil
	}
	output := make(ThinkingLevelMap, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func cloneStringMap(input map[string]string) map[string]string {
	if input == nil {
		return nil
	}
	output := make(map[string]string, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}
