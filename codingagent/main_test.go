package codingagent_test

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/nankedr/pig/codingagent"
)

func TestMainDispatchesStaticMetadataToProcessStdout(t *testing.T) {
	help, err := os.ReadFile(filepath.Join("testdata", "pig_help.golden.txt"))
	if err != nil {
		t.Fatal(err)
	}

	for _, test := range []struct {
		name       string
		arguments  []string
		wantStdout string
	}{
		{name: "help", arguments: []string{"--help"}, wantStdout: string(help)},
		{name: "version", arguments: []string{"--version"}, wantStdout: codingagent.Version + "\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			stdout, err := captureMainStdout(t, func() error {
				return codingagent.Main(context.Background(), test.arguments)
			})
			if err != nil {
				t.Fatalf("Main(%q): %v", test.arguments, err)
			}
			if stdout != test.wantStdout {
				t.Fatalf("Main(%q) stdout = %q, want %q", test.arguments, stdout, test.wantStdout)
			}
		})
	}
}

func TestMainPreservesDeferredMainOptions(t *testing.T) {
	options := codingagent.MainOptions{
		ExtensionFactories: []codingagent.InlineExtension{nil},
	}
	stdout, err := captureMainStdout(t, func() error {
		return codingagent.Main(context.Background(), []string{"--version"}, options)
	})
	if err != nil {
		t.Fatalf("Main(--version, options): %v", err)
	}
	if want := codingagent.Version + "\n"; stdout != want {
		t.Fatalf("Main(--version, options) stdout = %q, want %q", stdout, want)
	}

	stdout, err = captureMainStdout(t, func() error {
		return codingagent.Main(context.Background(), nil, options)
	})
	if stdout != "" {
		t.Fatalf("Main(nil, options) stdout = %q, want empty", stdout)
	}
	var unavailable *codingagent.NotImplementedError
	if !errors.As(err, &unavailable) || unavailable.Operation != "mode.print.text" {
		t.Fatalf("Main(nil, options) error = %#v, want codingagent.mode.print.text", err)
	}
}

func TestMainPropagatesCancellationBeforeWritingOutput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stdout, err := captureMainStdout(t, func() error {
		return codingagent.Main(ctx, []string{"--help"})
	})
	if stdout != "" {
		t.Fatalf("Main(canceled, --help) stdout = %q, want empty", stdout)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Main(canceled, --help) error = %v, want context.Canceled", err)
	}
}

func TestMainReportsWarningsBeforeReturningAModeStub(t *testing.T) {
	stderr, err := captureMainStderr(t, func() error {
		return codingagent.Main(context.Background(), []string{"--thinking", "invalid"})
	})
	wantStderr := "Warning: Invalid thinking level \"invalid\". Valid values: off, minimal, low, medium, high, xhigh, max\n"
	if stderr != wantStderr {
		t.Fatalf("Main(--thinking invalid) stderr = %q, want %q", stderr, wantStderr)
	}
	var unavailable *codingagent.NotImplementedError
	if !errors.As(err, &unavailable) || unavailable.Operation != "mode.print.text" {
		t.Fatalf("Main(--thinking invalid) error = %#v, want codingagent.mode.print.text", err)
	}
}

func captureMainStdout(t *testing.T, run func() error) (string, error) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = writer
	defer func() { os.Stdout = original }()
	runErr := run()
	os.Stdout = original
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	return string(output), runErr
}

func captureMainStderr(t *testing.T, run func() error) (string, error) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stderr
	os.Stderr = writer
	defer func() { os.Stderr = original }()
	runErr := run()
	os.Stderr = original
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	return string(output), runErr
}
