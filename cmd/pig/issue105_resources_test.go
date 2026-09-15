package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPigLocalResourceDiagnostics(t *testing.T) {
	binary := buildPigBinary(t)
	data, err := os.ReadFile("../../parity/oracle/fixtures/local-resources.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Case struct {
			Input struct {
				Scenarios []struct {
					Name                   string
					Files                  map[string]string
					Global, Project, Paths map[string][]string
				}
			}
		}
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	scenario := fixture.Case.Input.Scenarios[0]
	for _, candidate := range fixture.Case.Input.Scenarios {
		if candidate.Name == "mixed-priority" {
			scenario = candidate
			break
		}
	}
	if scenario.Name != "mixed-priority" {
		t.Fatal("missing mixed-priority fixture")
	}
	dir := t.TempDir()
	cwd, agentDir := filepath.Join(dir, "repo"), filepath.Join(dir, "agent")
	write := func(path string, data []byte) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for path, content := range scenario.Files {
		write(strings.ReplaceAll(filepath.Join(dir, path), "/.pi/", "/.pig/"), []byte(content))
	}
	for path, settings := range map[string]map[string][]string{filepath.Join(agentDir, "settings.json"): scenario.Global, filepath.Join(cwd, ".pig/settings.json"): scenario.Project} {
		data, _ := json.Marshal(settings)
		write(path, data)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	args := []string{"--mode", "rpc", "--provider", "deepseek", "--model", "deepseek-v4-flash", "--api-key", "fixture", "--no-session", "--no-tools", "--no-context-files", "--offline", "--approve"}
	for kind, flag := range map[string]string{"prompts": "--prompt-template", "skills": "--skill", "themes": "--theme"} {
		for _, path := range scenario.Paths[kind] {
			args = append(args, flag, path)
		}
	}
	cmd := exec.CommandContext(ctx, binary, args...)
	cmd.Dir = cwd
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + filepath.Join(dir, "home"), "PIG_CODING_AGENT_DIR=" + agentDir}
	cmd.Stdin = strings.NewReader("{\"type\":\"get_commands\",\"id\":105}\n")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("%v: %s", err, stderr.String())
	}
	var response struct {
		Success bool
		Data    struct {
			Commands []struct {
				Source     string
				SourceInfo struct{ Path, Source, Scope string }
			}
		}
	}
	if err = json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &response); err != nil {
		t.Fatalf("%v: %s", err, stdout.String())
	}
	if !response.Success || len(response.Data.Commands) != 2 {
		t.Fatalf("commands: %s", stdout.String())
	}
	for _, command := range response.Data.Commands {
		if command.SourceInfo.Source != "local" || command.SourceInfo.Scope != "project" || !strings.Contains(command.SourceInfo.Path, "/.pig/more/") {
			t.Fatalf("unexpected winner: %+v", command)
		}
	}
	for _, path := range []string{"prompts/same.md", "skills/same/SKILL.md", "themes/same.json"} {
		for _, part := range []string{filepath.Join(cwd, ".pig/more", path) + " (project:local)", filepath.Join(agentDir, path) + " (user:auto)", filepath.Join(dir, "extra", path) + " (temporary:local)"} {
			if !strings.Contains(stderr.String(), part) {
				t.Errorf("missing collision source %q: %s", part, stderr.String())
			}
		}
	}
}
