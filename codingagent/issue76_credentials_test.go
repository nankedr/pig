package codingagent_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func credential76Set(t *testing.T, store ai.CredentialStore, provider, key string) {
	t.Helper()
	_, err := store.Modify(context.Background(), ai.ProviderID(provider), func(context.Context, ai.Credential) (ai.Credential, error) {
		return ai.APIKeyCredential{Type: ai.AuthTypeAPIKey, Key: ai.Some(key)}, nil
	}, ai.AuthOperationOptions{})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCredential76CrossProcess(t *testing.T) {
	if path := os.Getenv("PIG_TEST_76_AUTH"); path != "" {
		store, err := codingagent.NewAuthStorage(path)
		if err != nil {
			t.Fatal(err)
		}
		provider := os.Getenv("PIG_TEST_76_PROVIDER")
		for i := 0; i < 5; i++ {
			credential76Set(t, store, provider, fmt.Sprint(i))
		}
		return
	}
	path := filepath.Join(t.TempDir(), "private", "auth.json")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	commands := []*exec.Cmd{}
	for i := 0; i < 8; i++ {
		cmd := exec.Command(executable, "-test.run=^TestCredential76CrossProcess$")
		cmd.Env = append(os.Environ(), "PIG_TEST_76_AUTH="+path, fmt.Sprintf("PIG_TEST_76_PROVIDER=p%d", i))
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		commands = append(commands, cmd)
	}
	for _, cmd := range commands {
		if err := cmd.Wait(); err != nil {
			t.Fatal(err)
		}
	}
	store, err := codingagent.NewAuthStorage(path)
	if err != nil {
		t.Fatal(err)
	}
	infos, err := store.List(context.Background(), ai.AuthOperationOptions{})
	if err != nil || len(infos) != 8 {
		t.Fatalf("lost concurrent providers: %d %v", len(infos), err)
	}
	for _, info := range infos {
		value, err := store.Read(context.Background(), info.ProviderID, ai.AuthOperationOptions{})
		if err != nil {
			t.Fatal(err)
		}
		key, _ := value.(ai.APIKeyCredential).Key.Value()
		if key != "4" {
			t.Fatal("lost mutation")
		}
	}
	for path, mode := range map[string]os.FileMode{path: 0600, filepath.Dir(path): 0700} {
		stat, err := os.Stat(path)
		if err != nil || stat.Mode().Perm() != mode {
			t.Fatalf("permissions: %v %v", stat, err)
		}
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	credential76Set(t, store, "p0", "replacement")
	after, err := os.Stat(path)
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("credential write replaced file inode")
	}
}

func TestCredential76CommandCacheAndErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "auth.json")
	marker := filepath.Join(dir, "counter")
	store, err := codingagent.NewAuthStorage(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, fail := range []bool{false, true} {
		end := "printf '  SYNTHETIC_76_COMMAND  '"
		if fail {
			end = "printf SYNTHETIC_76_STDERR >&2; exit 1"
		}
		command := "!printf x >> '" + marker + "'; " + end
		credential76Set(t, store, "deepseek", command)
		if _, err := codingagent.ReadStoredCredential(context.Background(), "deepseek", path); err != nil {
			t.Fatal(err)
		}
		if _, err := store.List(context.Background(), ai.AuthOperationOptions{}); err != nil {
			t.Fatal(err)
		}
		if !fail {
			if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("raw read/list executed command")
			}
		}
		for i := 0; i < 2; i++ {
			value, err := store.Read(context.Background(), "deepseek", ai.AuthOperationOptions{})
			if fail {
				if err == nil || strings.Contains(err.Error(), "SYNTHETIC") {
					t.Fatal("command failure leaked or disappeared")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				key, _ := value.(ai.APIKeyCredential).Key.Value()
				if key != "SYNTHETIC_76_COMMAND" {
					t.Fatal("command result not trimmed")
				}
			}
		}
	}
	data, err := os.ReadFile(marker)
	if err != nil || string(data) != "xx" {
		t.Fatalf("command cache: %q %v", data, err)
	}
	for _, key := range []string{"", "$PIG_76_MISSING", "!exit 76"} {
		credential76Set(t, store, "deepseek", key)
		if _, err := store.Read(context.Background(), "deepseek", ai.AuthOperationOptions{}); err == nil {
			t.Fatal("unresolved owned entry succeeded")
		}
	}
}

