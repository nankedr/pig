package ai_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"reflect"
	"testing"

	"github.com/nankedr/pig/ai"
)

func TestOpenAIUsagePreservesReasoningAbsenceAndExplicitZero(t *testing.T) {
	tests := []struct {
		name string
		sse  string
		want ai.Optional[int64]
	}{
		{
			name: "absent",
			sse:  "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":0}}\n\ndata: [DONE]\n\n",
			want: ai.Absent[int64](),
		},
		{
			name: "explicit zero",
			sse:  "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":0,\"completion_tokens_details\":{\"reasoning_tokens\":0}}}\n\ndata: [DONE]\n\n",
			want: ai.Some[int64](0),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			stream := issue61OpenAIStream(context.Background(), test.sse)
			events := collectAssistantEvents(t, stream)
			result, err := stream.Result(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			done := events[len(events)-1].(ai.AssistantMessageDoneEvent)
			if result.Usage.Reasoning != test.want || !reflect.DeepEqual(done.Message.Usage, result.Usage) {
				t.Fatalf("event/result usage = (%#v, %#v), want reasoning %#v", done.Message.Usage, result.Usage, test.want)
			}
		})
	}
}

func TestOpenAIFailureDoesNotInventUsage(t *testing.T) {
	providerErr := errors.New("provider failed")
	key := "test-key"
	model := openAITextModel("https://example.test/v1")
	stream := ai.StreamOpenAICompletions(context.Background(), model, ai.Context{}, ai.OpenAICompletionsOptions{
		StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{
			APIKey: &key,
			Fetch: func(context.Context, ai.FetchRequest) (ai.FetchResponse, error) {
				return ai.FetchResponse{}, providerErr
			},
		}},
	})
	result, err := stream.Result(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.StopReason != ai.StopReasonError || result.Usage != (ai.Usage{}) {
		t.Fatalf("failed outcome = %#v, want error with zero usage", result)
	}
}

func TestOpenAICancellationPreservesOnlyReportedUsage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	reader, writer := io.Pipe()
	stream := issue61OpenAIStreamWithFetch(ctx, func(context.Context, ai.FetchRequest) (ai.FetchResponse, error) {
		return ai.FetchResponse{Status: http.StatusOK, BodyReader: reader}, nil
	})
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = io.WriteString(writer, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":4,\"completion_tokens\":1}}\n\n")
		cancel()
		_ = writer.Close()
	}()
	result, err := stream.Result(context.Background())
	<-done
	if err != nil {
		t.Fatal(err)
	}
	if result.StopReason != ai.StopReasonAborted || result.Usage != (ai.Usage{Input: 4, Output: 1, TotalTokens: 5, Cost: ai.UsageCost{Input: .000004, Output: .000002, Total: .000006}}) {
		t.Fatalf("aborted outcome = %#v, want only provider-reported usage", result)
	}
}

func issue61OpenAIStream(ctx context.Context, sse string) *ai.AssistantMessageEventStream {
	return issue61OpenAIStreamWithFetch(ctx, func(context.Context, ai.FetchRequest) (ai.FetchResponse, error) {
		return ai.FetchResponse{Status: http.StatusOK, Body: []byte(sse)}, nil
	})
}

func issue61OpenAIStreamWithFetch(ctx context.Context, fetch ai.FetchFunction) *ai.AssistantMessageEventStream {
	key := "test-key"
	return ai.StreamOpenAICompletions(ctx, openAITextModel("https://example.test/v1"), ai.Context{}, ai.OpenAICompletionsOptions{
		StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{APIKey: &key, Fetch: fetch}},
	})
}
