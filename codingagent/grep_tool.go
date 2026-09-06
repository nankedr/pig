package codingagent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf16"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"golang.org/x/text/encoding/unicode"
)

type grepAbortError struct{ cause error }

func (e grepAbortError) Error() string { return "Operation aborted" }
func (e grepAbortError) Unwrap() error { return e.cause }

const grepMaxLineLength = 500

const grepPromptSnippet = "Search file contents for patterns (respects .gitignore)"

func CreateGrepToolDefinition(cwd string, options ...GrepToolOptions) (ToolDefinition, error) {
	var ops GrepOperations = localGrepOperations{}
	if len(options) > 0 && options[0].Operations != nil {
		ops = options[0].Operations
	}
	return ToolDefinition{Name: "grep", Label: "grep", Description: "Search file contents for a pattern. Returns matching lines with file paths and line numbers. Respects .gitignore. Output is truncated to 100 matches or 50KB (whichever is hit first). Long lines are truncated to 500 chars.", PromptSnippet: grepPromptSnippet,
		Parameters: json.RawMessage(`{"type":"object","properties":{"pattern":{"type":"string","description":"Search pattern (regex or literal string)"},"path":{"type":"string","description":"Directory or file to search (default: current directory)"},"glob":{"type":"string","description":"Filter files by glob pattern, e.g. '*.ts' or '**/*.spec.ts'"},"ignoreCase":{"type":"boolean","description":"Case-insensitive search (default: false)"},"literal":{"type":"boolean","description":"Treat pattern as literal string instead of regex (default: false)"},"context":{"type":"number","description":"Number of lines to show before and after each match (default: 0)"},"limit":{"type":"number","description":"Maximum number of matches to return (default: 100)"}},"required":["pattern"]}`),
		Execute: func(ctx context.Context, _ string, value ai.JSONValue, _ agent.AgentToolUpdateCallback[ai.JSONValue]) (agent.ErasedAgentToolResult, error) {
			args, _ := value.(map[string]any)
			pattern, ok := args["pattern"].(string)
			if !ok {
				return agent.ErasedAgentToolResult{}, errors.New("grep requires string pattern")
			}
			return executeGrep(ctx, cwd, ops, pattern, args)
		}}, nil
}

func CreateGrepTool(cwd string, options ...GrepToolOptions) (agent.ErasedAgentTool, error) {
	definition, err := CreateGrepToolDefinition(cwd, options...)
	if err != nil {
		return agent.ErasedAgentTool{}, err
	}
	return agent.EraseAgentTool(agent.AgentTool[ai.JSONValue, ai.JSONValue]{Tool: ai.Tool{Name: definition.Name, Description: definition.Description, Parameters: definition.Parameters}, Label: definition.Label, DecodeValidated: func(v ai.JSONValue) ai.JSONValue { return v }, Execute: definition.Execute})
}