func TestCredential76TimeoutAndCancellation(t *testing.T) {
	store, err := codingagent.NewAuthStorage(filepath.Join(t.TempDir(), "auth.json"))
	if err != nil {
		t.Fatal(err)
	}
	credential76Set(t, store, "deepseek", "!sleep 30; printf SYNTHETIC_76_TIMEOUT")
	start := time.Now()
	if _, err := store.Read(context.Background(), "deepseek", ai.AuthOperationOptions{}); err == nil || strings.Contains(err.Error(), "SYNTHETIC") {
		t.Fatal("command timeout failed")
	}
	if elapsed := time.Since(start); elapsed < 9*time.Second || elapsed > 13*time.Second {
		t.Fatalf("timeout=%v", elapsed)
	}
	credential76Set(t, store, "deepseek", "!sleep 30; printf SYNTHETIC_76_CANCEL")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start = time.Now()
	if _, err := store.Read(ctx, "deepseek", ai.AuthOperationOptions{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("cancel: %v", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("cancellation left shell pipes open")
	}
}

func TestCredential76MutationCancellationAndCorruption(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	store, err := codingagent.NewAuthStorage(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = store.Modify(ctx, "deepseek", func(context.Context, ai.Credential) (ai.Credential, error) {
		t.Fatal("canceled callback ran")
		return nil, nil
	}, ai.AuthOperationOptions{})
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("pre-canceled mutation created file")
	}
	credential76Set(t, store, "deepseek", "before")
	entered := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	ctx, cancel = context.WithCancel(context.Background())
	go func() {
		_, err := store.Modify(ctx, "deepseek", func(context.Context, ai.Credential) (ai.Credential, error) {
			close(entered)
			<-release
			return ai.APIKeyCredential{Type: ai.AuthTypeAPIKey, Key: ai.Some("after")}, nil
		}, ai.AuthOperationOptions{})
		done <- err
	}()
	<-entered
	waitCtx, waitCancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	if err := store.Delete(waitCtx, "deepseek", ai.AuthOperationOptions{}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal(err)
	}
	waitCancel()
	cancel()
	close(release)
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	value, err := store.Read(context.Background(), "deepseek", ai.AuthOperationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	key, _ := value.(ai.APIKeyCredential).Key.Value()
	if key != "before" {
		t.Fatal("canceled mutation committed")
	}
	for _, data := range []string{`{"SYNTHETIC_76_CORRUPT`, `null`, `[]`, `{"deepseek":{"type":"SYNTHETIC_76_BAD_TYPE"}}`} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		_, err := store.Modify(context.Background(), "deepseek", func(context.Context, ai.Credential) (ai.Credential, error) {
			return ai.APIKeyCredential{Type: ai.AuthTypeAPIKey, Key: ai.Some("next")}, nil
		}, ai.AuthOperationOptions{})
		if err == nil || strings.Contains(err.Error(), "SYNTHETIC") {
			t.Fatal("corrupt store failure leaked or disappeared")
		}
		after, _ := os.ReadFile(path)
		if string(after) != data {
			t.Fatal("corrupt store overwritten")
		}
	}
}

func TestCredential76CanonicalPathsAndExtraFields(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("PIG_CODING_AGENT_DIR", "")
	path, err := codingagent.ResolveAuthPath("")
	if err != nil || path != filepath.Join(home, ".pig", "agent", "auth.json") {
		t.Fatalf("canonical path %q %v", path, err)
	}
	t.Setenv("PIG_CODING_AGENT_DIR", "~/custom")
	path, err = codingagent.ResolveAuthPath("")
	if err != nil || path != filepath.Join(home, "custom", "auth.json") {
		t.Fatalf("agent override %q %v", path, err)
	}
	path, err = codingagent.ResolveAuthPath("~/explicit.json")
	if err != nil || path != filepath.Join(home, "explicit.json") {
		t.Fatalf("explicit override %q %v", path, err)
	}
	store, err := codingagent.NewAuthStorage(path)
	if err != nil {
		t.Fatal(err)
	}
	original := `{"type":"api_key","key":"first","custom":{"large":9007199254740993},"env":{"KEY":"value"}}`
	credential, err := ai.UnmarshalCredential([]byte(original))
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Modify(context.Background(), "deepseek", func(context.Context, ai.Credential) (ai.Credential, error) { return credential, nil }, ai.AuthOperationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Modify(context.Background(), "deepseek", func(_ context.Context, current ai.Credential) (ai.Credential, error) {
		value := current.(ai.APIKeyCredential)
		value.Key = ai.Some("second")
		return value, nil
	}, ai.AuthOperationOptions{})
	if err != nil {
		t.Fatal(err)
	}
	stored, err := codingagent.ReadStoredCredential(context.Background(), "deepseek", path)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := ai.MarshalCredential(stored)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(encoded, &fields)
	if string(fields["custom"]) != `{"large":9007199254740993}` {
		t.Fatal("unknown credential field lost precision")
	}
}
