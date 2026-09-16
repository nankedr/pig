package m5gate_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/nankedr/pig/codingagent"
)

func TestPigM5LocalWorkflow(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	repo := filepath.Join(filepath.Dir(file), "../..")
	binary := os.Getenv("PIG_BINARY")
	if binary == "" {
		binary = filepath.Join(t.TempDir(), "pig")
		cmd := exec.Command("go", "build", "-race", "-o", binary, "./cmd/pig")
		cmd.Dir = repo
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build: %v: %s", err, out)
		}
	}
	for _, mode := range []string{"trusted", "denied", "explicit"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			cwd, dir, sessions := filepath.Join(root, "repo"), filepath.Join(root, "agent"), filepath.Join(root, "sessions")
			write := func(path, data string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(data), 0600); err != nil {
					t.Fatal(err)
				}
			}
			write(filepath.Join(cwd, "AGENTS.md"), "CONTEXT_ALWAYS_LOADED")
			write(filepath.Join(dir, "SYSTEM.md"), "GLOBAL_SYSTEM")
			write(filepath.Join(cwd, ".pig/SYSTEM.md"), "TRUSTED_PROJECT_SYSTEM")
			write(filepath.Join(dir, "settings.json"), `{"theme":"local","compaction":{"enabled":false},"retry":{"enabled":false}}`)
			write(filepath.Join(cwd, ".pig/prompts/forbidden.md"), "PROJECT_ONLY")
			write(filepath.Join(cwd, "package.json"), `{"pi":{"extensions":["effect.js"]},"scripts":{"postinstall":"touch INSTALLED"}}`)
			write(filepath.Join(cwd, "effect.js"), `require('node:fs').writeFileSync('EXECUTED','bad')`)
			resourceDir := dir
			if mode == "trusted" {
				resourceDir = filepath.Join(cwd, ".pig")
			}
			if mode == "explicit" {
				resourceDir = filepath.Join(root, "explicit")
			}
			theme, err := codingagent.LoadBuiltinTheme("light")
			if err != nil {
				t.Fatal(err)
			}
			resources := func(version, accent string) {
				write(filepath.Join(resourceDir, "prompts/review.md"), version+" review $1")
				write(filepath.Join(resourceDir, "skills/check/SKILL.md"), "---\nname: check\ndescription: "+version+" checks\n---\n"+version+" SKILL_BODY")
				colors := theme.ResolvedColors()
				colors["accent"] = accent
				data, err := json.Marshal(map[string]any{"name": "local", "colors": colors})
				if err != nil {
					t.Fatal(err)
				}
				write(filepath.Join(resourceDir, "themes/local.json"), string(data))
			}
			resources("ONE", "#112233")
			requests := make(chan string, 10)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				requests <- string(body)
				w.Header().Set("Content-Type", "text/event-stream")
				fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"M5_OK\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
			}))
			defer server.Close()
			path := filepath.Join(root, "empty-bin")
			if err := os.Mkdir(path, 0700); err != nil {
				t.Fatal(err)
			}
			env := []string{"PATH=" + path, "HOME=" + filepath.Join(root, "home"), "PIG_CODING_AGENT_DIR=" + dir, "PIG_DEEPSEEK_BASE_URL=" + server.URL, "DEEPSEEK_API_KEY=fixture"}
			flags := []string{"--offline", "--no-tools"}
			if mode == "trusted" {
				flags = append(flags, "--approve")
			} else {
				flags = append(flags, "--no-approve")
			}
			if mode == "explicit" {
				flags = append(flags, "--no-skills", "--no-themes", "--no-prompt-templates", "--skill", filepath.Join(resourceDir, "skills"), "--prompt-template", filepath.Join(resourceDir, "prompts"), "--theme", filepath.Join(resourceDir, "themes/local.json"))
			}
			run := func(args ...string) string {
				t.Helper()
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, binary, append(append([]string{}, flags...), args...)...)
				cmd.Dir, cmd.Env = cwd, env
				output, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("CLI: %v: %s", err, output)
				}
				return string(output)
			}
			prompt := func(args ...string) string {
				t.Helper()
				args = append([]string{"--provider", "deepseek", "--model", "deepseek-v4-flash", "--session-dir", sessions, "-p"}, args...)
				if out := run(args...); !strings.Contains(out, "M5_OK") {
					t.Fatal(out)
				}
				select {
				case request := <-requests:
					return request
				default:
					t.Fatal("missing provider request")
					return ""
				}
			}
			first := prompt("/review main.go")
			if !strings.Contains(first, "ONE review main.go") || !strings.Contains(first, "CONTEXT_ALWAYS_LOADED") || strings.Contains(first, "TRUSTED_PROJECT_SYSTEM") != (mode == "trusted") {
				t.Fatal(first)
			}
			files, err := filepath.Glob(filepath.Join(sessions, "*.jsonl"))
			if err != nil || len(files) != 1 {
				t.Fatalf("sessions: %v %v", files, err)
			}
			manager, err := codingagent.OpenSessionManager(files[0], nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			id, leaf := manager.GetSessionID(), *manager.GetLeafID()
			second := prompt("/skill:check main.go", "--session", files[0])
			if !strings.Contains(second, "ONE SKILL_BODY") || !strings.Contains(second, "ONE review main.go") || !strings.Contains(second, "M5_OK") {
				t.Fatal(second)
			}
			resources("TWO", "#445566")
			third := prompt("/review resumed.go", "--session", files[0])
			if !strings.Contains(third, "TWO review resumed.go") || !strings.Contains(third, "ONE SKILL_BODY") {
				t.Fatal(third)
			}
			fourth := prompt("/skill:check resumed.go", "--session", files[0])
			if !strings.Contains(fourth, "TWO SKILL_BODY") {
				t.Fatal(fourth)
			}
			manager, err = codingagent.OpenSessionManager(files[0], nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			if manager.GetSessionID() != id || manager.GetEntry(leaf) == nil {
				t.Fatal("resume lost session identity or history")
			}
			htmlPath := filepath.Join(root, "session.html")
			run("--export", files[0], htmlPath)
			html, err := os.ReadFile(htmlPath)
			if err != nil {
				t.Fatal(err)
			}
			encoded := strings.SplitN(string(html), `<script id="session-data" type="application/json">`, 2)
			if len(encoded) != 2 {
				t.Fatal("missing embedded session")
			}
			transcript, err := base64.StdEncoding.DecodeString(strings.SplitN(encoded[1], "</script>", 2)[0])
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(html), "--accent: #445566;") || !strings.Contains(string(transcript), "M5_OK") {
				t.Fatal("export lost theme or messages")
			}
			for _, p := range []string{"node_modules", "package-lock.json", "INSTALLED", "EXECUTED"} {
				if _, err := os.Stat(filepath.Join(cwd, p)); !os.IsNotExist(err) {
					t.Fatalf("unexpected package/extension effect: %s (%v)", p, err)
				}
			}
		})
	}
}
