package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPigThemesHTMLParity(t *testing.T) {
	binary := buildPigBinary(t)
	raw, err := os.ReadFile("../../parity/oracle/fixtures/themes.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Case struct {
			Input struct{ Documents []json.RawMessage }
		}
		Observation struct {
			Outcome struct {
				Colors []struct {
					Name               string
					HTML, ShadowedHTML map[string]string
				}
			}
		}
	}
	if err = json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	sourceData, err := os.ReadFile("../../parity/export-html/session.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range fixture.Observation.Outcome.Colors {
		t.Run(want.Name, func(t *testing.T) {
			dir := t.TempDir()
			source, output, themePath := filepath.Join(dir, "session.jsonl"), filepath.Join(dir, "out.html"), filepath.Join(dir, "theme.json")
			for path, data := range map[string][]byte{source: sourceData, themePath: fixture.Case.Input.Documents[i], filepath.Join(dir, "settings.json"): []byte(`{"theme":"` + want.Name + `"}`)} {
				if err = os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, binary, "--export", source, output, "--theme", themePath, "--no-themes")
			cmd.Dir = dir
			cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + dir, "PIG_CODING_AGENT_DIR=" + dir, "PIG_OFFLINE=1"}
			if b, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%v: %s", err, b)
			}
			html, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			for key, color := range want.HTML {
				if !strings.Contains(string(html), "--"+key+": "+color+";") {
					t.Fatalf("Pi color %s=%s missing", key, color)
				}
			}
			var shadow map[string]any
			if err = json.Unmarshal(fixture.Case.Input.Documents[i], &shadow); err != nil {
				t.Fatal(err)
			}
			shadow["colors"].(map[string]any)["accent"] = "#010203"
			shadow["export"] = map[string]any{"pageBg": "#040506"}
			modified, _ := json.Marshal(shadow)
			if err = os.WriteFile(themePath, modified, 0600); err != nil {
				t.Fatal(err)
			}
			cmd = exec.CommandContext(ctx, binary, "--export", source, output, "--theme", themePath, "--no-themes")
			cmd.Dir = dir
			cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + dir, "PIG_CODING_AGENT_DIR=" + dir, "PIG_OFFLINE=1"}
			if b, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%v: %s", err, b)
			}
			html, err = os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			for key, color := range want.ShadowedHTML {
				if !strings.Contains(string(html), "--"+key+": "+color+";") {
					t.Fatalf("shadowed Pi color %s=%s missing", key, color)
				}
			}
			after, _ := os.ReadFile(source)
			if string(after) != string(sourceData) {
				t.Fatal("modified input")
			}
		})
	}
}

func TestPigThemesProjectTrustAndDiagnostics(t *testing.T) {
	binary := buildPigBinary(t)
	dir := t.TempDir()
	state := filepath.Join(dir, "agent")
	if err := os.MkdirAll(filepath.Join(dir, ".pig/themes"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(state, 0700); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("../../codingagent/themes/light.json")
	if err != nil {
		t.Fatal(err)
	}
	data = []byte(strings.Replace(string(data), `"name": "light"`, `"name": "project"`, 1))
	session, err := os.ReadFile("../../parity/export-html/session.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	source, output := filepath.Join(dir, "session.jsonl"), filepath.Join(dir, "out.html")
	for path, b := range map[string][]byte{source: session, filepath.Join(dir, ".pig/themes/project.json"): data, filepath.Join(dir, ".pig/settings.json"): []byte(`{"theme":"project"}`)} {
		if err = os.WriteFile(path, b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	for _, approved := range []bool{false, true} {
		args := []string{"--export", source, output, "--theme", "missing.json"}
		if approved {
			args = append(args, "--approve")
		} else {
			args = append(args, "--no-approve")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		cmd := exec.CommandContext(ctx, binary, args...)
		cmd.Dir = dir
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + dir, "PIG_CODING_AGENT_DIR=" + state, "PIG_OFFLINE=1"}
		b, err := cmd.CombinedOutput()
		cancel()
		if err != nil {
			t.Fatalf("%v: %s", err, b)
		}
		if !strings.Contains(string(b), "theme path does not exist") {
			t.Fatal("missing theme not diagnosed", string(b))
		}
		html, _ := os.ReadFile(output)
		var doc struct {
			Colors map[string]any
			Vars   map[string]any
		}
		json.Unmarshal(data, &doc)
		v := doc.Colors["text"].(string)
		if !strings.HasPrefix(v, "#") {
			v = doc.Vars[v].(string)
		}
		if strings.Contains(string(html), "--text: "+v+";") != approved {
			t.Fatal("export project trust ignored")
		}
	}
}
