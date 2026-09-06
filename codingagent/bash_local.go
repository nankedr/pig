package codingagent

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

type localBashOperations struct{ shellPath string }

func CreateLocalBashOperations(options ...BashToolOptions) BashOperations {
	var shellPath string
	if len(options) > 0 {
		shellPath = options[0].ShellPath
	}
	return localBashOperations{shellPath: shellPath}
}

func GetShellConfig(paths ...string) (ShellConfig, error) {
	if len(paths) > 0 && paths[0] != "" {
		if _, err := os.Stat(paths[0]); err != nil {
			return ShellConfig{}, fmt.Errorf("Custom shell path not found: %s", paths[0])
		}
		return ShellConfig{Shell: paths[0], Args: []string{"-c"}}, nil
	}
	return defaultBashShell()
}

func shellEnvironment() map[string]string {
	env := map[string]string{}
	for _, entry := range os.Environ() {
		key, value, _ := strings.Cut(entry, "=")
		env[key] = value
	}
	if dir, err := GetAgentDir(); err == nil {
		bin := filepath.Join(dir, "bin")
		if !slices.Contains(filepath.SplitList(env["PATH"]), bin) {
			env["PATH"] = bin + string(os.PathListSeparator) + env["PATH"]
		}
	}
	return env
}

func (o localBashOperations) Exec(ctx context.Context, command, cwd string, options BashExecOptions) (BashExecResult, error) {
	var timeout <-chan time.Time
	if options.Timeout != nil {
		seconds := *options.Timeout
		if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds <= 0 {
			return BashExecResult{}, fmt.Errorf("Invalid timeout: must be a finite number of seconds")
		}
		if seconds > 2147483.647 {
			return BashExecResult{}, fmt.Errorf("Invalid timeout: maximum is 2147483.647 seconds")
		}
	}
	if ctx.Err() != nil {
		return BashExecResult{}, errors.New("aborted")
	}
	config, err := GetShellConfig(o.shellPath)
	if err != nil {
		return BashExecResult{}, err
	}
	if _, err := os.Stat(cwd); err != nil {
		return BashExecResult{}, fmt.Errorf("Working directory does not exist: %s\nCannot execute bash commands.", cwd)
	}
	cmd := exec.Command(config.Shell, append(config.Args, command)...)
	cmd.Dir = cwd
	env := options.Env
	if env == nil {
		env = shellEnvironment()
	}
	cmd.Env = make([]string, 0, len(env))
	for k, v := range env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	if err := configureBashProcess(cmd); err != nil {
		return BashExecResult{}, err
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		return BashExecResult{}, err
	}
	defer reader.Close()
	cmd.Stdout, cmd.Stderr = writer, writer
	err = cmd.Start()
	writer.Close()
	if err != nil {
		return BashExecResult{}, err
	}
	chunks := make(chan []byte)
	stop := make(chan struct{})
	drained := make(chan struct{})
	go func() {
		defer close(drained)
		defer close(chunks)
		buf := make([]byte, 32768)
		for {
			n, err := reader.Read(buf)
			if n > 0 {
				select {
				case chunks <- append([]byte(nil), buf[:n]...):
				case <-stop:
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()
	defer func() { close(stop); reader.Close(); <-drained }()
	waited := make(chan error, 1)
	go func() { waited <- cmd.Wait() }()
	if options.Timeout != nil {
		timer := time.NewTimer(time.Duration(*options.Timeout * float64(time.Second)))
		defer timer.Stop()
		timeout = timer.C
	}
	idle := time.NewTimer(time.Hour)
	idle.Stop()
	defer idle.Stop()
	var idleC <-chan time.Time
	canceled, timedOut, exited := false, false, false
	ctxDone := ctx.Done()
	for !exited || chunks != nil {
		select {
		case data, ok := <-chunks:
			if !ok {
				chunks = nil
				continue
			}
			if options.OnData != nil {
				options.OnData(data)
			}
			if exited {
				idle.Reset(100 * time.Millisecond)
				idleC = idle.C
			}
		case <-waited:
			waited = nil
			exited = true
			idle.Reset(100 * time.Millisecond)
			idleC = idle.C
		case <-idleC:
			chunks = nil
		case <-ctxDone:
			canceled = true
			ctxDone = nil
			killBashProcess(cmd.Process)
		case <-timeout:
			timedOut = true
			timeout = nil
			killBashProcess(cmd.Process)
		}
	}
	if canceled || ctx.Err() != nil {
		return BashExecResult{}, errors.New("aborted")
	}
	if timedOut {
		return BashExecResult{}, fmt.Errorf("timeout:%g", *options.Timeout)
	}
	code := cmd.ProcessState.ExitCode()
	if code < 0 {
		return BashExecResult{}, nil
	}
	return BashExecResult{ExitCode: &code}, nil
}
