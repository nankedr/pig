package parity_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/nankedr/pig/ai"
)

func TestResponsesToolsFixedPiOracle(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(parityRepoRoot(t), "parity/oracle/fixtures/responses-tools.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Commit        string   `json:"baseline_commit"`
		Deterministic bool     `json:"deterministic"`
		Hash          string   `json:"observation_hash"`
		Model         ai.Model `json:"model"`
		Cases         []struct {
			Name    string                    `json:"name"`
			Context ai.Context                `json:"context"`
			Options ai.OpenAIResponsesOptions `json:"options"`
			SSE     string                    `json:"sse"`
			Request struct {
				URL  string         `json:"url"`
				Body map[string]any `json:"body"`
			} `json:"request"`
			Types   []string `json:"types"`
			Outcome struct {
				Content []ai.ToolCall `json:"content"`
				Stop    ai.StopReason `json:"stopReason"`
			} `json:"outcome"`
		} `json:"cases"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Commit != "936aff00918de1187f085f123c2812d8f2d67745" || !fixture.Deterministic {
		t.Fatal("fixture is not fixed Pi evidence")
	}
	var raw struct {
		Cases json.RawMessage `json:"cases"`
	}
	json.Unmarshal(data, &raw)
	var compact bytes.Buffer
	json.Compact(&compact, raw.Cases)
	digest := sha256.Sum256(compact.Bytes())
	if hex.EncodeToString(digest[:]) != fixture.Hash {
		t.Fatal("fixture observations changed")
	}
	for _, tc := range fixture.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			key := "fixture-key"
			tokens := int64(512)
			cache := ai.CacheRetentionNone
			options := tc.Options
			options.APIKey = &key
			options.MaxTokens = &tokens
			options.CacheRetention = &cache
			options.Fetch = func(_ context.Context, r ai.FetchRequest) (ai.FetchResponse, error) {
				var body map[string]any
				if err := json.Unmarshal(r.Body, &body); err != nil {
					t.Fatal(err)
				}
				if r.URL != tc.Request.URL || !reflect.DeepEqual(body, tc.Request.Body) {
					t.Errorf("request=%s %#v; Pi=%#v", r.URL, body, tc.Request.Body)
				}
				return ai.FetchResponse{Status: 200, Body: []byte(tc.SSE)}, nil
			}
			stream := ai.StreamOpenAIResponses(context.Background(), fixture.Model, tc.Context, options)
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
				t.Fatalf("events=%v outcome=%#v; Pi=%v %#v", types, out, tc.Types, tc.Outcome)
			}
			for i, block := range out.Content {
				if call, ok := block.(ai.ToolCall); !ok || !reflect.DeepEqual(call, tc.Outcome.Content[i]) {
					t.Fatalf("call=%#v; Pi=%#v", block, tc.Outcome.Content[i])
				}
			}
		})
	}
}
