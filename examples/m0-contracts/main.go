package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/nankedr/pig/ai"
)

func main() {
	ctx := context.Background()
	stream := ai.StreamOpenAICompletions(ctx, ai.Model{API: ai.APIOpenAICompletions}, ai.Context{}, ai.OpenAICompletionsOptions{
		StreamOptions: ai.StreamOptions{SamplingParams: map[string]json.RawMessage{"top_p": json.RawMessage(`0.8`)}},
	})
	_, err := stream.Result(ctx)
	if !errors.Is(err, ai.ErrNotImplemented) {
		fmt.Fprintln(os.Stderr, "unexpected M0 result:", err)
		os.Exit(1)
	}
	fmt.Println("M0 Capability Stub: true")
}
