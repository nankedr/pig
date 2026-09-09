package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/nankedr/pig/codingagent"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	dir, err := os.MkdirTemp("", "pig-rpc-control-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	binary := filepath.Join(dir, "pig")
	build := exec.Command("go", "build", "-o", binary, "./cmd/pig")
	if out, err := build.CombinedOutput(); err != nil {
		return fmt.Errorf("build: %w: %s", err, out)
	}
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		once.Do(func() { close(started) })
		select {
		case <-release:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"id\":\"example\",\"model\":\"deepseek-v4-flash\",\"choices\":[{\"delta\":{\"content\":\"hello over RPC\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	client := codingagent.NewRPCClient(codingagent.RPCClientOptions{CLIPath: &binary, CWD: &dir, Args: []string{"--provider", "deepseek", "--model", "deepseek-v4-flash", "--api-key", "example", "--no-session", "--no-tools", "--no-context-files", "--offline"}, Env: map[string]string{"PIG_CODING_AGENT_DIR": dir, "PIG_DEEPSEEK_BASE_URL": server.URL}})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := client.Start(ctx); err != nil {
		return err
	}
	defer client.Stop(context.Background())
	bash, err := client.Bash(ctx, "printf rpc-context")
	if err != nil {
		return err
	}
	if err = client.SetFollowUpMode(ctx, "one-at-a-time"); err != nil {
		return err
	}
	err = client.Prompt(ctx, "read the Bash context")
	if err != nil {
		return err
	}
	select {
	case <-started:
	case <-ctx.Done():
		return ctx.Err()
	}
	if err = client.FollowUp(ctx, "then answer once more"); err != nil {
		return err
	}
	close(release)
	err = client.WaitForIdle(ctx)
	if err != nil {
		return err
	}
	text, err := client.GetLastAssistantText(ctx)
	if err != nil {
		return err
	}
	stats, err := client.GetSessionStats(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("bash=%s, users=%d, assistants=%d, answer=%s\n", bash.Output, stats.UserMessages, stats.AssistantMessages, *text)
	return client.Stop(ctx)
}
