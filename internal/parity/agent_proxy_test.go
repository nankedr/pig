package parity_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/internal/parity"
)

func TestAgentProxyParity(t *testing.T) {
	fixture, locked := loadCompatFixture(t, "agent-proxy")
	oracle, err := parity.NewFixtureDriver(fixture, locked)
	if err != nil {
		t.Fatal(err)
	}
	result, err := parity.RunCase(context.Background(), fixture.Case, oracle, parity.DriverFunc{SurfaceName: parity.SurfaceGoSDK, ObserveFunc: observeAgentProxy})
	if err != nil || !result.Match || len(result.Normalizations) != 0 {
		t.Fatalf("proxy parity differences=%v: %v\nGo outcome: %s", result.Differences, err, result.Pig.Outcome)
	}
}

func observeAgentProxy(ctx context.Context, declaration parity.Case) (parity.Observation, error) {
	var input struct {
		Model     ai.Model                 `json:"model"`
		Context   ai.Context               `json:"context"`
		Options   agent.ProxyStreamOptions `json:"options"`
		Scenarios []struct {
			Name   string            `json:"name"`
			Events []json.RawMessage `json:"events"`
			Cancel bool              `json:"cancel"`
		} `json:"scenarios"`
	}
	if err := json.Unmarshal(declaration.Input, &input); err != nil {
		return parity.Observation{}, err
	}
	outcomes := []map[string]any{}
	for _, scenario := range input.Scenarios {
		outcome, err := func() (map[string]any, error) {
			ctx, cancel := context.WithCancelCause(ctx)
			defer cancel(nil)
			request := make(chan map[string]any, 1)
			writerDone := make(chan struct{})
			options := input.Options
			options.ProxyURL, options.AuthToken = "https://proxy.invalid", "proxy-secret"
			options.Fetch = func(ctx context.Context, r ai.FetchRequest) (ai.FetchResponse, error) {
				var body any
				if err := json.Unmarshal(r.Body, &body); err != nil {
					return ai.FetchResponse{}, err
				}
				request <- map[string]any{"url": r.URL, "method": r.Method, "headers": r.Headers, "body": body}
				reader, writer := io.Pipe()
				go func() {
					defer close(writerDone)
					defer writer.Close()
					for _, event := range scenario.Events {
						var compact bytes.Buffer
						_ = json.Compact(&compact, event)
						if _, err := fmt.Fprintf(writer, "data: %s\n\n", compact.Bytes()); err != nil {
							return
						}
					}
					if scenario.Cancel {
						<-ctx.Done()
					}
				}()
				return ai.FetchResponse{Status: 200, BodyReader: reader}, nil
			}
			stream := agent.StreamProxy(ctx, input.Model, input.Context, options)
			results := make([]map[string]any, 3)
			resultErrors := make([]error, 3)
			var wg sync.WaitGroup
			for i := range results {
				wg.Add(1)
				go func() {
					defer wg.Done()
					message, err := stream.Result(context.Background())
					resultErrors[i] = err
					results[i] = projectProxyMessage(message)
				}()
			}
			events := []map[string]any{}
			for {
				event, ok, err := stream.Next(context.Background())
				if err != nil {
					return nil, err
				}
				if !ok {
					break
				}
				data, err := ai.MarshalAssistantMessageEvent(event)
				if err != nil {
					return nil, err
				}
				var projected map[string]any
				if err := json.Unmarshal(data, &projected); err != nil {
					return nil, err
				}
				for _, key := range []string{"partial", "message", "error"} {
					if message, ok := projected[key].(map[string]any); ok {
						delete(message, "timestamp")
					}
				}
				events = append(events, projected)
				if scenario.Cancel && len(events) == len(scenario.Events) {
					cancel(errors.New("Request aborted by user"))
				}
			}
			wg.Wait()
			for _, err := range resultErrors {
				if err != nil {
					return nil, err
				}
			}
			<-writerDone
			return map[string]any{"name": scenario.Name, "request": <-request, "events": events, "results": results}, nil
		}()
		if err != nil {
			return parity.Observation{}, err
		}
		outcomes = append(outcomes, outcome)
	}
	data, err := json.Marshal(outcomes)
	sideEffects := []parity.SideEffect{}
	return parity.Observation{Outcome: data, SideEffects: &sideEffects}, err
}

func projectProxyMessage(message ai.AssistantMessage) map[string]any {
	data, _ := ai.MarshalMessage(message)
	var result map[string]any
	json.Unmarshal(data, &result)
	delete(result, "timestamp")
	return result
}
