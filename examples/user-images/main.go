package main

import (
	"context"
	"fmt"
	"os"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: go run ./examples/user-images IMAGE")
		os.Exit(2)
	}
	block, err := ai.LoadImageFile(os.Args[1])
	if err != nil {
		panic(err)
	}
	runtime, err := codingagent.CreateHeadlessSession(context.Background(), codingagent.CreateHeadlessSessionOptions{Provider: ai.ProviderIDDeepSeek, Model: "deepseek-flash", API: ai.APIOpenAIResponses, Environment: ai.ProviderEnv{"DEEPSEEK_API_KEY": os.Getenv("DEEPSEEK_API_KEY")}, NoTools: codingagent.NoToolsAll, NoContextFiles: true})
	if err != nil {
		panic(err)
	}
	defer runtime.Dispose(context.Background())
	out, err := codingagent.RunHeadless(context.Background(), runtime, codingagent.HeadlessRunOptions{Messages: []string{"Describe the picture."}, InitialImages: []ai.ImageContent{block}})
	if err != nil {
		panic(err)
	}
	if out.FinalMessage == nil || out.FinalMessage.StopReason != ai.StopReasonStop {
		fmt.Fprintln(os.Stderr, "image generation failed")
		os.Exit(1)
	}
	fmt.Println(out.Text)
	fmt.Println("Session:", *runtime.Session().SessionFile())
}
