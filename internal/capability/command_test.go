package capability_test

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nankedr/pig/codingagent"
)

func TestCommandStubsHaveNoSideEffects(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))

	for _, tt := range []struct {
		name       string
		path       string
		arguments  []string
		wantStderr string
	}{
		{name: "pig print mode requires explicit provider", path: "./cmd/pig", wantStderr: "Error: Headless mode requires --provider <provider>\n"},
		{name: "pig invalid thinking warning", path: "./cmd/pig", arguments: []string{"--thinking", "invalid"}, wantStderr: "Warning: Invalid thinking level \"invalid\". Valid values: off, minimal, low, medium, high, xhigh, max\nError: Headless mode requires --provider <provider>\n"},
		{name: "pig package command", path: "./cmd/pig", arguments: []string{"install", "npm:example"}, wantStderr: "codingagent.command.install: not implemented\n"},
		{name: "pig extension discovery", path: "./cmd/pig", arguments: []string{"--extension", "extension.ts"}, wantStderr: "codingagent.extension.discovery: not implemented\n"},
		{name: "pig extension flag", path: "./cmd/pig", arguments: []string{"--review=deep"}, wantStderr: "codingagent.extension.flag.review: not implemented\n"},
		{name: "pig-ai list", path: "./cmd/pig-ai", arguments: []string{"list"}, wantStderr: "ai.CLI.List: not implemented\n"},
		{name: "pig-ai explicit auth path", path: "./cmd/pig-ai", arguments: []string{"login", "anthropic", "--auth-path", "explicit-auth.json"}, wantStderr: "ai.CLI.Login: not implemented\n"},
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
				if dependency == "os/exec" {
					t.Fatalf("process dependency = %s", dependency)
				}
			}

			temp := t.TempDir()
			binary := filepath.Join(temp, filepath.Base(tt.path))
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
			seedContaminationSentinels(t, home, work)
			before := snapshotTrees(t, home, work, tempState)
			var successEvents, stderr bytes.Buffer
			command := exec.Command(binary, tt.arguments...)
			command.Dir = work
			command.Env = contaminatedEnvironment(home, tempState, proxyURL)
			command.Stdout = &successEvents
			command.Stderr = &stderr
			err = command.Run()
			if err == nil {
				t.Fatal("stub command returned success")
			}
			if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
				t.Fatalf("command error = %v, want exit 1", err)
			}
			if successEvents.Len() != 0 {
				t.Fatalf("success events = %q", successEvents.String())
			}
			if text := stderr.String(); text != tt.wantStderr {
				t.Fatalf("stderr = %q, want %q", text, tt.wantStderr)
			}
			if after := snapshotTrees(t, home, work, tempState); fmt.Sprint(after) != fmt.Sprint(before) {
				t.Fatalf("filesystem state changed:\nbefore: %#v\nafter:  %#v", before, after)
			}
			if networkRequests.Load() != 0 {
				t.Fatalf("network requests = %d", networkRequests.Load())
			}
		})
	}
}

