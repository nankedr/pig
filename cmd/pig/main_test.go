package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestProcessReportsStdoutWriteFailure(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate test source")
	}
	packageDir := filepath.Dir(file)
	binary := filepath.Join(t.TempDir(), "pig")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	build := exec.Command("go", "build", "-o", binary, ".")
	build.Dir = packageDir
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build pig: %v\n%s", err, output)
	}

	for _, arguments := range [][]string{{"--help"}, {"--version"}} {
		t.Run(arguments[0], func(t *testing.T) {
			stdoutPath := filepath.Join(t.TempDir(), "stdout")
			if err := os.WriteFile(stdoutPath, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			stdout, err := os.Open(stdoutPath)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := stdout.Close(); err != nil {
					t.Errorf("close stdout: %v", err)
				}
			})

			var stderr bytes.Buffer
			command := exec.Command(binary, arguments...)
			command.Stdout = stdout
			command.Stderr = &stderr
			err = command.Run()
			if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 1 {
				t.Fatalf("run %v error = %v, want exit 1", arguments, err)
			}
			if got, want := stderr.String(), "Error: Failed to write stdout.\n"; got != want {
				t.Fatalf("stderr = %q, want %q", got, want)
			}
		})
	}
}
