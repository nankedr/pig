package ai_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nankedr/pig/ai"
)

func TestModelsOutcomeRegressionNilStreamOptionsBecomeTerminalInvariant(t *testing.T) {
	model := outcomeRegressionModel("nil-options-provider", "nil-options-model")
	var streamCalls atomic.Int32
	provider := ai.CreateProvider(ai.CreateProviderOptions{
		ID:   model.Provider,
		Auth: outcomeRegressionConfiguredAuth(),
		API: ai.SingleProviderAPI(ai.ProviderStreams{
			Stream: func(context.Context, ai.Model, ai.Context, ai.StreamOptions) *ai.AssistantMessageEventStream {
				streamCalls.Add(1)
				return completedOutcomeRegressionStream(model)
			},
		}),
	})
	models := ai.CreateModels()
	models.SetProvider(provider)

	var typedNil *ai.OpenAIResponsesOptions
	tests := []struct {
		name    string
		options []ai.ModelsStreamOption
	}{
		{name: "nil outer option", options: []ai.ModelsStreamOption{nil}},
		{
			name: "typed nil provider option",
			options: []ai.ModelsStreamOption{
				ai.ModelsAPIStreamOptions[*ai.OpenAIResponsesOptions]{Options: typedNil},
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := models.Stream(context.Background(), model, ai.Context{}, test.options...).Result(context.Background())
			if err != nil {
				t.Fatalf("Result() error = %v, want terminal invariant outcome", err)
			}
			outcomeRegressionAssertTerminalOutcome(t, result, model, ai.StopReasonError, string(ai.ModelsErrorCodeStream))
		})
	}
	if got := streamCalls.Load(); got != 0 {
		t.Fatalf("provider stream calls = %d, want zero for invalid options", got)
	}
}

func TestCreatedProviderTypedNilStreamOptionsBecomeTerminalInvariant(t *testing.T) {
	model := outcomeRegressionModel("direct-nil-options-provider", "direct-nil-options-model")
	var streamCalls atomic.Int32
	provider := ai.CreateProvider(ai.CreateProviderOptions{
		ID: model.Provider,
		API: ai.SingleProviderAPI(ai.ProviderStreams{
			Stream: func(context.Context, ai.Model, ai.Context, ai.StreamOptions) *ai.AssistantMessageEventStream {
				streamCalls.Add(1)
				return completedOutcomeRegressionStream(model)
			},
		}),
	})

	var options *ai.OpenAIResponsesOptions
	result, err := provider.Stream(context.Background(), model, ai.Context{}, options).Result(context.Background())
	if err != nil {
		t.Fatalf("Result() error = %v, want terminal invariant outcome", err)
	}
	outcomeRegressionAssertTerminalOutcome(t, result, model, ai.StopReasonError, string(ai.ModelsErrorCodeStream))
	if got := streamCalls.Load(); got != 0 {
		t.Fatalf("provider stream calls = %d, want zero for typed nil options", got)
	}
}

