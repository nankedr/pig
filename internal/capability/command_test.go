package capability_test

import (
	"bytes"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

func TestCommandStubsHaveNoSideEffects(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))

	for _, tt := range []struct {
		name   string
		path   string
		module string
	}{
		{name: "pig", path: "./cmd/pig", module: "codingagent"},
		{name: "pig-ai", path: "./cmd/pig-ai", module: "ai"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			proxyURL, networkRequests := startNetworkProbe(t)
			list := exec.Command("go", "list", "-deps", tt.path)
			list.Dir = root
			output, err := list.CombinedOutput()
			if err != nil {
				t.Fatalf("list dependencies: %v\n%s", err, output)
			}
			for _, dependency := range strings.Fields(string(output)) {
				if dependency == "net" || strings.HasPrefix(dependency, "net/") || dependency == "os/exec" {
					t.Fatalf("network dependency = %s", dependency)
				}
			}

			temp := t.TempDir()
			binary := filepath.Join(temp, tt.name)
			build := exec.Command("go", "build", "-o", binary, tt.path)
			build.Dir = root
			if output, err := build.CombinedOutput(); err != nil {
				t.Fatalf("build command: %v\n%s", err, output)
			}

			home := filepath.Join(temp, "home")
			work := filepath.Join(temp, "work")
			tempState := filepath.Join(temp, "tmp")
			if err := os.Mkdir(home, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(work, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(tempState, 0o700); err != nil {
				t.Fatal(err)
			}
			var successEvents, stderr bytes.Buffer
			command := exec.Command(binary)
			command.Dir = work
			command.Env = append(os.Environ(),
				"HOME="+home,
				"TMPDIR="+tempState,
				"HTTP_PROXY="+proxyURL,
				"HTTPS_PROXY="+proxyURL,
				"ALL_PROXY="+proxyURL,
				"NO_PROXY=",
			)
			command.Stdout = &successEvents
			command.Stderr = &stderr
			err = command.Run()
			if err == nil {
				t.Fatal("stub command returned success")
			}
			if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() == 0 {
				t.Fatalf("command error = %v", err)
			}
			if successEvents.Len() != 0 {
				t.Fatalf("success events = %q", successEvents.String())
			}
			if text := stderr.String(); !strings.Contains(text, tt.module) || !strings.Contains(text, "command") || !strings.Contains(text, "not implemented") {
				t.Fatalf("stderr does not identify the capability: %q", text)
			}
			assertEmptyDir(t, home)
			assertEmptyDir(t, work)
			assertEmptyDir(t, tempState)
			if networkRequests.Load() != 0 {
				t.Fatalf("network requests = %d", networkRequests.Load())
			}
		})
	}
}

func startNetworkProbe(t *testing.T) (string, *atomic.Int64) {
	t.Helper()
	requests := &atomic.Int64{}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Logf("runtime network probe unavailable: %v", err)
		return "http://127.0.0.1:1", requests
	}
	server := &http.Server{Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	})}
	go server.Serve(listener)
	t.Cleanup(func() {
		server.Close()
	})
	return "http://" + listener.Addr().String(), requests
}

func assertEmptyDir(t *testing.T, path string) {
	t.Helper()
	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("state files in %s = %v", path, entries)
	}
}
