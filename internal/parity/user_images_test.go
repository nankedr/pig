package parity_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/nankedr/pig/ai"
)

func TestUserImagesFixedPiOracle(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(parityRepoRoot(t), "parity/oracle/fixtures/user-images.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Commit string `json:"baseline_commit"`
		Hash   string `json:"observation_hash"`
		Cases  []struct {
			Model   ai.Model
			Context ai.Context
			Wire    []any
		}
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Commit != "936aff00918de1187f085f123c2812d8f2d67745" {
		t.Fatal("unlocked Oracle")
	}
	var raw struct{ Cases json.RawMessage }
	json.Unmarshal(data, &raw)
	var compact bytes.Buffer
	json.Compact(&compact, raw.Cases)
	if fmt.Sprintf("%x", sha256.Sum256(compact.Bytes())) != fixture.Hash {
		t.Fatal("fixture drift")
	}
	for _, tc := range fixture.Cases {
		t.Run(string(tc.Model.API)+fmt.Sprint(len(tc.Wire)), func(t *testing.T) {
			var got []any
			key := "fixture"
			options := ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{APIKey: &key, Fetch: func(_ context.Context, r ai.FetchRequest) (ai.FetchResponse, error) {
				var body struct {
					Input    []any
					Messages []any
				}
				if err := json.Unmarshal(r.Body, &body); err != nil {
					return ai.FetchResponse{}, err
				}
				got = body.Input
				if tc.Model.API == ai.APIOpenAICompletions {
					got = body.Messages
				}
				sse := `data: {"type":"response.completed","response":{"status":"completed"}}` + "\n\n"
				if tc.Model.API == ai.APIOpenAICompletions {
					sse = `data: {"choices":[{"delta":{},"finish_reason":"stop"}]}` + "\n\ndata: [DONE]\n\n"
				}
				return ai.FetchResponse{Status: 200, Headers: map[string]string{"Content-Type": "text/event-stream"}, Body: []byte(sse)}, nil
			}}}
			out, err := ai.Complete(context.Background(), tc.Model, tc.Context, options)
			if err != nil || out.StopReason != ai.StopReasonStop || !reflect.DeepEqual(got, tc.Wire) {
				t.Fatalf("wire=%#v want=%#v out=%+v err=%v", got, tc.Wire, out, err)
			}
		})
	}
}
