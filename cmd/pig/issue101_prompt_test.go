//go:build !windows

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPigSystemPrompts(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("fixture requires non-root permissions")
	}
	binary := buildPigBinary(t)
	data, err := os.ReadFile("../../parity/oracle/fixtures/system-prompts.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Case struct {
			Input struct {
				Scenarios []struct {
					Name              string
					Files             map[string]string
					Trusted, Disabled bool
					System            *string
					Append            []string
					Unreadable        string
				}
			}
		}
		Observation struct {
			Outcome []struct {
				Name                     string
				System                   *string
				Append                   []string
				Custom, Context, Warning bool
			}
		}
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	requests := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []struct {
				Role    string
				Content json.RawMessage
			}
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		prompt := ""
		for _, m := range body.Messages {
			if m.Role == "system" {
				if err := json.Unmarshal(m.Content, &prompt); err != nil {
					t.Error(err)
				}
			}
		}
		requests <- prompt
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"PROMPT_OK\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	for i, scenario := range fixture.Case.Input.Scenarios {
		t.Run(scenario.Name, func(t *testing.T) {
			dir := t.TempDir()
			cwd, agentDir := filepath.Join(dir, "repo"), filepath.Join(dir, "agent")
			for _, p := range []string{cwd, agentDir} {
				if err := os.MkdirAll(p, 0700); err != nil {
					t.Fatal(err)
				}
			}
			for p, content := range scenario.Files {
				path := filepath.Join(dir, strings.ReplaceAll(p, "/.pi/", "/.pig/"))
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(content), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if scenario.Unreadable != "" {
				path := filepath.Join(dir, scenario.Unreadable)
				if err := os.Chmod(path, 0); err != nil {
					t.Fatal(err)
				}
				defer os.Chmod(path, 0600)
			}
			expand := func(s string) string {
				return strings.ReplaceAll(strings.ReplaceAll(s, "$ROOT", dir), "/.pi/", "/.pig/")
			}
			args := []string{"-p", "hello", "--provider", "deepseek", "--model", "deepseek-v4-flash", "--no-tools", "--offline", "--no-session"}
			if scenario.Trusted {
				args = append(args, "--approve")
			}
			if scenario.Disabled {
				args = append(args, "--no-context-files", "--no-extensions", "--no-skills", "--no-prompt-templates", "--no-themes")
			}
			if scenario.System != nil {
				args = append(args, "--system-prompt", expand(*scenario.System))
			}
			if scenario.Append != nil {
				if len(scenario.Append) == 0 {
					args = append(args, "--append-system-prompt", "")
				}
				for _, a := range scenario.Append {
					args = append(args, "--append-system-prompt", expand(a))
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			command := exec.CommandContext(ctx, binary, args...)
			command.Dir = cwd
			command.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + dir, "PIG_CODING_AGENT_DIR=" + agentDir, "DEEPSEEK_API_KEY=fixture", "PIG_DEEPSEEK_BASE_URL=" + server.URL}
			var stdout, stderr bytes.Buffer
			command.Stdout = &stdout
			command.Stderr = &stderr
			if err := command.Run(); err != nil || stdout.String() != "PROMPT_OK\n" {
				t.Fatalf("CLI: %v %s %s", err, stdout.String(), stderr.String())
			}
			var prompt string
			select {
			case prompt = <-requests:
			default:
				t.Fatal("no model request")
			}
			want := fixture.Observation.Outcome[i]
			if want.Custom {
				if !strings.HasPrefix(prompt, expand(*want.System)) {
					t.Fatalf("replacement: %s", prompt)
				}
			} else if !strings.HasPrefix(prompt, "You are an expert coding assistant") {
				t.Fatalf("default: %s", prompt)
			}
			if !strings.Contains(prompt, expand(strings.Join(want.Append, "\n\n"))) {
				t.Fatalf("append order: %s", prompt)
			}
			if strings.Contains(prompt, "CONTEXT") != want.Context || strings.Contains(prompt, "SECRET") || strings.Contains(prompt, "IGNORED") {
				t.Fatalf("resource trust/disable: %s", prompt)
			}
			if strings.Contains(stderr.String(), "Could not read") != want.Warning {
				t.Fatalf("diagnostics: %s", stderr.String())
			}
		})
	}
}