func TestModelsOutcomeRegressionStreamCancellationRetainsPartialContent(t *testing.T) {
	models := ai.CreateModels()
	model := outcomeRegressionModel("partial-provider", "partial-model")
	started := make(chan struct{})

	models.SetProvider(ai.CreateProvider(ai.CreateProviderOptions{
		ID: "partial-provider",
		Auth: ai.ProviderAuth{
			APIKey: &ai.APIKeyAuth{
				Name: "Configured",
				Resolve: func(context.Context, ai.APIKeyResolveInput) (ai.Optional[ai.AuthResult], error) {
					return ai.Some(ai.AuthResult{Auth: ai.ModelAuth{APIKey: ai.Some("key")}}), nil
				},
			},
		},
		Models: []ai.Model{model},
		API: ai.SingleProviderAPI(ai.ProviderStreams{
			Stream: func(ctx context.Context, got ai.Model, _ ai.Context, _ ai.StreamOptions) *ai.AssistantMessageEventStream {
				stream := ai.NewAssistantMessageEventStream()
				partial := ai.AssistantMessage{
					Role:     ai.MessageRoleAssistant,
					Content:  []ai.AssistantContent{ai.TextContent{Type: ai.ContentTypeText, Text: "partial text"}},
					API:      got.API,
					Provider: got.Provider,
					Model:    got.ID,
				}
				stream.Push(ai.AssistantMessageTextDeltaEvent{
					Type:         ai.AssistantMessageEventTypeTextDelta,
					ContentIndex: 0,
					Delta:        "partial text",
					Partial:      partial,
				})
				close(started)
				<-ctx.Done()
				return stream
			},
			StreamSimple: func(context.Context, ai.Model, ai.Context, ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
				t.Fatal("StreamSimple() should not be called")
				return nil
			},
		}),
	}))

	ctx, cancel := context.WithCancel(context.Background())
	stream := models.Stream(ctx, model, ai.Context{})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("inner stream did not publish a partial event before cancellation")
	}
	cancel()

	wait, doneCancel := context.WithTimeout(context.Background(), time.Second)
	defer doneCancel()
	result, err := stream.Result(wait)
	if err != nil {
		t.Fatalf("Stream().Result() error = %v, want terminal aborted outcome", err)
	}
	if result.StopReason != ai.StopReasonAborted {
		t.Fatalf("StopReason = %q, want %q", result.StopReason, ai.StopReasonAborted)
	}
	if len(result.Content) != 1 {
		t.Fatalf("Content = %#v, want the partial text content retained", result.Content)
	}
	text, ok := result.Content[0].(ai.TextContent)
	if !ok || text.Text != "partial text" {
		t.Fatalf("partial content = %#v, want TextContent(partial text)", result.Content[0])
	}
}

func TestModelsOutcomeRegressionCompleteCancellationReturnsAbortedOutcome(t *testing.T) {
	for _, test := range []struct {
		name     string
		complete func(ai.Models, context.Context, ai.Model) (ai.AssistantMessage, error)
	}{
		{
			name: "stream",
			complete: func(models ai.Models, ctx context.Context, model ai.Model) (ai.AssistantMessage, error) {
				return models.Complete(ctx, model, ai.Context{})
			},
		},
		{
			name: "simple",
			complete: func(models ai.Models, ctx context.Context, model ai.Model) (ai.AssistantMessage, error) {
				return models.CompleteSimple(ctx, model, ai.Context{})
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			models := ai.CreateModels()
			model := outcomeRegressionModel("complete-cancel-provider", "complete-cancel-model")
			models.SetProvider(ai.CreateProvider(ai.CreateProviderOptions{
				ID:   model.Provider,
				Auth: outcomeRegressionConfiguredAuth(),
				API: ai.SingleProviderAPI(ai.ProviderStreams{
					Stream: func(ctx context.Context, _ ai.Model, _ ai.Context, _ ai.StreamOptions) *ai.AssistantMessageEventStream {
						stream := ai.NewAssistantMessageEventStream()
						go func() { <-ctx.Done() }()
						return stream
					},
					StreamSimple: func(ctx context.Context, _ ai.Model, _ ai.Context, _ ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
						stream := ai.NewAssistantMessageEventStream()
						go func() { <-ctx.Done() }()
						return stream
					},
				}),
			}))

			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			result, err := test.complete(models, ctx, model)
			if err != nil {
				t.Fatalf("Complete() error = %v, want nil terminal outcome", err)
			}
			if result.StopReason != ai.StopReasonAborted {
				t.Fatalf("Complete() StopReason = %q, want %q", result.StopReason, ai.StopReasonAborted)
			}
		})
	}
}

