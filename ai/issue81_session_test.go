package ai_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/nankedr/pig/ai"
)

func TestOpenAICompletionsSessionIdentityWithoutCache(t *testing.T) {
	model := openAITextModel("https://example.invalid")
	var requests []ai.FetchRequest
	key := "fixture"
	options := ai.OpenAICompletionsOptions{StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{APIKey: &key, Fetch: func(_ context.Context, r ai.FetchRequest) (ai.FetchResponse, error) {
		requests = append(requests, r)
		return ai.FetchResponse{Status: 200, Headers: map[string]string{"content-type": "text/event-stream"}, Body: []byte("data: {\"choices\":[{\"delta\":{\"content\":\"done\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")}, nil
	}}}}
	for _, id := range []string{"", "original", "forked"} {
		if id != "" {
			retention := ai.CacheRetentionNone
			options.CacheRetention = &retention
			options.SessionID = &id
		}
		out, err := ai.StreamOpenAICompletions(context.Background(), model, ai.Context{}, options).Result(context.Background())
		if err != nil || out.StopReason != ai.StopReasonStop {
			t.Fatalf("id=%q: %+v %v", id, out, err)
		}
	}
	if len(requests) != 3 || !reflect.DeepEqual(requests[0], requests[1]) || !reflect.DeepEqual(requests[0], requests[2]) {
		t.Fatalf("no-cache session affected request: %+v", requests)
	}
}