func TestPigStaticMetadataCommands(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	temp := t.TempDir()
	binary := filepath.Join(temp, "pig")
	build := exec.Command("go", "build", "-o", binary, "./cmd/pig")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build command: %v\n%s", err, output)
	}
	help := readFixture(t, root, "codingagent/testdata/pig_help.golden.txt")
	installHelp := readFixture(t, root, "codingagent/testdata/pig_install_help.golden.txt")
	removeHelp := readFixture(t, root, "codingagent/testdata/pig_remove_help.golden.txt")
	updateHelp := readFixture(t, root, "codingagent/testdata/pig_update_help.golden.txt")
	listHelp := readFixture(t, root, "codingagent/testdata/pig_list_help.golden.txt")
	configHelp := readFixture(t, root, "codingagent/testdata/pig_config_help.golden.txt")
	authHelp := readFixture(t, root, "codingagent/testdata/pig_auth_help.golden.txt")

	for _, tt := range []struct {
		name       string
		arguments  []string
		wantOutput string
	}{
		{name: "help", arguments: []string{"--help"}, wantOutput: help},
		{name: "short help", arguments: []string{"-h"}, wantOutput: help},
		{name: "version", arguments: []string{"--version"}, wantOutput: codingagent.Version + "\n"},
		{name: "short version", arguments: []string{"-v"}, wantOutput: codingagent.Version + "\n"},
		{name: "install help", arguments: []string{"install", "--help"}, wantOutput: installHelp},
		{name: "install short help", arguments: []string{"install", "-h"}, wantOutput: installHelp},
		{name: "remove help", arguments: []string{"remove", "--help"}, wantOutput: removeHelp},
		{name: "remove short help", arguments: []string{"remove", "-h"}, wantOutput: removeHelp},
		{name: "uninstall help", arguments: []string{"uninstall", "--help"}, wantOutput: removeHelp},
		{name: "uninstall short help", arguments: []string{"uninstall", "-h"}, wantOutput: removeHelp},
		{name: "update help", arguments: []string{"update", "--help"}, wantOutput: updateHelp},
		{name: "update short help", arguments: []string{"update", "-h"}, wantOutput: updateHelp},
		{name: "list help", arguments: []string{"list", "--help"}, wantOutput: listHelp},
		{name: "list short help", arguments: []string{"list", "-h"}, wantOutput: listHelp},
		{name: "config help", arguments: []string{"config", "--help"}, wantOutput: configHelp},
		{name: "config short help", arguments: []string{"config", "-h"}, wantOutput: configHelp},
		{name: "auth default help", arguments: []string{"auth"}, wantOutput: authHelp},
		{name: "auth help command", arguments: []string{"auth", "help"}, wantOutput: authHelp},
		{name: "auth help", arguments: []string{"auth", "--help"}, wantOutput: authHelp},
		{name: "auth short help", arguments: []string{"auth", "-h"}, wantOutput: authHelp},
		{name: "auth check help", arguments: []string{"auth", "check", "--help"}, wantOutput: authHelp},
		{name: "auth check short help", arguments: []string{"auth", "check", "-h"}, wantOutput: authHelp},
		{name: "auth API key help", arguments: []string{"auth", "print-api-key", "--help"}, wantOutput: authHelp},
		{name: "auth API key short help", arguments: []string{"auth", "print-api-key", "-h"}, wantOutput: authHelp},
		{name: "auth bearer token help", arguments: []string{"auth", "print-bearer-token", "--help"}, wantOutput: authHelp},
		{name: "auth bearer token short help", arguments: []string{"auth", "print-bearer-token", "-h"}, wantOutput: authHelp},
	} {
		t.Run(tt.name, func(t *testing.T) {
			proxyURL, networkRequests := startNetworkProbe(t)
			home := filepath.Join(temp, "home-"+strings.ReplaceAll(tt.name, " ", "-"))
			work := filepath.Join(temp, "work-"+strings.ReplaceAll(tt.name, " ", "-"))
			tempState := filepath.Join(temp, "tmp-"+strings.ReplaceAll(tt.name, " ", "-"))
			for _, path := range []string{home, work, tempState} {
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			seedContaminationSentinels(t, home, work)
			before := snapshotTrees(t, home, work, tempState)
			var stdout, stderr bytes.Buffer
			command := exec.Command(binary, tt.arguments...)
			command.Dir = work
			command.Env = contaminatedEnvironment(home, tempState, proxyURL)
			command.Stdout = &stdout
			command.Stderr = &stderr
			if err := command.Run(); err != nil {
				t.Fatalf("run %v: %v; stderr=%q", tt.arguments, err, stderr.String())
			}
			if stdout.String() != tt.wantOutput {
				t.Fatalf("stdout = %q, want %q", stdout.String(), tt.wantOutput)
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q, want empty", stderr.String())
			}
			if after := snapshotTrees(t, home, work, tempState); fmt.Sprint(after) != fmt.Sprint(before) {
				t.Fatalf("filesystem state changed:\nbefore: %#v\nafter:  %#v", before, after)
			}
			if networkRequests.Load() != 0 {
				t.Fatalf("network requests = %d", networkRequests.Load())
			}
		})
	}
}

func TestPigAIStaticHelpCommands(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	temp := t.TempDir()
	binary := filepath.Join(temp, "pig-ai")
	wantHelp := readFixture(t, root, "internal/pigaicli/testdata/pig_ai_help.golden.txt")
	build := exec.Command("go", "build", "-o", binary, "./cmd/pig-ai")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build command: %v\n%s", err, output)
	}

	for _, tt := range []struct {
		name      string
		arguments []string
	}{
		{name: "no arguments"},
		{name: "help command", arguments: []string{"help"}},
		{name: "long help", arguments: []string{"--help"}},
		{name: "short help", arguments: []string{"-h"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			proxyURL, networkRequests := startNetworkProbe(t)
			home := filepath.Join(temp, "home-"+strings.ReplaceAll(tt.name, " ", "-"))
			work := filepath.Join(temp, "work-"+strings.ReplaceAll(tt.name, " ", "-"))
			tempState := filepath.Join(temp, "tmp-"+strings.ReplaceAll(tt.name, " ", "-"))
			for _, path := range []string{home, work, tempState} {
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			seedContaminationSentinels(t, home, work)
			before := snapshotTrees(t, home, work, tempState)
			var stdout, stderr bytes.Buffer
			command := exec.Command(binary, tt.arguments...)
			command.Dir = work
			command.Env = contaminatedEnvironment(home, tempState, proxyURL)
			command.Stdout = &stdout
			command.Stderr = &stderr
			if err := command.Run(); err != nil {
				t.Fatalf("run %v: %v; stderr=%q", tt.arguments, err, stderr.String())
			}
			if stdout.String() != wantHelp {
				t.Fatalf("stdout = %q, want %q", stdout.String(), wantHelp)
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q, want empty", stderr.String())
			}
			if after := snapshotTrees(t, home, work, tempState); fmt.Sprint(after) != fmt.Sprint(before) {
				t.Fatalf("filesystem state changed:\nbefore: %#v\nafter:  %#v", before, after)
			}
			if networkRequests.Load() != 0 {
				t.Fatalf("network requests = %d", networkRequests.Load())
			}
		})
	}
}

