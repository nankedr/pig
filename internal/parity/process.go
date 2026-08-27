package parity

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

type CLIInput struct {
	Arguments []string `json:"arguments"`
	Stdin     *string  `json:"stdin,omitempty"`
}

type CommandDriver struct {
	Path string
	Dir  string
	Env  []string
}

func (CommandDriver) Surface() Surface { return SurfaceCLI }

func (d CommandDriver) Observe(ctx context.Context, c Case) (Observation, error) {
	var input CLIInput
	if err := decodeSingleJSON(c.Input, &input, disallowUnknownJSON); err != nil {
		return Observation{}, fmt.Errorf("decode CLI input: %w", err)
	}

	command := exec.CommandContext(ctx, d.Path, input.Arguments...)
	command.Dir = d.Dir
	command.Env = d.Env
	if input.Stdin != nil {
		command.Stdin = strings.NewReader(*input.Stdin)
	}
	var stdout, stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	if ctx.Err() != nil {
		return Observation{}, fmt.Errorf("run %s: %w", d.Path, ctx.Err())
	}
	var exitError *exec.ExitError
	if err != nil && !errors.As(err, &exitError) {
		return Observation{}, fmt.Errorf("run %s: %w", d.Path, err)
	}
	code := command.ProcessState.ExitCode()
	status := &ExitStatus{}
	if code >= 0 {
		status.Code = &code
	} else {
		status.Signal = exitSignal(command.ProcessState)
	}
	stdoutText, stderrText := stdout.String(), stderr.String()
	return Observation{Stdout: &stdoutText, Stderr: &stderrText, ExitStatus: status}, nil
}
