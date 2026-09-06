package codingagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
)

func CreateFindToolDefinition(cwd string, options ...FindToolOptions) (ToolDefinition, error) {
	var operations FindOperations
	if len(options) > 0 {
		operations = options[0].Operations
	}
	return ToolDefinition{
		Name: "find", Label: "find",
		Description:   fmt.Sprintf("Search for files by glob pattern. Returns matching file paths relative to the search directory. Respects .gitignore. Output is truncated to 1000 results or %dKB (whichever is hit first).", DefaultMaxBytes/1024),
		PromptSnippet: "Find files by glob pattern (respects .gitignore)",
		Parameters:    json.RawMessage(`{"type":"object","properties":{"pattern":{"type":"string","description":"Glob pattern to match files, e.g. '*.ts', '**/*.json', or 'src/**/*.spec.ts'"},"path":{"type":"string","description":"Directory to search in (default: current directory)"},"limit":{"type":"number","description":"Maximum number of results (default: 1000)"}},"required":["pattern"]}`),
		Execute: func(ctx context.Context, _ string, args ai.JSONValue, _ agent.AgentToolUpdateCallback[ai.JSONValue]) (agent.ErasedAgentToolResult, error) {
			if err := context.Cause(ctx); err != nil {
				return agent.ErasedAgentToolResult{}, err
			}
			object, _ := args.(map[string]any)
			pattern, ok := object["pattern"].(string)
			if !ok {
				return agent.ErasedAgentToolResult{}, fmt.Errorf("find requires string pattern")
			}
			path, limit, err := directoryToolArguments(cwd, args, 1000)
			if err != nil {
				return agent.ErasedAgentToolResult{}, err
			}
			var paths []string
			if operations != nil {
				exists, err := operations.Exists(ctx, path)
				if err != nil {
					return agent.ErasedAgentToolResult{}, err
				}
				if err := context.Cause(ctx); err != nil {
					return agent.ErasedAgentToolResult{}, err
				}
				if !exists {
					return agent.ErasedAgentToolResult{}, fmt.Errorf("Path not found: %s", path)
				}
				if limit != math.Trunc(limit) || limit >= float64(math.MaxInt) || limit < float64(math.MinInt) {
					return agent.ErasedAgentToolResult{}, fmt.Errorf("custom find operations require an integer limit representable by Go int")
				}
				paths, err = operations.Glob(ctx, pattern, path, FindGlobOptions{Ignore: []string{"**/node_modules/**", "**/.git/**"}, Limit: int(limit)})
				if err != nil {
					return agent.ErasedAgentToolResult{}, err
				}
			} else {
				paths, err = runFind(ctx, pattern, path, limit)
			}
			if cause := context.Cause(ctx); cause != nil {
				return agent.ErasedAgentToolResult{}, cause
			}
			if err != nil {
				return agent.ErasedAgentToolResult{}, err
			}
			if len(paths) == 0 {
				return directoryToolResult("No files found matching pattern", nil), nil
			}
			paths = append([]string{}, paths...)
			for i, p := range paths {
				trailing := strings.HasSuffix(p, string(filepath.Separator)) || runtime.GOOS == "windows" && strings.HasSuffix(p, "/")
				if filepath.IsAbs(p) {
					p, err = filepath.Rel(path, p)
					if p == "." {
						p = ""
					}
					if err != nil {
						return agent.ErasedAgentToolResult{}, err
					}
				}
				p = filepath.ToSlash(p)
				if trailing && !strings.HasSuffix(p, "/") {
					p += "/"
				}
				paths[i] = p
			}
			notice := ""
			details := map[string]any{}
			if float64(len(paths)) >= limit {
				notice = fmt.Sprintf("%s results limit reached", directoryLimitString(limit))
				if operations == nil {
					notice += fmt.Sprintf(". Use limit=%s for more, or refine pattern", directoryLimitString(limit*2))
				}
				details["resultLimitReached"] = limit
			}
			return formatDirectoryResults(paths, notice, details), nil
		},
	}, nil
}

func CreateFindTool(cwd string, options ...FindToolOptions) (agent.ErasedAgentTool, error) {
	definition, err := CreateFindToolDefinition(cwd, options...)
	if err != nil {
		return agent.ErasedAgentTool{}, err
	}
	return eraseDirectoryTool(definition)
}

func findBinary(ctx context.Context) (string, error) {
	dir, err := GetAgentDir()
	if err != nil {
		return "", err
	}
	name := "fd"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	local := filepath.Join(dir, "bin", name)
	if _, err := os.Stat(local); err == nil {
		return local, nil
	}
	for _, name := range []string{"fd", "fdfind"} {
		if path, err := exec.LookPath(name); err == nil {
			command, err := findCommand(ctx, path, "--version")
			if err != nil {
				return "", err
			}
			err = command.Run()
			if cause := context.Cause(ctx); cause != nil {
				return "", cause
			}
			var exit *exec.ExitError
			if err == nil || errors.As(err, &exit) {
				return path, nil
			}
		}
	}
	return "", fmt.Errorf("fd is not available; install fd (or fdfind) on PATH or in %s; automatic downloads are disabled", filepath.Join(dir, "bin"))
}

func runFind(ctx context.Context, pattern, path string, limit float64) ([]string, error) {
	binary, err := findBinary(ctx)
	if err != nil {
		return nil, err
	}
	args := []string{"--glob", "--color=never", "--hidden"}
	insideGit := false
	for current := path; ; current = filepath.Dir(current) {
		if err := context.Cause(ctx); err != nil {
			return nil, err
		}
		if readToolPathExists(filepath.Join(current, ".git")) {
			insideGit = true
			break
		}
		if filepath.Dir(current) == current {
			break
		}
	}
	if !insideGit {
		args = append(args, "--no-require-git")
	}
	args = append(args, "--max-results", directoryLimitString(limit))
	if strings.Contains(pattern, "/") {
		args = append(args, "--full-path")
		if !strings.HasPrefix(pattern, "/") && !strings.HasPrefix(pattern, "**/") && pattern != "**" {
			pattern = "**/" + pattern
		}
		if runtime.GOOS == "windows" {
			pattern = strings.ReplaceAll(pattern, "/", `[/\]`)
		}
	}
	args = append(args, "--", pattern, path)
	command, err := findCommand(ctx, binary, args...)
	if err != nil {
		return nil, err
	}
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err = command.Run()
	if cause := context.Cause(ctx); cause != nil {
		return nil, cause
	}
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			return nil, fmt.Errorf("Failed to run fd: %w", err)
		}
		if stdout.Len() == 0 {
			message := strings.TrimSpace(stderr.String())
			if message == "" {
				message = fmt.Sprintf("fd exited with code %d", exit.ExitCode())
			}
			return nil, errors.New(message)
		}
	}
	lines := []string{}
	for _, line := range strings.Split(strings.ReplaceAll(stdout.String(), "\r", "\n"), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines, nil
}

func findCommand(ctx context.Context, binary string, args ...string) (*exec.Cmd, error) {
	command := exec.CommandContext(ctx, binary, args...)
	if err := configureBashProcess(command); err != nil {
		return nil, notImplemented("FindOperations.platform")
	}
	command.Cancel = func() error { killBashProcess(command.Process); return nil }
	command.WaitDelay = time.Second
	return command, nil
}
