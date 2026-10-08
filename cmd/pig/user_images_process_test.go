package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func TestUserImagesCLICrossProcessRestoreAndFork(t *testing.T) {
	data, err := os.ReadFile("../../parity/services/user-image.png")
	if err != nil {
		t.Fatal(err)
	}
	url := "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		raw, _ := json.Marshal(body["input"])
		if body["model"] != "deepseek-flash" || r.URL.Path != "/responses" || !bytes.Contains(raw, []byte(url)) {
			t.Errorf("actual image wire missing: %s", raw)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"type":"response.output_text.delta","delta":"IMAGE_SEEN"}`+"\n\n"+`data: {"type":"response.completed","response":{"status":"completed"}}`+"\n\n")
	}))
	defer server.Close()
	root, dir := taskRoot133(t)
	path := filepath.Join(root, "picture.dat")
	os.WriteFile(path, data, 0600)
	binary := binary133(t)
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(binary, append([]string{"--print", "--no-tools", "--no-context-files"}, args...)...)
		cmd.Dir, cmd.Env = root, taskEnv133(root, dir, server.URL)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("CLI: %v %s", err, out)
		}
		if !strings.Contains(string(out), "IMAGE_SEEN") {
			t.Fatalf("no successful image answer: %s", out)
		}
		return string(out)
	}
	run("--provider", "deepseek", "--model", "deepseek-flash", "--api", "openai-responses", "@"+path, "describe image")
	dirSessions := filepath.Join(dir, "sessions")
	sessions, err := codingagent.ListAllSessions(t.Context(), codingagent.SessionListOptions{SessionDir: &dirSessions})
	if err != nil || len(sessions) != 1 {
		t.Fatalf("sessions=%v %v", sessions, err)
	}
	saved := sessions[0].Path
	manager, err := codingagent.OpenSessionManager(saved, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	user := manager.BuildSessionContext().Messages[0].(ai.UserMessage)
	blocks, _ := user.Content.Blocks()
	if len(blocks) != 2 || blocks[1].(ai.ImageContent).Data != base64.StdEncoding.EncodeToString(data) {
		t.Fatal("formal v3 lost image")
	}
	os.Remove(path)
	run("--session", saved, "continue same image")
	before, _ := os.ReadFile(saved)
	run("--fork", saved, "continue fork image")
	after, _ := os.ReadFile(saved)
	if !bytes.Equal(before, after) || calls.Load() != 3 {
		t.Fatal("fork source changed")
	}
	// Missing input must fail before model requests.
	cmd := exec.Command(binary, "--print", "--no-tools", "--model", "deepseek-flash", "@"+path, "describe")
	cmd.Dir, cmd.Env = root, taskEnv133(root, dir, server.URL)
	if out, err := cmd.CombinedOutput(); err == nil || !strings.Contains(string(out), "image file") || calls.Load() != 3 {
		t.Fatalf("missing image: %s %v", out, err)
	}
}