func TestModelsOutcomeRegressionFetchDeferredCancellationReturnsAbortedOutcome(t *testing.T) {
	models := ai.CreateModels()
	model := outcomeRegressionModel("deferred-cancel-provider", "deferred-cancel-model")
	handle := ai.DeferredHandle{Provider: model.Provider, ModelID: model.ID, API: model.API, ID: "job-cancel"}
	started := make(chan struct{})
	models.SetProvider(ai.CreateProvider(ai.CreateProviderOptions{
		ID:   model.Provider,
		Auth: outcomeRegressionConfiguredAuth(),
		API: ai.SingleProviderAPI(ai.ProviderStreams{
			FetchDeferred: func(ctx context.Context, _ ai.Model, _ ai.DeferredHandle, _ ai.DeferredFetchOptions) (*ai.AssistantMessageEventStream, error) {
				stream := ai.NewAssistantMessageEventStream()
				close(started)
				go func() { <-ctx.Done() }()
				return stream, nil
			},
		}),
	}))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct {
		message ai.AssistantMessage
		err     error
	}, 1)
	go func() {
		message, err := models.FetchDeferred(ctx, model, handle)
		done <- struct {
			message ai.AssistantMessage
			err     error
		}{message: message, err: err}
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("deferred provider stream did not start")
	}
	cancel()
	select {
	case result := <-done:
		if result.err != nil {
			t.Fatalf("FetchDeferred() error = %v, want terminal aborted outcome", result.err)
		}
		outcomeRegressionAssertTerminalOutcome(t, result.message, model, ai.StopReasonAborted, "request canceled")
	case <-time.After(time.Second):
		t.Fatal("FetchDeferred() did not return after cancellation")
	}
}

func TestModelsOutcomeRegressionFetchDeferredSetupAndProviderErrorsBecomeTerminalOutcome(t *testing.T) {
	t.Run("provider fetch error becomes terminal outcome", func(t *testing.T) {
		models := ai.CreateModels()
		model := outcomeRegressionModel("fetch-provider", "deferred-model")
		handle := ai.DeferredHandle{Provider: model.Provider, ModelID: model.ID, API: model.API, ID: "job-1"}
		providerErr := errors.New("provider fetch failed")

		models.SetProvider(ai.CreateProvider(ai.CreateProviderOptions{
			ID: "fetch-provider",
			Auth: ai.ProviderAuth{
				APIKey: &ai.APIKeyAuth{
					Name: "Configured",
					Resolve: func(context.Context, ai.APIKeyResolveInput) (ai.Optional[ai.AuthResult], error) {
						return ai.Some(ai.AuthResult{Auth: ai.ModelAuth{APIKey: ai.Some("key")}}), nil
					},
				},
			},
			Models: []ai.Model{model},
			API: ai.SingleProviderAPI(ai.ProviderStreams{
				Stream: func(context.Context, ai.Model, ai.Context, ai.StreamOptions) *ai.AssistantMessageEventStream {
					return completedOutcomeRegressionStream(model)
				},
				StreamSimple: func(context.Context, ai.Model, ai.Context, ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
					return completedOutcomeRegressionStream(model)
				},
				FetchDeferred: func(context.Context, ai.Model, ai.DeferredHandle, ai.DeferredFetchOptions) (*ai.AssistantMessageEventStream, error) {
					return nil, providerErr
				},
			}),
		}))

		result, err := models.FetchDeferred(context.Background(), model, handle)
		if err != nil {
			t.Fatalf("FetchDeferred(provider error) error = %v, want nil terminal outcome", err)
		}
		outcomeRegressionAssertTerminalOutcome(t, result, model, ai.StopReasonError, "provider: request failed")
	})

	t.Run("setup auth error becomes terminal outcome", func(t *testing.T) {
		models := ai.CreateModels()
		model := outcomeRegressionModel("auth-provider", "deferred-model")
		handle := ai.DeferredHandle{Provider: model.Provider, ModelID: model.ID, API: model.API, ID: "job-2"}

		models.SetProvider(ai.CreateProvider(ai.CreateProviderOptions{
			ID: "auth-provider",
			Auth: ai.ProviderAuth{
				APIKey: &ai.APIKeyAuth{
					Name: "Missing key",
					Resolve: func(context.Context, ai.APIKeyResolveInput) (ai.Optional[ai.AuthResult], error) {
						return ai.Absent[ai.AuthResult](), nil
					},
				},
			},
			Models: []ai.Model{model},
			API: ai.SingleProviderAPI(ai.ProviderStreams{
				Stream: func(context.Context, ai.Model, ai.Context, ai.StreamOptions) *ai.AssistantMessageEventStream {
					return completedOutcomeRegressionStream(model)
				},
				StreamSimple: func(context.Context, ai.Model, ai.Context, ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
					return completedOutcomeRegressionStream(model)
				},
				FetchDeferred: func(context.Context, ai.Model, ai.DeferredHandle, ai.DeferredFetchOptions) (*ai.AssistantMessageEventStream, error) {
					t.Fatal("FetchDeferred() reached provider despite unresolved auth")
					return nil, nil
				},
			}),
		}))

		result, err := models.FetchDeferred(context.Background(), model, handle)
		if err != nil {
			t.Fatalf("FetchDeferred(setup error) error = %v, want nil terminal outcome", err)
		}
		outcomeRegressionAssertTerminalOutcome(t, result, model, ai.StopReasonError, "Provider is not configured")
	})

	t.Run("stub not implemented stays reachable", func(t *testing.T) {
		models := ai.CreateModels()
		model := outcomeRegressionModel("stubbed-provider", "deferred-model")
		handle := ai.DeferredHandle{Provider: model.Provider, ModelID: model.ID, API: model.API, ID: "job-3"}

		models.SetProvider(ai.CreateProvider(ai.CreateProviderOptions{
			ID:   "stubbed-provider",
			Auth: outcomeRegressionConfiguredAuth(),
			API:  ai.SingleProviderAPI(ai.NewStubProviderStreams()),
		}))

		_, err := models.FetchDeferred(context.Background(), model, handle)
		if !errors.Is(err, ai.ErrNotImplemented) {
			t.Fatalf("FetchDeferred(stub) error = %v, want ErrNotImplemented reachable", err)
		}
	})
}

