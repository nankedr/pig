package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/nankedr/pig/ai"
)

func main() {
	ctx := context.Background()
	stream := ai.StreamOpenAICompletions(ctx, ai.Model{API: ai.APIOpenAICompletions}, ai.Context{}, ai.OpenAICompletionsOptions{})
	_, err := stream.Result(ctx)
	if !errors.Is(err, ai.ErrNotImplemented) {
		fmt.Fprintln(os.Stderr, "unexpected M0 result:", err)
		os.Exit(1)
	}
	fmt.Println("M0 Capability Stub: true")
}
