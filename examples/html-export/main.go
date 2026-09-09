package main

import (
	"context"
	"fmt"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"os"
	"path/filepath"
)

func main() {
	dir, err := os.MkdirTemp("", "pig-html-example-")
	must(err)
	defer os.RemoveAll(dir)
	sm, err := codingagent.NewSessionManager(dir, &dir)
	must(err)
	_, err = sm.AppendMessage(ai.UserMessage{Role: "user", Content: ai.UserText("# Pig HTML\n\n**离线导出**与 `Go SDK`"), Timestamp: 1})
	must(err)
	_, err = sm.AppendMessage(ai.AssistantMessage{Role: "assistant", Content: []ai.AssistantContent{ai.TextContent{Type: "text", Text: "```go\nfmt.Println(\"hello\")\n```"}}, StopReason: ai.StopReasonStop, Timestamp: 2})
	must(err)
	session := codingagent.NewAgentSession(codingagent.AgentSessionConfig{SessionManager: sm})
	output := filepath.Join(dir, "session.html")
	if len(os.Args) > 1 {
		output = os.Args[1]
	}
	path, err := session.ExportToHTML(context.Background(), output)
	must(err)
	info, err := os.Stat(path)
	must(err)
	fmt.Printf("HTML export: %d bytes; pass an output path to keep the file.\n", info.Size())
}
func must(err error) {
	if err != nil {
		panic(err)
	}
}