func TestModelsOutcomeRegressionTerminalErrorsRedactInjectedSecrets(t *testing.T) {
	const secret = "sk-live-should-never-enter-session"
	models := ai.CreateModels()
	model := outcomeRegressionModel("redaction-provider", "redaction-model")
	models.SetProvider(ai.CreateProvider(ai.CreateProviderOptions{
		ID: model.Provider,
		Auth: ai.ProviderAuth{APIKey: &ai.APIKeyAuth{
			Name: "Failing auth",
			Resolve: func(context.Context, ai.APIKeyResolveInput) (ai.Optional[ai.AuthResult], error) {
				return ai.Absent[ai.AuthResult](), errors.New("upstream rejected token " + secret)
			},
		}},
		API: ai.SingleProviderAPI(recordingProviderStreams(func(context.Context, ai.Model, ai.Context, ai.StreamOptions) *ai.AssistantMessageEventStream {
			t.Fatal("provider stream reached after auth failure")
			return nil
		}, nil)),
	}))

	result, err := models.Complete(context.Background(), model, ai.Context{})
	if err != nil {
		t.Fatalf("Complete() error = %v, want terminal outcome", err)
	}
	message, ok := result.ErrorMessage.Value()
	if !ok || message != "auth: API key auth failed for provider redaction-provider" {
		t.Fatalf("redacted ErrorMessage = (%q, %t), want stable auth classification", message, ok)
	}
	if contains(message, secret) {
		t.Fatalf("terminal ErrorMessage leaked injected secret: %q", message)
	}
}

