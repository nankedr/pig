package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPigLocalExtensionsNeverExecute(t *testing.T) {
	binary := buildPigBinary(t)
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"explicit", []string{"--no-extensions", "-e", "effect.js"}, "temporary:effect.js"},
		{"remote", []string{"-e", "npm:must-not-install"}, "package sources not implemented"},
		{"unknown", []string{"--zeta", "--alpha"}, "extension.flag.zeta"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			agentDir := filepath.Join(root, "agent")
			if err := os.MkdirAll(agentDir, 0700); err != nil {
				t.Fatal(err)
			}
			source := `require('node:fs').writeFileSync('EXECUTED','bad'); throw Error('MODULE IMPORTED');`
			if err := os.WriteFile(filepath.Join(root, "effect.js"), []byte(source), 0600); err != nil {
				t.Fatal(err)
			}
			before := snapshot107(t, root)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, binary, tc.args...)
			cmd.Dir = root
			cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + root, "PIG_CODING_AGENT_DIR=" + agentDir, "PIG_OFFLINE=1"}
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			err := cmd.Run()
			if ctx.Err() != nil {
				t.Fatal(ctx.Err())
			}
			if err == nil || stdout.Len() != 0 || !strings.Contains(stderr.String(), tc.want) || !strings.Contains(stderr.String(), "not implemented") {
				t.Fatalf("err=%v stdout=%q stderr=%q", err, stdout.String(), stderr.String())
			}
			if after := snapshot107(t, root); before != after {
				t.Fatalf("state changed: before=%s after=%s", before, after)
			}
		})
	}
}

func snapshot107(t *testing.T, root string) string {
	t.Helper()
	var out strings.Builder
	if err := filepath.WalkDir(root, func(path string, e os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		out.WriteString(path)
		if !e.IsDir() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			out.Write(data)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return out.String()
}

func TestPigLocalExtensionsResourcesRemainUsable(t *testing.T) {
	binary := buildPigBinary(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests.Add(1); w.WriteHeader(500) }))
	defer server.Close()
	for _, disabled := range []bool{false, true} {
		t.Run(fmt.Sprint(disabled), func(t *testing.T) {
			root := t.TempDir()
			agentDir := filepath.Join(root, "agent")
			for _, path := range []string{filepath.Join(agentDir, "extensions"), filepath.Join(agentDir, "prompts"), filepath.Join(root, "bin")} {
				if err := os.MkdirAll(path, 0700); err != nil {
					t.Fatal(err)
				}
			}
			source := fmt.Sprintf(`require('node:fs').writeFileSync(%q,'bad'); fetch(%q); throw Error('MODULE IMPORTED');`, filepath.Join(root, "EXECUTED"), server.URL)
			for path, data := range map[string]string{
				filepath.Join(agentDir, "extensions/effect.js"): source,
				filepath.Join(agentDir, "prompts/hello.md"):     "hello template",
				filepath.Join(agentDir, "settings.json"):        `{"extensions":["extensions/effect.js"]}`,
			} {
				if err := os.WriteFile(path, []byte(data), 0600); err != nil {
					t.Fatal(err)
				}
			}
			for _, name := range []string{"node", "npm", "git"} {
				if err := os.WriteFile(filepath.Join(root, "bin", name), []byte("#!/bin/sh\nprintf invoked > '"+filepath.Join(root, "PROCESS")+"'\nexit 1\n"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			before := snapshot107(t, root)
			args := []string{"--mode", "rpc", "--provider", "deepseek", "--model", "deepseek-v4-flash", "--api-key", "fixture", "--no-session", "--no-tools", "--no-context-files", "--offline"}
			if disabled {
				args = append(args, "--no-extensions")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, binary, args...)
			cmd.Dir = root
			cmd.Env = []string{"HOME=" + root, "PATH=" + filepath.Join(root, "bin"), "PIG_CODING_AGENT_DIR=" + agentDir, "HTTP_PROXY=" + server.URL, "HTTPS_PROXY=" + server.URL, "DEEPSEEK_BASE_URL=" + server.URL}
			cmd.Stdin = strings.NewReader("{\"type\":\"get_commands\",\"id\":107}\n")
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("%v: %s", err, stderr.String())
			}
			if !strings.Contains(stdout.String(), `"name":"hello"`) || strings.Contains(stdout.String(), `"source":"extension"`) {
				t.Fatalf("resources or events: %s", stdout.String())
			}
			if strings.Contains(stderr.String(), "not executed") == disabled {
				t.Fatalf("discovery diagnostic: %s", stderr.String())
			}
			if before != snapshot107(t, root) {
				t.Fatal("discovery wrote extension/session state or spawned a process")
			}
		})
	}
	if requests.Load() != 0 {
		t.Fatal("discovery accessed network")
	}
}
