package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/nankedr/pig/codingagent"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	endpoint := os.Getenv("PIG_DEEPSEEK_BASE_URL")
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "http" || parsed.Hostname() != "127.0.0.1" {
		return fmt.Errorf("run through parity/terminal/m6-workflow.py <example-binary> --sdk with its controlled loopback service")
	}
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	sessions, key := filepath.Join(cwd, "sessions"), "synthetic"
	runtime, err := codingagent.CreateHeadlessSession(ctx, codingagent.CreateHeadlessSessionOptions{
		CWD: cwd, AgentDir: os.Getenv("PIG_CODING_AGENT_DIR"), SessionDir: &sessions,
		Provider: "deepseek", Model: "deepseek-v4-flash", APIKey: &key, BaseURL: &endpoint,
		Offline: true, NoTools: codingagent.NoToolsAll, NoExtensions: true,
	})
	if err != nil {
		return err
	}
	defer runtime.Dispose(context.Background())
	mode := codingagent.NewInteractiveMode(runtime)
	defer mode.Stop()
	return mode.Run(ctx)
}
