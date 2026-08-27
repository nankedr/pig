package parity_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/nankedr/pig/internal/parity"
)

func TestCommandDriverCapturesProductOutputAndFailureExit(t *testing.T) {
	input, err := json.Marshal(parity.CLIInput{
		Arguments: []string{"-test.run=^TestCommandDriverHelperProcess$"},
		Stdin:     pointer("request\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	driver := parity.CommandDriver{
		Path: os.Args[0],
		Env:  append(os.Environ(), "PIG_PARITY_HELPER=1"),
	}

	observation, err := driver.Observe(context.Background(), parity.Case{Input: input})
	if err != nil {
		t.Fatalf("Observe() = %v", err)
	}
	if observation.Stdout == nil || *observation.Stdout != "stdout: request\n" {
		t.Fatalf("stdout = %v", observation.Stdout)
	}
	if observation.Stderr == nil || *observation.Stderr != "stderr\n" {
		t.Fatalf("stderr = %v", observation.Stderr)
	}
	if observation.ExitStatus == nil || observation.ExitStatus.Code == nil || *observation.ExitStatus.Code != 7 || observation.ExitStatus.Signal != "" {
		t.Fatalf("exit status = %+v", observation.ExitStatus)
	}
}

func TestCommandDriverHelperProcess(t *testing.T) {
	if os.Getenv("PIG_PARITY_HELPER") != "1" {
		return
	}
	var input string
	if _, err := fmt.Scan(&input); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	fmt.Fprintf(os.Stdout, "stdout: %s\n", input)
	fmt.Fprintln(os.Stderr, "stderr")
	os.Exit(7)
}
