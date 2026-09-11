//go:build !windows

package codingagent_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/nankedr/pig/codingagent"
)

func TestContextFilesRuntimeInjectionDoesNotDiscover(t *testing.T) {
	if os.Getenv("PIG_CONTEXT100_INJECTED_CHILD") == "1" {
		ctx := context.Background()
		cwd := t.TempDir()
		agentDir := t.TempDir()
		if err := syscall.Mkfifo(filepath.Join(cwd, ".git"), 0600); err != nil {
			t.Fatal(err)
		}
		models, _, _ := config87Runtime(t)
		model, _, _ := models.GetModel("deepseek", "deepseek-v4-flash")
		settings, _ := codingagent.NewInMemorySettingsManager(codingagent.Settings{})
		created, err := codingagent.CreateAgentSession(ctx, codingagent.CreateAgentSessionOptions{CWD: cwd, AgentDir: agentDir, Model: &model, ModelRuntime: models, SettingsManager: settings, ResourceLoader: context100Loader{files: []codingagent.AgentsFile{{Path: "virtual/AGENTS.md", Content: "EXPLICIT"}}}})
		if err != nil {
			t.Fatal(err)
		}
		created.Session.Dispose()
		services, err := codingagent.CreateAgentSessionServices(ctx, codingagent.CreateAgentSessionServicesOptions{CWD: cwd, AgentDir: agentDir, ModelRuntime: models, SettingsManager: settings, ResourceLoaderOptions: codingagent.DefaultResourceLoaderOptions{NoContextFiles: true}})
		if err != nil {
			t.Fatal(err)
		}
		disabled, err := codingagent.CreateAgentSessionFromServices(ctx, codingagent.CreateAgentSessionFromServicesOptions{Services: services, Model: &model})
		if err != nil {
			t.Fatal(err)
		}
		defer disabled.Session.Dispose()
		if files, err := disabled.Session.ResourceLoader().GetAgentsFiles(); err != nil || len(files) != 0 {
			t.Fatalf("disabled services: %v %v", files, err)
		}
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestContextFilesRuntimeInjectionDoesNotDiscover$", "-test.count=1")
	command.Env = append(os.Environ(), "PIG_CONTEXT100_INJECTED_CHILD=1")
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("injected loader unexpectedly discovered local .git FIFO: %v %v %s", ctx.Err(), err, out)
	}
}
