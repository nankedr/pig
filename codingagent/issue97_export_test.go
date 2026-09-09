package codingagent_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/codingagent"
)

func TestIssue97ExportCLIParity(t *testing.T) {
	raw, err := os.ReadFile("../parity/oracle/fixtures/export-html.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Case        struct{ Input struct{ Session string } }
		Observation struct {
			Outcome struct{ Data json.RawMessage }
		}
	}
	if err = json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	source := filepath.Join(dir, "session.jsonl")
	output := filepath.Join(dir, "session.html")
	if err = os.WriteFile(source, []byte(f.Case.Input.Session), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := codingagent.RunCLI(context.Background(), codingagent.CLIInvocation{Arguments: []string{"--export", source, output}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Stdout != "Exported to: "+output+"\n" {
		t.Fatal(result)
	}
	got := issue97HTMLData(t, output)
	var want any
	if err = json.Unmarshal(f.Observation.Outcome.Data, &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("export data mismatch\ngot %#v\nwant %#v", got, want)
	}
	after, err := os.ReadFile(source)
	if err != nil || string(after) != f.Case.Input.Session {
		t.Fatal("export modified source", err)
	}
}

func issue97HTMLData(t *testing.T, path string) map[string]any {
	t.Helper()
	html, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile(`id="session-data" type="application/json">([^<]+)`).FindSubmatch(html)
	if len(m) != 2 {
		t.Fatal("missing embedded session data")
	}
	data, err := base64.StdEncoding.DecodeString(string(m[1]))
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err = json.Unmarshal(data, &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestIssue97ExportSDKSnapshot(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "session.jsonl")
	output := filepath.Join(dir, "sdk.html")
	input, err := os.ReadFile("../parity/export-html/session.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(source, input, 0600); err != nil {
		t.Fatal(err)
	}
	manager, err := codingagent.OpenSessionManager(source, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Select an earlier branch without rewriting the append-only source.
	if err = manager.Branch("entry-6"); err != nil {
		t.Fatal(err)
	}
	session := codingagent.NewAgentSession(codingagent.AgentSessionConfig{SessionManager: manager})
	got, err := session.ExportToHTML(context.Background(), output)
	if err != nil {
		t.Fatal(err)
	}
	if got != output || issue97HTMLData(t, output)["leafId"] != "entry-6" {
		t.Fatal("SDK lost current leaf")
	}
	if _, err = codingagent.NewAgentSession(codingagent.AgentSessionConfig{SessionManager: codingagent.NewInMemorySessionManager(dir)}).ExportToHTML(context.Background(), output); err == nil {
		t.Fatal("exported in-memory session")
	}
}

func TestIssue97ExportFailuresAndPaths(t *testing.T) {
	dir := t.TempDir()
	root := issue32RepoRoot(t)
	t.Chdir(dir)
	valid, err := os.ReadFile(filepath.Join(root, "parity/export-html/session.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(dir, "source.jsonl")
	output := filepath.Join(dir, "out.html")
	if err = os.WriteFile(source, valid, 0600); err != nil {
		t.Fatal(err)
	}
	path, err := codingagent.ExportFromFile(context.Background(), source)
	if err != nil || path != "pig-session-source.html" {
		t.Fatal(path, err)
	}
	for _, target := range []string{source, dir, filepath.Join(dir, "missing", "out.html")} {
		if _, err = codingagent.ExportFromFile(context.Background(), source, target); err == nil {
			t.Fatalf("accepted output %q", target)
		}
	}
	link := filepath.Join(dir, "link.html")
	if err = os.Link(source, link); err != nil {
		t.Fatal(err)
	}
	if _, err = codingagent.ExportFromFile(context.Background(), source, link); err == nil {
		t.Fatal("overwrote source hardlink")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = codingagent.ExportFromFile(ctx, source, output); err == nil {
		t.Fatal("ignored cancellation")
	}
	if _, err = codingagent.ExportFromFile(context.Background(), "missing.jsonl", output); err == nil {
		t.Fatal("accepted missing input")
	}
	cases := map[string]string{
		"empty": "", "invalid": "{oops", "scalar": "1", "v2": strings.Replace(string(valid), `"version":3`, `"version":2`, 1),
		"broken-line": string(valid) + "{oops\n", "cycle": strings.Replace(string(valid), `"parentId":null`, `"parentId":"entry-3"`, 1),
		"image":           strings.Replace(string(valid), `"type":"text","text":"hello"`, `"type":"image","data":"x","mimeType":"image/png"`, 1),
		"unknown":         strings.Replace(string(valid), `"role":"assistant"`, `"role":"futureRole"`, 1),
		"invalid-content": strings.Replace(string(valid), `"content":"custom text"`, `"content":null`, 1),
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(source, []byte(input), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := codingagent.ExportFromFile(context.Background(), source, output); err == nil {
				t.Fatal("accepted invalid/unsupported input")
			}
			after, _ := os.ReadFile(source)
			if string(after) != input {
				t.Fatal("modified invalid source")
			}
			if _, err := os.Stat(output); !os.IsNotExist(err) {
				t.Fatal("wrote failed export", err)
			}
		})
	}
}

func TestIssue97ExportSDKMetadataAndInvalidUsage(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "session.jsonl")
	output := filepath.Join(dir, "sdk.html")
	input, err := os.ReadFile("../parity/export-html/session.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(source, input, 0600); err != nil {
		t.Fatal(err)
	}
	sm, err := codingagent.OpenSessionManager(source, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	a, err := agent.NewAgent(agent.AgentOptions{InitialState: &agent.AgentInitialState{SystemPrompt: "system </script><script>bad()</script>"}})
	if err != nil {
		t.Fatal(err)
	}
	s := codingagent.NewAgentSession(codingagent.AgentSessionConfig{Agent: a, SessionManager: sm})
	if _, err = s.ExportToHTML(context.Background(), output); err != nil {
		t.Fatal(err)
	}
	if issue97HTMLData(t, output)["systemPrompt"] != "system </script><script>bad()</script>" {
		t.Fatal("missing SDK system prompt")
	}
	invalid := strings.Replace(string(input), `"output":1`, `"output":"bad"`, 1)
	if err = os.WriteFile(source, []byte(invalid), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = codingagent.ExportFromFile(context.Background(), source, output); err == nil {
		t.Fatal("accepted malformed usage")
	}
}
