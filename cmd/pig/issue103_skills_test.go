package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPigSkills(t *testing.T) {
	binary := buildPigBinary(t)
	data, err := os.ReadFile("../../parity/oracle/fixtures/skills.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Case struct {
			Input struct {
				Scenarios []struct {
					Name, CWD                     string
					Links, ExtraFiles             map[string]string
					Files                         map[string]string
					Trusted, Disabled             bool
					Paths, Calls, Global, Project []string
				}
			}
		}
		Observation struct{ Outcome []struct{ Expanded []string } }
	}
	if err = json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	requests := make(chan string, 100)
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
			if m.Role == "user" {
				if err := json.Unmarshal(m.Content, &prompt); err != nil {
					var blocks []struct{ Text string }
					if err = json.Unmarshal(m.Content, &blocks); err != nil {
						t.Error(err)
					}
					prompt = ""
					for _, b := range blocks {
						prompt += b.Text
					}
				}
			}
		}
		requests <- prompt
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"SKILL_OK\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	for i, scenario := range fixture.Case.Input.Scenarios {
		t.Run(scenario.Name, func(t *testing.T) {
			dir, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			cwdPath := scenario.CWD
			if cwdPath == "" {
				cwdPath = "repo"
			}
			cwd, agentDir := filepath.Join(dir, cwdPath), filepath.Join(dir, "agent")
			write := func(path, content string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(content), 0600); err != nil {
					t.Fatal(err)
				}
			}
			for path, content := range scenario.Files {
				write(strings.ReplaceAll(filepath.Join(dir, path), "/.pi/", "/.pig/"), content)
			}
			for path, content := range scenario.ExtraFiles {
				write(filepath.Join(dir, path), content)
			}
			for path, target := range scenario.Links {
				if err := os.Symlink(target, strings.ReplaceAll(filepath.Join(dir, path), "/.pi/", "/.pig/")); err != nil {
					t.Fatal(err)
				}
			}
			global, _ := json.Marshal(map[string]any{"skills": scenario.Global})
			project, _ := json.Marshal(map[string]any{"skills": scenario.Project})
			write(filepath.Join(agentDir, "settings.json"), string(global))
			write(filepath.Join(cwd, ".pig/settings.json"), string(project))
			calls := scenario.Calls
			args := []string{"-p", "--provider", "deepseek", "--model", "deepseek-v4-flash", "--no-tools", "--offline", "--no-session"}
			args = append(args, calls...)
			if scenario.Trusted {
				args = append(args, "--approve")
			}
			if scenario.Disabled {
				args = append(args, "--no-skills")
			}
			for _, path := range scenario.Paths {
				args = append(args, "--skill", strings.ReplaceAll(strings.ReplaceAll(path, ".pi/", ".pig/"), "$ROOT", dir))
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, binary, args...)
			cmd.Dir = cwd
			cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + filepath.Join(dir, "home"), "PIG_CODING_AGENT_DIR=" + agentDir, "DEEPSEEK_API_KEY=fixture", "PIG_DEEPSEEK_BASE_URL=" + server.URL}
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("CLI: %v\n%s\n%s", err, stdout.String(), stderr.String())
			}
			if !strings.Contains(stdout.String(), "SKILL_OK") {
				t.Fatalf("missing reply: %s", stdout.String())
			}
			for j := range calls {
				select {
				case got := <-requests:
					got = strings.ReplaceAll(strings.ReplaceAll(got, dir, "$ROOT"), "/.pig", "/.pi")
					if got != fixture.Observation.Outcome[i].Expanded[j] {
						t.Fatalf("model input %d: %q != %q", j, got, fixture.Observation.Outcome[i].Expanded[j])
					}
				default:
					t.Fatal("missing model request")
				}
			}
			if scenario.Name == "missing" && !strings.Contains(stderr.String(), "skill path does not exist") {
				t.Fatalf("missing diagnostics: %s", stderr.String())
			}
		})
	}
}

func TestRPC103SkillCommands(t *testing.T) {
	binary := buildPigBinary(t)
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "skills/review"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skills/review/SKILL.md"), []byte("---\nname: review\ndescription: Review code\ndisable-model-invocation: true\n---\nReview $1"), 0600); err != nil {
		t.Fatal(err)
	}
	p := startRPC94(t, binary, "PIG_CODING_AGENT_DIR="+dir, "HOME="+t.TempDir())
	if _, err := io.WriteString(p.input, "{\"id\":103,\"type\":\"get_commands\"}\n"); err != nil {
		t.Fatal(err)
	}
	record := p.record(t)
	if record["success"] != true {
		t.Fatal(record)
	}
	commands := record["data"].(map[string]any)["commands"].([]any)
	if len(commands) != 1 {
		t.Fatal(record)
	}
	command := commands[0].(map[string]any)
	source := command["sourceInfo"].(map[string]any)
	if command["name"] != "skill:review" || command["description"] != "Review code" || command["source"] != "skill" || source["source"] != "auto" || source["scope"] != "user" || source["path"] != filepath.Join(dir, "skills/review/SKILL.md") {
		t.Fatal(record)
	}
}