func TestProductArgumentErrorsUseExactExitContract(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	temp := t.TempDir()

	binaries := map[string]string{}
	for _, path := range []string{"./cmd/pig", "./cmd/pig-ai"} {
		binary := filepath.Join(temp, filepath.Base(path))
		build := exec.Command("go", "build", "-o", binary, path)
		build.Dir = root
		if output, err := build.CombinedOutput(); err != nil {
			t.Fatalf("build %s: %v\n%s", path, err, output)
		}
		binaries[path] = binary
	}

	for _, tt := range []struct {
		name       string
		path       string
		arguments  []string
		wantStderr string
	}{
		{name: "pig invalid mode", path: "./cmd/pig", arguments: []string{"--mode", "invalid"}, wantStderr: "Error: Invalid mode \"invalid\". Valid values: text, json, rpc\n"},
		{name: "pig missing install source", path: "./cmd/pig", arguments: []string{"install"}, wantStderr: "Error: Missing install source.\n"},
		{name: "pig-ai version absent", path: "./cmd/pig-ai", arguments: []string{"--version"}, wantStderr: "Error: Unknown command: --version\n"},
		{name: "pig-ai logout absent", path: "./cmd/pig-ai", arguments: []string{"logout"}, wantStderr: "Error: Unknown command: logout\n"},
		{name: "pig-ai missing auth path", path: "./cmd/pig-ai", arguments: []string{"list", "--auth-path"}, wantStderr: "Error: --auth-path requires a value\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			command := exec.Command(binaries[tt.path], tt.arguments...)
			command.Stdout = &stdout
			command.Stderr = &stderr
			err := command.Run()
			if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
				t.Fatalf("command error = %v, want exit 1", err)
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want empty", stdout.String())
			}
			if got := stderr.String(); got != tt.wantStderr {
				t.Fatalf("stderr = %q, want %q", got, tt.wantStderr)
			}
		})
	}
}

func TestPigRedirectedCharacterDeviceSelectsPrintMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("/dev/null is Unix-specific")
	}
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	temp := t.TempDir()
	binary := filepath.Join(temp, "pig")
	build := exec.Command("go", "build", "-o", binary, "./cmd/pig")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build command: %v\n%s", err, output)
	}
	null, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()
	var stderr bytes.Buffer
	command := exec.Command(binary)
	command.Stdin = null
	command.Stdout = null
	command.Stderr = &stderr
	err = command.Run()
	if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
		t.Fatalf("run error = %v, want exit 1", err)
	}
	if got, want := stderr.String(), "Error: Headless mode requires --provider <provider>\n"; got != want {
		t.Fatalf("stderr = %q, want %q", got, want)
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

func seedContaminationSentinels(t *testing.T, home, work string) {
	t.Helper()
	for path, contents := range map[string]string{
		filepath.Join(home, ".pi", "agent", "auth.json"):  "pi-home-secret",
		filepath.Join(home, ".pig", "agent", "auth.json"): "{malformed-pig-auth",
		filepath.Join(home, ".pig", "settings.json"):      "{malformed-pig-settings",
		filepath.Join(work, ".pi", "settings.json"):       "pi-project-state",
		filepath.Join(work, ".pig", "settings.json"):      "{malformed-project-settings",
		filepath.Join(work, "auth.json"):                  "cwd-auth-secret",
		filepath.Join(work, "explicit-auth.json"):         "{malformed-explicit-auth",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func contaminatedEnvironment(home, tempState, proxyURL string) []string {
	return []string{
		"HOME=" + home,
		"TMPDIR=" + tempState,
		"HTTP_PROXY=" + proxyURL,
		"HTTPS_PROXY=" + proxyURL,
		"ALL_PROXY=" + proxyURL,
		"NO_PROXY=",
		"PI_CODING_AGENT_DIR=" + filepath.Join(home, ".pi"),
		"PI_PROVIDER=contamination-provider",
		"PI_MODEL=contamination-model",
		"PI_OFFLINE=0",
	}
}

func snapshotTrees(t *testing.T, roots ...string) []string {
	t.Helper()
	var snapshot []string
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			item := root + "/" + relative + " " + info.Mode().String()
			if !entry.IsDir() {
				contents, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				item += " " + string(contents)
			}
			snapshot = append(snapshot, item)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return snapshot
}

func readFixture(t *testing.T, root, path string) string {
	t.Helper()
	contents, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}
