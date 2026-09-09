package main

import (
	"context"
	"fmt"
	"github.com/nankedr/pig/codingagent"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	dir, err := os.MkdirTemp("", "pig-rpc-lifecycle-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	binary := filepath.Join(dir, "pig")
	if out, err := exec.Command("go", "build", "-o", binary, "./cmd/pig").CombinedOutput(); err != nil {
		return fmt.Errorf("build: %w: %s", err, out)
	}
	if err = os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"compaction":{"enabled":false,"keepRecentTokens":1},"retry":{"enabled":false}}`), 0600); err != nil {
		return err
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"local lifecycle answer\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":10,\"completion_tokens\":2,\"total_tokens\":12}}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	client := codingagent.NewRPCClient(codingagent.RPCClientOptions{CLIPath: &binary, CWD: &dir, Args: []string{"--provider", "deepseek", "--model", "deepseek-v4-flash", "--api-key", "example", "--no-tools", "--no-context-files", "--offline"}, Env: map[string]string{"PIG_CODING_AGENT_DIR": dir, "PIG_DEEPSEEK_BASE_URL": server.URL}})
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err = client.Start(ctx); err != nil {
		return err
	}
	defer client.Stop(context.Background())
	for _, prompt := range []string{"first question", "second question"} {
		if _, err = client.PromptAndWait(ctx, prompt); err != nil {
			return err
		}
	}
	source, err := client.GetState(ctx)
	if err != nil {
		return err
	}
	users, err := client.GetForkMessages(ctx)
	if err != nil {
		return err
	}
	text, _, err := client.Fork(ctx, users[1].EntryID)
	if err != nil {
		return err
	}
	if _, err = client.PromptAndWait(ctx, text+" with another approach"); err != nil {
		return err
	}
	if _, err = client.SwitchSession(ctx, *source.SessionFile); err != nil {
		return err
	}
	summary, err := client.Compact(ctx)
	if err != nil {
		return err
	}
	if _, err = client.PromptAndWait(ctx, "continue from the summary"); err != nil {
		return err
	}
	entries, leaf, err := client.GetEntries(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("fork text=%q; compacted=%t; entries=%d; leaf=%s\n", text, summary.Summary != "", len(entries), *leaf)
	return client.Stop(ctx)
}
