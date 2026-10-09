package main

import (
	"context"
	"fmt"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"os"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: image-workflow <pig-binary> <local-image>")
		os.Exit(2)
	}
	image, err := ai.LoadImageFile(os.Args[2])
	if err != nil {
		panic(err)
	}
	client := codingagent.NewRPCClient(codingagent.RPCClientOptions{CLIPath: &os.Args[1], Args: []string{"--model", "deepseek-flash", "--api", "openai-responses", "--thinking", "off"}})
	ctx := context.Background()
	if err := client.Start(ctx); err != nil {
		panic(err)
	}
	defer client.Stop(ctx)
	if _, err := client.PromptAndWait(ctx, "Describe the screenshot, then use read if more context is needed.", []ai.ImageContent{image}); err != nil {
		panic(err)
	}
	path, err := client.ExportHTML(ctx, "image-workflow.html")
	if err != nil {
		panic(err)
	}
	fmt.Println(path)
}
