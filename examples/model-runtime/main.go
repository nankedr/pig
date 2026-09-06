package main

import (
	"context"
	"fmt"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func main() {
	ctx := context.Background()
	runtime, err := codingagent.NewModelRuntime(ctx, codingagent.CreateModelRuntimeOptions{Credentials: ai.NewInMemoryCredentialStore(), Offline: true})
	must(err)
	resolved, err := codingagent.ResolveCLIModel(codingagent.ResolveCliModelOptions{CLIModel: "deepseek/deepseek-v4-flash:high", ModelRuntime: runtime})
	must(err)
	if resolved.Error != nil {
		panic(*resolved.Error)
	}
	catalog, err := runtime.GetModels()
	must(err)
	key := "synthetic-example-key"
	reply, err := runtime.CompleteSimple(ctx, *resolved.Model, ai.Context{}, ai.ModelsSimpleStreamOptions{SimpleStreamOptions: ai.SimpleStreamOptions{StreamOptions: ai.StreamOptions{ProviderRequestOptions: ai.ProviderRequestOptions{APIKey: &key, Fetch: func(context.Context, ai.FetchRequest) (ai.FetchResponse, error) {
		return ai.FetchResponse{Status: 200, Body: []byte("data: {\"choices\":[{\"delta\":{\"content\":\"offline transport reply\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")}, nil
	}}}}})
	must(err)
	fmt.Printf("catalog=%d model=%s/%s thinking=%s stop=%s\n", len(catalog), resolved.Model.Provider, resolved.Model.ID, *resolved.ThinkingLevel, reply.StopReason)
}
func must(err error) {
	if err != nil {
		panic(err)
	}
}
