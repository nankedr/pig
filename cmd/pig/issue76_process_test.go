package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func TestPigCanonicalCredentials(t *testing.T) {
	binary := buildPigBinary(t)
	home := t.TempDir()
	cwd := t.TempDir()
	sessions := filepath.Join(home, "sessions")
	path := filepath.Join(home, ".pig", "agent", "auth.json")
	store, err := codingagent.NewAuthStorage(path)
	if err != nil {
		t.Fatal(err)
	}
	set := func(key string) {
		t.Helper()
		_, err := store.Modify(context.Background(), "deepseek", func(context.Context, ai.Credential) (ai.Credential, error) {
			return ai.APIKeyCredential{Type: ai.AuthTypeAPIKey, Key: ai.Some(key)}, nil
		}, ai.AuthOperationOptions{})
		if err != nil {
			t.Fatal(err)
		}
	}
	secret := "SYNTHETIC_76_STORED_SECRET"
	set(secret)
	headers := make(chan string, 10)
	var reject atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		headers <- r.Header.Get("Authorization")
		if reject.Load() {
			http.Error(w, "invalid credential "+secret, http.StatusUnauthorized)
			return
		}
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), secret) {
			t.Error("credential leaked into request messages")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"reply\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	run := func(key string, flags ...string) (string, error) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		args := []string{"-p", "hello", "--provider", "deepseek", "--model", "deepseek-v4-flash", "--no-tools", "--session-dir", sessions}
		args = append(args, flags...)
		cmd := exec.CommandContext(ctx, binary, args...)
		cmd.Dir = cwd
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "DEEPSEEK_API_KEY=" + key, "PIG_DEEPSEEK_BASE_URL=" + server.URL}
		output, err := cmd.CombinedOutput()
		if strings.Contains(string(output), secret) {
			t.Fatal("secret leaked into process output")
		}
		return string(output), err
	}
	for _, test := range []struct {
		key, want string
		flags     []string
	}{{"", secret, nil}, {"ENV_76", secret, nil}, {"ENV_76", "CLI_76", []string{"--api-key", "CLI_76"}}} {
		output, err := run(test.key, test.flags...)
		if err != nil || output != "reply\n" {
			t.Fatalf("headless: %v %s", err, output)
		}
		if got := <-headers; got != "Bearer "+test.want {
			t.Fatal("wrong credential identity")
		}
	}
	reject.Store(true)
	if _, err := run(""); err == nil {
		t.Fatal("rejected credential succeeded")
	}
	<-headers
	reject.Store(false)

	set("$MISSING_76")
	if output, err := run("ENV_76"); err == nil || !strings.Contains(output, "credential") {
		t.Fatalf("unresolved entry did not fail: %v %s", err, output)
	}
	select {
	case <-headers:
		t.Fatal("failed credential fell back to environment")
	default:
	}
	if err := store.Delete(context.Background(), "deepseek", ai.AuthOperationOptions{}); err != nil {
		t.Fatal(err)
	}
	if output, err := run("ENV_76"); err != nil || output != "reply\n" {
		t.Fatalf("environment fallback: %v %s", err, output)
	}
	if <-headers != "Bearer ENV_76" {
		t.Fatal("environment identity missing")
	}
	// Global command credentials run after the project trust decision, including denial.
	marker := filepath.Join(home, "command-ran")
	set("!printf x >> '" + marker + "'; printf '" + secret + "'")
	if err := os.MkdirAll(filepath.Join(cwd, ".pig"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cwd, ".pig", "settings.json"), []byte("{invalid-untrusted-settings"), 0600); err != nil {
		t.Fatal(err)
	}
	trustPath := filepath.Join(home, ".pig", "agent", "trust.json")
	if err := os.WriteFile(trustPath, []byte("{invalid-trust"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := run(""); err == nil {
		t.Fatal("invalid trust decision succeeded")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("command executed before trust decision")
	}
	if err := os.Remove(trustPath); err != nil {
		t.Fatal(err)
	}
	if output, err := run("", "--no-approve"); err != nil || output != "reply\n" {
		t.Fatalf("global credential after trust denial: %v %s", err, output)
	}
	if <-headers != "Bearer "+secret {
		t.Fatal("command credential identity missing")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("post-trust command did not execute")
	}
	if err := store.Delete(context.Background(), "deepseek", ai.AuthOperationOptions{}); err != nil {
		t.Fatal(err)
	}
	for _, decoy := range []string{filepath.Join(cwd, "auth.json"), filepath.Join(home, ".pi", "agent", "auth.json")} {
		if err := os.MkdirAll(filepath.Dir(decoy), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(decoy, []byte(`{"deepseek":{"type":"api_key","key":"DECOY_76"}}`), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := run(""); err == nil {
		t.Fatal("read cwd or Pi credential fallback")
	}
	select {
	case <-headers:
		t.Fatal("unexpected fallback request")
	default:
	}

	if err := filepath.WalkDir(sessions, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if strings.Contains(string(data), secret) {
				t.Fatal("credential leaked into persisted session")
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
