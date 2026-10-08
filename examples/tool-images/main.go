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
		fmt.Fprintln(os.Stderr, "usage: go run ./examples/tool-images IMAGE")
		os.Exit(2)
	}
	runtime, err := codingagent.CreateHeadlessSession(context.Background(), codingagent.CreateHeadlessSessionOptions{Model: "deepseek-flash", Provider: ai.ProviderIDDeepSeek, API: ai.APIOpenAIResponses, Thinking: "off", Environment: ai.ProviderEnv{"DEEPSEEK_API_KEY": os.Getenv("DEEPSEEK_API_KEY")}, NoContextFiles: true})
	if err != nil {
		panic(err)
	}
	defer runtime.Dispose(context.Background())
	outcome, err := codingagent.RunHeadless(context.Background(), runtime, codingagent.HeadlessRunOptions{Messages: []string{fmt.Sprintf("Use read to open %q, identify its main colors, then use write to put their English names in image-colors.txt.", os.Args[1])}})
	if err != nil {
		panic(err)
	}
	if outcome.FinalMessage == nil || outcome.FinalMessage.StopReason != ai.StopReasonStop {
		fmt.Fprintln(os.Stderr, "tool vision task failed")
		os.Exit(1)
	}
	fmt.Println(outcome.Text)
	fmt.Println("Session:", *runtime.Session().SessionFile())
}