func executeGrep(ctx context.Context, cwd string, ops GrepOperations, pattern string, args map[string]any) (agent.ErasedAgentToolResult, error) {
	if ctx.Err() != nil {
		return agent.ErasedAgentToolResult{}, grepAbortError{context.Cause(ctx)}
	}
	binary, err := grepBinary(ctx)
	if err != nil {
		return agent.ErasedAgentToolResult{}, err
	}
	path, _ := args["path"].(string)
	if path == "" {
		path = "."
	}
	path, err = resolveToolPath(path, cwd)
	if err != nil {
		return agent.ErasedAgentToolResult{}, err
	}
	directory, err := ops.IsDirectory(ctx, path)
	if ctx.Err() != nil {
		return agent.ErasedAgentToolResult{}, grepAbortError{context.Cause(ctx)}
	}
	if err != nil {
		return agent.ErasedAgentToolResult{}, fmt.Errorf("Path not found: %s", path)
	}
	contextLines, _ := readToolNumber(args["context"])
	contextLines = max(0, contextLines)
	limit := 100.0
	if number, ok := readToolNumber(args["limit"]); ok {
		limit = max(1, number)
	}
	commandArgs := []string{"--json", "--line-number", "--color=never", "--hidden"}
	if args["ignoreCase"] == true {
		commandArgs = append(commandArgs, "--ignore-case")
	}
	if args["literal"] == true {
		commandArgs = append(commandArgs, "--fixed-strings")
	}
	if glob, _ := args["glob"].(string); glob != "" {
		commandArgs = append(commandArgs, "--glob", glob)
	}
	commandArgs = append(commandArgs, "--", pattern, path)
	cmd := exec.Command(binary, commandArgs...)
	if err := configureBashProcess(cmd); err != nil {
		return agent.ErasedAgentToolResult{}, err
	}
	type match struct {
		Path       struct{ Text string }
		Lines      struct{ Text *string }
		LineNumber *float64 `json:"line_number"`
	}
	var matches []match
	count := 0.0
	limitReached := false
	running := true
	writer := &grepEventWriter{onLine: func(line []byte) {
		if count >= limit {
			return
		}
		var event struct {
			Type string
			Data match
		}
		if json.Unmarshal(line, &event) != nil || event.Type != "match" {
			return
		}
		count++
		if event.Data.Path.Text != "" && event.Data.LineNumber != nil {
			matches = append(matches, event.Data)
		}
		if count >= limit {
			limitReached = true
			if running {
				killBashProcess(cmd.Process)
			}
		}
	}}
	var stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = writer, &stderr
	err = runGrepProcess(ctx, cmd)
	running = false
	writer.flush()
	if ctx.Err() != nil {
		return agent.ErasedAgentToolResult{}, grepAbortError{context.Cause(ctx)}
	}
	if err != nil && !limitReached && (cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != 1) {
		if cmd.ProcessState == nil {
			return agent.ErasedAgentToolResult{}, fmt.Errorf("Failed to run ripgrep: %w", err)
		}
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			code := fmt.Sprint(cmd.ProcessState.ExitCode())
			if code == "-1" {
				code = "null"
			}
			message = "ripgrep exited with code " + code
		}
		return agent.ErasedAgentToolResult{}, errors.New(message)
	}
	result := agent.ErasedAgentToolResult{}
	text := "No matches found"
	if count > 0 {
		var output []string
		linesTruncated := false
		appendLine := func(prefix, line string) {
			units := utf16.Encode([]rune(line))
			if len(units) > grepMaxLineLength {
				line = string(utf16.Decode(units[:grepMaxLineLength])) + "... [truncated]"
				linesTruncated = true
			}
			output = append(output, prefix+line)
		}
		cache := map[string][]string{}
		for _, match := range matches {
			if ctx.Err() != nil {
				return agent.ErasedAgentToolResult{}, grepAbortError{context.Cause(ctx)}
			}
			file := filepath.Base(match.Path.Text)
			if directory {
				if relative, err := filepath.Rel(path, match.Path.Text); err == nil && relative != "" && !strings.HasPrefix(relative, "..") {
					file = strings.ReplaceAll(relative, `\`, "/")
				}
			}
			number := *match.LineNumber
			if contextLines == 0 && match.Lines.Text != nil {
				line := strings.TrimSuffix(strings.ReplaceAll(strings.ReplaceAll(*match.Lines.Text, "\r\n", "\n"), "\r", ""), "\n")
				appendLine(file+":"+formatReadToolNumber(number)+": ", line)
				continue
			}
			lines, ok := cache[match.Path.Text]
			if !ok {
				content, err := ops.ReadFile(ctx, match.Path.Text)
				if ctx.Err() != nil {
					return agent.ErasedAgentToolResult{}, grepAbortError{context.Cause(ctx)}
				}
				if err == nil {
					lines = strings.Split(strings.ReplaceAll(strings.ReplaceAll(content, "\r\n", "\n"), "\r", "\n"), "\n")
				}
				cache[match.Path.Text] = lines
			}
			if len(lines) == 0 {
				output = append(output, file+":"+formatReadToolNumber(number)+": (unable to read file)")
				continue
			}
			start, end := number, number
			if contextLines > 0 {
				start = max(1, number-contextLines)
				end = min(float64(len(lines)), number+contextLines)
			}
			for current := start; current <= end; current++ {
				if ctx.Err() != nil {
					return agent.ErasedAgentToolResult{}, grepAbortError{context.Cause(ctx)}
				}
				line := ""
				if current == math.Trunc(current) && current >= 1 && current <= float64(len(lines)) {
					line = lines[int(current)-1]
				}
				prefix := file + "-" + formatReadToolNumber(current) + "- "
				if current == number {
					prefix = file + ":" + formatReadToolNumber(current) + ": "
				}
				appendLine(prefix, strings.ReplaceAll(line, "\r", ""))
			}
		}
		truncation := TruncateHead(strings.Join(output, "\n"), TruncationOptions{MaxLines: 9007199254740991})
		text = truncation.Content
		details := map[string]any{}
		var notices []string
		if limitReached {
			details["matchLimitReached"] = limit
			notices = append(notices, formatReadToolNumber(limit)+" matches limit reached. Use limit="+formatReadToolNumber(limit*2)+" for more, or refine pattern")
		}
		if truncation.Truncated {
			details["truncation"] = readToolTruncationJSON(truncation)
			notices = append(notices, FormatSize(DefaultMaxBytes)+" limit reached")
		}
		if linesTruncated {
			details["linesTruncated"] = true
			notices = append(notices, "Some lines truncated to 500 chars. Use read tool to see full lines")
		}
		if len(notices) > 0 {
			text += "\n\n[" + strings.Join(notices, ". ") + "]"
			result.Details = details
		}
	}
	result.Content = []ai.ToolResultContent{ai.TextContent{Type: "text", Text: text}}
	return result, nil
}

func runGrepProcess(ctx context.Context, cmd *exec.Cmd) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	canceled := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { killBashProcess(cmd.Process); close(canceled) })
	err := cmd.Wait()
	if !stop() {
		<-canceled
	}
	return err
}

func grepBinary(ctx context.Context) (string, error) {
	dir, err := GetAgentDir()
	if err != nil {
		return "", err
	}
	local := filepath.Join(dir, "bin", "rg")
	if _, err := os.Stat(local); err == nil {
		return local, nil
	}
	probe := exec.Command("rg", "--version")
	if err := configureBashProcess(probe); err != nil {
		return "", err
	}
	probe.Stdout, probe.Stderr = io.Discard, io.Discard
	err = runGrepProcess(ctx, probe)
	if ctx.Err() != nil {
		return "", grepAbortError{context.Cause(ctx)}
	}
	if err == nil || probe.ProcessState != nil {
		return "rg", nil
	}
	mode := "Automatic download is not implemented"
	if ResolveOffline(false) {
		mode = "Offline mode enabled, skipping download"
	}
	return "", fmt.Errorf("ripgrep (rg) is not available. %s. Install ripgrep in PATH or %s", mode, local)
}

type grepEventWriter struct {
	pending []byte
	onLine  func([]byte)
}

func (w *grepEventWriter) Write(data []byte) (int, error) {
	size := len(data)
	w.pending = append(w.pending, data...)
	for {
		i := bytes.IndexByte(w.pending, '\n')
		if i < 0 {
			break
		}
		w.onLine(w.pending[:i])
		w.pending = w.pending[i+1:]
	}
	return size, nil
}

func (w *grepEventWriter) flush() {
	if len(w.pending) > 0 {
		w.onLine(w.pending)
		w.pending = nil
	}
}

type localGrepOperations struct{}

func (localGrepOperations) IsDirectory(ctx context.Context, path string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return false, err
	}
	return info.IsDir(), nil
}
func (localGrepOperations) ReadFile(ctx context.Context, path string) (string, error) {
	data, err := (localReadOperations{}).ReadFile(ctx, path)
	if err != nil {
		return "", err
	}
	return unicode.UTF8.NewDecoder().String(string(data))
}

func (details GrepToolDetails) MarshalJSON() ([]byte, error) {
	payload := map[string]any{}
	if details.Truncation != nil {
		payload["truncation"] = readToolTruncationJSON(*details.Truncation)
	}
	if details.MatchLimitReached != nil {
		payload["matchLimitReached"] = *details.MatchLimitReached
	}
	if details.LinesTruncated {
		payload["linesTruncated"] = true
	}
	return json.Marshal(payload)
}