func TestModelsOutcomeRegressionTerminalErrorsRedactExternalModelsErrorFields(t *testing.T) {
	const secret = "sk-external-models-error-secret"
	models := ai.CreateModels()
	model := outcomeRegressionModel("external-models-error-provider", "external-models-error-model")
	models.SetProvider(ai.CreateProvider(ai.CreateProviderOptions{
		ID:   model.Provider,
		Auth: outcomeRegressionConfiguredAuth(),
		API: ai.SingleProviderAPI(ai.ProviderStreams{
			Stream: func(context.Context, ai.Model, ai.Context, ai.StreamOptions) *ai.AssistantMessageEventStream {
				stream := ai.NewAssistantMessageEventStream()
				message := ai.AssistantMessage{
					Role:       ai.MessageRoleAssistant,
					API:        model.API,
					Provider:   model.Provider,
					Model:      model.ID,
					StopReason: ai.StopReasonError,
					Timestamp:  time.Now().UnixMilli(),
				}
				stream.Push(ai.AssistantMessageErrorEvent{Type: ai.AssistantMessageEventTypeError, Reason: ai.StopReasonError, Error: message})
				stream.End(message)
				return stream
			},
		}),
	}))

	configured := ai.ModelsStreamOptions{ModelsRequestTransforms: ai.ModelsRequestTransforms{
		TransformHeaders: func(context.Context, ai.ProviderHeaders) (ai.ProviderHeaders, error) {
			return nil, &ai.ModelsError{Code: ai.ModelsErrorCodeProvider, Message: secret, Cause: errors.New(secret)}
		},
	}}
	result, err := models.Complete(context.Background(), model, ai.Context{}, configured)
	if err != nil {
		t.Fatalf("Complete() error = %v, want redacted terminal outcome", err)
	}
	message, ok := result.ErrorMessage.Value()
	if !ok || message != "provider: request failed" {
		t.Fatalf("ErrorMessage = (%q, %t), want external ModelsError fallback", message, ok)
	}
	if contains(message, secret) {
		t.Fatalf("terminal ErrorMessage leaked external ModelsError fields: %q", message)
	}
}

func TestModelsOutcomeRegressionUndeclaredDeferredCapabilitiesAreStructuredNotImplemented(t *testing.T) {
	modelA := outcomeRegressionModel("undeclared-provider", "model-a")
	handle := ai.DeferredHandle{Provider: modelA.Provider, ModelID: modelA.ID, API: modelA.API, ID: "job-4"}
	provider := ai.CreateProvider(ai.CreateProviderOptions{
		ID:   "undeclared-provider",
		Auth: outcomeRegressionConfiguredAuth(),
		API: ai.SingleProviderAPI(ai.ProviderStreams{
			Stream: func(context.Context, ai.Model, ai.Context, ai.StreamOptions) *ai.AssistantMessageEventStream {
				return completedOutcomeRegressionStream(modelA)
			},
			StreamSimple: func(context.Context, ai.Model, ai.Context, ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
				return completedOutcomeRegressionStream(modelA)
			},
		}),
	})

	if stream, err := provider.FetchDeferred(context.Background(), modelA, handle, ai.DeferredFetchOptions{}); stream != nil {
		t.Fatalf("provider.FetchDeferred() stream = %p, want nil", stream)
	} else if !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("provider.FetchDeferred() error = %v, want structured ErrNotImplemented", err)
	}
	if err := provider.CancelDeferred(context.Background(), modelA, handle, ai.DeferredCancelOptions{}); !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("provider.CancelDeferred() error = %v, want structured ErrNotImplemented", err)
	}
	models := ai.CreateModels()
	models.SetProvider(provider)
	if err := models.CancelDeferred(context.Background(), modelA, handle); !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("models.CancelDeferred() error = %v, want structured ErrNotImplemented", err)
	}
}

func TestModelsOutcomeRegressionBuiltinStubPreservesNotImplementedIdentity(t *testing.T) {
	models := ai.BuiltinModels()
	model := outcomeRegressionModel(ai.ProviderIDOpenAI, "pending-capture-model")
	model.API = ai.APIOpenAIResponses

	apiKey := "synthetic-key"
	stream := models.Stream(context.Background(), model, ai.Context{}, ai.ModelsStreamOptions{
		StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{APIKey: &apiKey}},
	})
	if _, err := stream.Result(context.Background()); !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("BuiltinModels().Stream() error = %v, want ErrNotImplemented", err)
	}
}

