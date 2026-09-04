package parity_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/internal/baseline"
	"github.com/nankedr/pig/internal/parity"
)

func TestDeferredLifecycleParity(t *testing.T) {
	root := parityRepoRoot(t)
	lock, _, err := baseline.Load(filepath.Join(root, "parity", "baseline"))
	if err != nil {
		t.Fatal(err)
	}
	locked := parity.Baseline{ID: lock.BaselineID, Commit: lock.Upstream.Commit, Repository: lock.Upstream.Repository}
	fixture, err := parity.LoadFixture(filepath.Join(root, "parity", "oracle", "fixtures", "deferred-lifecycle.json"), locked)
	if err != nil {
		t.Fatal(err)
	}
	oracle, err := parity.NewFixtureDriver(fixture, locked)
	if err != nil {
		t.Fatal(err)
	}
	pig := parity.DriverFunc{SurfaceName: parity.SurfaceGoSDK, ObserveFunc: observeDeferredLifecycle}
	result, err := parity.RunCase(context.Background(), fixture.Case, oracle, pig)
	if err != nil || !result.Match || len(result.Normalizations) != 0 {
		t.Fatalf("deferred parity = %+v, %v", result, err)
	}
}

func observeDeferredLifecycle(ctx context.Context, declaration parity.Case) (parity.Observation, error) {
	var input struct {
		API            ai.API        `json:"api"`
		Provider       ai.ProviderID `json:"provider"`
		ModelID        string        `json:"model_id"`
		PendingFetches int           `json:"pending_fetches"`
		PollAfterMS    int64         `json:"poll_after_ms"`
		Response       string        `json:"response"`
		Failure        string        `json:"failure"`
		Context        ai.Context    `json:"context"`
		Actions        []string      `json:"actions"`
	}
	if err := json.Unmarshal(declaration.Input, &input); err != nil {
		return parity.Observation{}, err
	}
	faux, err := ai.NewFauxProvider(ai.RegisterFauxProviderOptions{API: input.API, Provider: input.Provider, Deferred: &ai.FauxDeferredOptions{PendingFetches: &input.PendingFetches, PollAfterMS: &input.PollAfterMS}})
	if err != nil {
		return parity.Observation{}, err
	}
	models := ai.CreateModels()
	models.SetProvider(faux.Provider)
	model, _ := faux.GetModel(input.ModelID)
	response, _ := ai.FauxAssistantMessage(ai.FauxAssistantText(input.Response), ai.FauxAssistantMessageOptions{Timestamp: ai.Some(int64(42))})
	factoryCalls := 0
	faux.SetResponses([]ai.FauxResponseStep{response, ai.FauxResponseFactory(func(ai.Context, *ai.SimpleStreamOptions, *ai.FauxProviderState, ai.Model) (ai.AssistantMessage, error) {
		factoryCalls++
		return response, nil
	}), response, ai.FauxResponseFactory(func(ai.Context, *ai.SimpleStreamOptions, *ai.FauxProviderState, ai.Model) (ai.AssistantMessage, error) {
		factoryCalls++
		return ai.AssistantMessage{}, errors.New(input.Failure)
	})})
	steps, hooks := map[string]any{}, []string{}
	var handle ai.DeferredHandle
	var final, failure ai.AssistantMessage
	for _, action := range input.Actions {
		options := ai.ProviderRequestOptions{OnResponse: func(_ context.Context, response ai.ProviderResponse, _ ai.Model) error {
			hooks = append(hooks, fmt.Sprintf("%s:%d", action, response.Status))
			return nil
		}}
		switch {
		case action == "sync" || strings.HasPrefix(action, "submit"):
			stream := faux.Provider.StreamSimple(ctx, model, input.Context, ai.SimpleStreamOptions{StreamOptions: ai.StreamOptions{ProviderRequestOptions: options}, Deferred: ai.DeferredBoolean{Enabled: action != "sync"}})
			events := []string{}
			for {
				event, ok, err := stream.Next(ctx)
				if err != nil {
					return parity.Observation{}, err
				}
				if !ok {
					break
				}
				name := string(event.AssistantMessageEventType())
				switch event := event.(type) {
				case ai.AssistantMessageDoneEvent:
					name += ":" + string(event.Reason)
				case ai.AssistantMessageErrorEvent:
					name += ":" + string(event.Reason)
				}
				events = append(events, name)
			}
			result, err := stream.Result(ctx)
			if err != nil {
				return parity.Observation{}, err
			}
			repeated, _ := stream.Result(ctx)
			steps[action] = map[string]any{"events": events, "message": projectDeferredMessage(result, ai.DeferredHandle{}), "repeat_equal": reflect.DeepEqual(result, repeated)}
			handle, _ = result.Deferred.Value()
		case action == "cancel":
			if err := models.CancelDeferred(ctx, model, handle, ai.ModelsDeferredCancelOptions{DeferredCancelOptions: options}); err != nil {
				return parity.Observation{}, err
			}
			steps[action] = map[string]any{"cancelled_count": len(faux.State.CancelledDeferred)}
		default:
			requested := handle
			if action == "unknown" {
				requested.ID = "unknown"
			} else if action == "mismatch" {
				requested.Provider = "foreign"
			}
			result, err := models.FetchDeferred(ctx, model, requested, ai.ModelsDeferredFetchOptions{DeferredFetchOptions: ai.DeferredFetchOptions{ProviderRequestOptions: options}})
			if err != nil {
				return parity.Observation{}, err
			}
			step := map[string]any{"message": projectDeferredMessage(result, requested)}
			if action == "final" {
				final = result
			} else if action == "repeat-final" {
				step["stable"] = reflect.DeepEqual(result, final)
			} else if action == "error-final" {
				failure = result
			} else if action == "repeat-error" {
				step["stable"] = reflect.DeepEqual(result, failure)
			}
			steps[action] = step
		}
	}
	outcome, err := json.Marshal(map[string]any{"steps": steps, "hooks": hooks, "factory_calls": factoryCalls, "call_count": faux.State.CallCount, "fetch_count": faux.State.DeferredFetchCount, "queue_remaining": faux.GetPendingResponseCount()})
	sideEffects := []parity.SideEffect{}
	return parity.Observation{Outcome: outcome, SideEffects: &sideEffects}, err
}

func projectDeferredMessage(message ai.AssistantMessage, handle ai.DeferredHandle) map[string]any {
	result := map[string]any{"role": message.Role, "api": message.API, "provider": message.Provider, "model": message.Model, "content": message.Content, "usage": message.Usage, "stopReason": message.StopReason}
	if deferred, ok := message.Deferred.Value(); ok {
		metadata := map[string]any{"provider": deferred.Provider, "modelId": deferred.ModelID, "api": deferred.API, "id_present": deferred.ID != "", "same_handle": handle.ID == "" || handle.ID == deferred.ID}
		if poll, ok := deferred.PollAfterMS.Value(); ok {
			metadata["pollAfterMs"] = poll
		}
		result["deferred"] = metadata
	}
	if text, ok := message.ErrorMessage.Value(); ok {
		if handle.ID != "" {
			text = strings.ReplaceAll(text, handle.ID, "<handle>")
		}
		result["errorMessage"] = text
	}
	return result
}
