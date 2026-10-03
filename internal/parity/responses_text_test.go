package parity_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/nankedr/pig/ai"
)

func TestResponsesTextFixedPiOracle(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(parityRepoRoot(t), "parity/oracle/fixtures/responses-text.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Commit        string `json:"baseline_commit"`
		Deterministic bool   `json:"deterministic"`
		Hash          string `json:"observation_hash"`
		Input         struct {
			Model   ai.Model   `json:"model"`
			Context ai.Context `json:"context"`
		} `json:"input"`
		Cases []struct {
			Name    string          `json:"name"`
			Simple  bool            `json:"simple"`
			Options json.RawMessage `json:"options"`
			SSE     string          `json:"sse"`
			Types   []string        `json:"types"`
			Request struct {
				URL  string         `json:"url"`
				Body map[string]any `json:"body"`
			} `json:"request"`
			Outcome struct {
				Content []ai.TextContent `json:"content"`
				Stop    ai.StopReason    `json:"stopReason"`
				Raw     *string          `json:"rawStopReason"`
				Error   *string          `json:"errorMessage"`
				Usage   ai.Usage         `json:"usage"`
			} `json:"outcome"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Commit != "936aff00918de1187f085f123c2812d8f2d67745" || !fixture.Deterministic {
		t.Fatal("fixture is not locked Pi evidence")
	}
	var raw struct {
		Cases json.RawMessage `json:"cases"`
	}
	json.Unmarshal(data, &raw)
	var compact bytes.Buffer
	if err := json.Compact(&compact, raw.Cases); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(compact.Bytes())
	if hex.EncodeToString(digest[:]) != fixture.Hash {
		t.Fatal("fixture observations changed")
	}
	for _, tc := range fixture.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			key := "fixture-key"
			tokens := int64(512)
			cache := ai.CacheRetentionNone
			var options ai.SimpleStreamOptions
			if err := json.Unmarshal(tc.Options, &options); err != nil {
				t.Fatal(err)
			}
			options.MaxTokens, options.CacheRetention = &tokens, &cache
			options.ProviderRequestOptions = ai.ProviderRequestOptions{APIKey: &key, Fetch: func(_ context.Context, r ai.FetchRequest) (ai.FetchResponse, error) {
				var body map[string]any
				if err := json.Unmarshal(r.Body, &body); err != nil {
					t.Fatal(err)
				}
				if r.URL != tc.Request.URL || !reflect.DeepEqual(body, tc.Request.Body) {
					t.Errorf("request = %s, %#v; Pi = %#v", r.URL, body, tc.Request)
				}
				return ai.FetchResponse{Status: 200, Body: []byte(tc.SSE)}, nil
			}}
			var stream *ai.AssistantMessageEventStream
			if tc.Simple {
				stream = ai.StreamSimpleOpenAIResponses(context.Background(), fixture.Input.Model, fixture.Input.Context, options)
			} else {
				stream = ai.StreamOpenAIResponses(context.Background(), fixture.Input.Model, fixture.Input.Context, ai.OpenAIResponsesOptions{StreamOptions: options.StreamOptions})
			}
			var types []string
			for {
				event, ok, err := stream.Next(context.Background())
				if err != nil {
					t.Fatal(err)
				}
				if !ok {
					break
				}
				types = append(types, string(event.AssistantMessageEventType()))
			}
			out, err := stream.Result(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(types, tc.Types) || out.StopReason != tc.Outcome.Stop || len(out.Content) != len(tc.Outcome.Content) {
				t.Fatalf("Pi events/outcome differ: %v, %#v", types, out)
			}
			for i, block := range out.Content {
				if text, ok := block.(ai.TextContent); !ok || text.Text != tc.Outcome.Content[i].Text {
					t.Fatalf("content = %#v", out.Content)
				}
			}
			raw, hasRaw := out.RawStopReason.Value()
			if hasRaw != (tc.Outcome.Raw != nil) || hasRaw && raw != *tc.Outcome.Raw {
				t.Fatalf("raw stop = %#v", out.RawStopReason)
			}
			message, hasError := out.ErrorMessage.Value()
			if hasError != (tc.Outcome.Error != nil) || hasError && message != *tc.Outcome.Error {
				t.Fatalf("error = %#v", out.ErrorMessage)
			}
			u, w := out.Usage, tc.Outcome.Usage
			if u.Input != w.Input || u.Output != w.Output || u.CacheRead != w.CacheRead || u.CacheWrite != w.CacheWrite || u.TotalTokens != w.TotalTokens || !reflect.DeepEqual(u.Reasoning, w.Reasoning) {
				t.Fatalf("usage = %#v; Pi = %#v", u, w)
			}
			if math.Abs(u.Cost.Total-w.Cost.Total) > 1e-15 {
				t.Fatalf("cost = %#v; Pi = %#v", u.Cost, w.Cost)
			}
		})
	}
}