func TestModelsOutcomeRegressionStubProviderStreamsDoNotAdvertiseDeferredCapabilities(t *testing.T) {
	model := outcomeRegressionModel("stub-capability-provider", "model-a")
	handle := ai.DeferredHandle{Provider: model.Provider, ModelID: model.ID, API: model.API, ID: "job-5"}
	provider := ai.CreateProvider(ai.CreateProviderOptions{
		ID:   "stub-capability-provider",
		Auth: outcomeRegressionConfiguredAuth(),
		API:  ai.SingleProviderAPI(ai.NewStubProviderStreams()),
	})

	if provider.SupportsFetchDeferred() {
		t.Fatal("SupportsFetchDeferred() = true, want false for NewStubProviderStreams capability probe")
	}
	if provider.SupportsCancelDeferred() {
		t.Fatal("SupportsCancelDeferred() = true, want false for NewStubProviderStreams capability probe")
	}

	if stream, err := provider.FetchDeferred(context.Background(), model, handle, ai.DeferredFetchOptions{}); stream != nil {
		t.Fatalf("stub provider FetchDeferred() stream = %p, want nil", stream)
	} else if !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("stub provider FetchDeferred() error = %v, want ErrNotImplemented", err)
	}
	if err := provider.CancelDeferred(context.Background(), model, handle, ai.DeferredCancelOptions{}); !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("stub provider CancelDeferred() error = %v, want ErrNotImplemented", err)
	}
}

func outcomeRegressionConfiguredAuth() ai.ProviderAuth {
	return ai.ProviderAuth{
		APIKey: &ai.APIKeyAuth{
			Name: "Configured",
			Resolve: func(context.Context, ai.APIKeyResolveInput) (ai.Optional[ai.AuthResult], error) {
				return ai.Some(ai.AuthResult{Auth: ai.ModelAuth{APIKey: ai.Some("key")}}), nil
			},
		},
	}
}

func completedOutcomeRegressionStream(model ai.Model) *ai.AssistantMessageEventStream {
	stream := ai.NewAssistantMessageEventStream()
	stream.Push(ai.AssistantMessageDoneEvent{
		Type:   ai.AssistantMessageEventTypeDone,
		Reason: ai.StopReasonStop,
		Message: ai.AssistantMessage{
			Role:       ai.MessageRoleAssistant,
			Content:    []ai.AssistantContent{},
			API:        model.API,
			Provider:   model.Provider,
			Model:      model.ID,
			StopReason: ai.StopReasonStop,
			Timestamp:  time.Now().UnixMilli(),
		},
	})
	return stream
}

func outcomeRegressionAssertTerminalOutcome(
	t *testing.T,
	message ai.AssistantMessage,
	model ai.Model,
	reason ai.StopReason,
	wantSubstring string,
) {
	t.Helper()
	if message.Role != ai.MessageRoleAssistant || message.StopReason != reason ||
		message.Provider != model.Provider || message.Model != model.ID || message.API != model.API {
		t.Fatalf("terminal outcome = %#v, want assistant %q outcome for %#v", message, reason, model)
	}
	errorMessage, ok := message.ErrorMessage.Value()
	if !ok || !contains(errorMessage, wantSubstring) {
		t.Fatalf("ErrorMessage = (%q, %t), want substring %q", errorMessage, ok, wantSubstring)
	}
}

func outcomeRegressionModel(provider ai.ProviderID, id string) ai.Model {
	return ai.Model{
		ID:       id,
		Name:     id,
		API:      ai.API("outcome-regression-api"),
		Provider: provider,
	}
}

func contains(haystack, needle string) bool {
	return len(needle) == 0 || stringContains(haystack, needle)
}

func stringContains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
