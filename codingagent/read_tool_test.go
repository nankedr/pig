package codingagent_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func TestReadToolResolvesHostPathsWithoutWorkspaceContainment(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "workspace")
	if err := os.Mkdir(cwd, 0o700); err != nil {
		t.Fatal(err)
	}
	relativePath := filepath.Join(cwd, "relative.txt")
	parentPath := filepath.Join(root, "parent.txt")
	absolutePath := filepath.Join(root, "absolute.txt")
	for path, content := range map[string]string{
		relativePath: "relative sentinel",
		parentPath:   "parent sentinel",
		absolutePath: "absolute sentinel",
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	symlinkPath := filepath.Join(cwd, "linked.txt")
	if err := os.Symlink(parentPath, symlinkPath); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "cwd relative", path: "relative.txt", want: "relative sentinel"},
		{name: "absolute", path: absolutePath, want: "absolute sentinel"},
		{name: "parent traversal", path: filepath.Join("..", "parent.txt"), want: "parent sentinel"},
		{name: "symlink", path: "linked.txt", want: "parent sentinel"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := runReadTool(t, context.Background(), cwd, map[string]any{"path": test.path})
			if result.IsError {
				t.Fatalf("ToolResult = %#v, want success", result)
			}
			if got := readToolResultText(t, result); got != test.want {
				t.Fatalf("read output = %q, want %q", got, test.want)
			}
		})
	}
}

func TestReadToolNormalizesPinnedPiPathFormsAndFallbacks(t *testing.T) {
	cwd := t.TempDir()
	paths := map[string]string{
		"plain name.txt": "unicode space",
		"at-prefix.txt":  "at prefix",
		"Screenshot 2026-09-02 at 1.00.00\u202fPM.png.txt": "narrow no-break space",
		"cafe\u0301.txt":                 "NFD",
		"Capture d\u2019ecran.txt":       "curly quote",
		"Capture d\u2019e\u0301cran.txt": "combined NFD and curly quote",
	}
	for name, content := range paths {
		if err := os.WriteFile(filepath.Join(cwd, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	absolutePath := filepath.Join(cwd, "at-prefix.txt")
	fileURL := (&url.URL{Scheme: "file", Path: absolutePath}).String()
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "unicode space", path: "plain\u00a0name.txt", want: "unicode space"},
		{name: "leading at", path: "@at-prefix.txt", want: "at prefix"},
		{name: "file URL", path: fileURL, want: "at prefix"},
		{name: "macOS AM PM", path: "Screenshot 2026-09-02 at 1.00.00 PM.png.txt", want: "narrow no-break space"},
		{name: "NFD", path: "caf\u00e9.txt", want: "NFD"},
		{name: "curly quote", path: "Capture d'ecran.txt", want: "curly quote"},
		{name: "combined NFD and curly quote", path: "Capture d'\u00e9cran.txt", want: "combined NFD and curly quote"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := runReadTool(t, context.Background(), cwd, map[string]any{"path": test.path})
			if result.IsError || readToolResultText(t, result) != test.want {
				t.Fatalf("ToolResult = %#v, want text %q", result, test.want)
			}
		})
	}

	t.Run("tilde", func(t *testing.T) {
		var accessed string
		operations := readOperationsStub{
			content:  []byte("home"),
			onAccess: func(path string) { accessed = path },
		}
		result := runReadTool(t, context.Background(), cwd, map[string]any{"path": "~"}, codingagent.ReadToolOptions{Operations: operations})
		if result.IsError || readToolResultText(t, result) != "home" {
			t.Fatalf("ToolResult = %#v, want home content", result)
		}
		home, err := os.UserHomeDir()
		if err != nil {
			t.Fatal(err)
		}
		if accessed != home {
			t.Fatalf("Access path = %q, want home %q", accessed, home)
		}
	})
}

func TestReadToolRejectsInvalidFileURLs(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "encoded path separator", path: "file:///tmp/encoded%2Fseparator.txt", want: "File URL path must not include encoded / characters"},
		{name: "lowercase encoded path separator", path: "file:///tmp/encoded%2fseparator.txt", want: "File URL path must not include encoded / characters"},
		{name: "malformed escape", path: "file:///tmp/malformed%zz.txt", want: "invalid URL escape"},
		{name: "remote host", path: "file://example.com/tmp/remote.txt", want: "File URL host must be \"localhost\" or empty"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := runReadTool(t, context.Background(), t.TempDir(), map[string]any{"path": test.path})
			if !result.IsError || !strings.Contains(readToolResultText(t, result), test.want) {
				t.Fatalf("ToolResult = %#v, want error containing %q", result, test.want)
			}
		})
	}
}

func TestReadToolAllowsTextOnlyCustomOperations(t *testing.T) {
	operations := textOnlyReadOperationsStub{content: []byte("remote text")}
	var _ codingagent.ReadOperations = operations
	result := runReadTool(t, context.Background(), t.TempDir(), map[string]any{"path": "remote.txt"}, codingagent.ReadToolOptions{Operations: operations})
	if result.IsError || readToolResultText(t, result) != "remote text" {
		t.Fatalf("ToolResult = %#v, want text-only custom operations to succeed", result)
	}
}

func TestReadToolUsesOneIndexedOffsetAndLimit(t *testing.T) {
	cwd := t.TempDir()
	content := "Line 1\nLine 2\nLine 3\nLine 4\nLine 5"
	if err := os.WriteFile(filepath.Join(cwd, "lines.txt"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		arguments map[string]any
		want      string
	}{
		{name: "complete file", arguments: map[string]any{"path": "lines.txt"}, want: content},
		{name: "offset", arguments: map[string]any{"path": "lines.txt", "offset": 3}, want: "Line 3\nLine 4\nLine 5"},
		{name: "limit", arguments: map[string]any{"path": "lines.txt", "limit": 2}, want: "Line 1\nLine 2\n\n[3 more lines in file. Use offset=3 to continue.]"},
		{name: "offset and limit", arguments: map[string]any{"path": "lines.txt", "offset": 3, "limit": 2}, want: "Line 3\nLine 4\n\n[1 more lines in file. Use offset=5 to continue.]"},
		{name: "fractional offset and limit", arguments: map[string]any{"path": "lines.txt", "offset": 2.5, "limit": 1.5}, want: "Line 2\nLine 3\n\n[2 more lines in file. Use offset=4 to continue.]"},
		{name: "oversized limit", arguments: map[string]any{"path": "lines.txt", "limit": 1e100}, want: content},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := runReadTool(t, context.Background(), cwd, test.arguments)
			if result.IsError {
				t.Fatalf("ToolResult = %#v, want success", result)
			}
			if got := readToolResultText(t, result); got != test.want {
				t.Fatalf("read output = %q, want %q", got, test.want)
			}
		})
	}

	for _, test := range []struct {
		offset float64
		want   string
	}{
		{offset: 1e20, want: "Offset 100000000000000000000 is beyond end of file (5 lines total)"},
		{offset: 1e21, want: "Offset 1e+21 is beyond end of file (5 lines total)"},
		{offset: 1e100, want: "Offset 1e+100 is beyond end of file (5 lines total)"},
	} {
		result := runReadTool(t, context.Background(), cwd, map[string]any{"path": "lines.txt", "offset": test.offset})
		if !result.IsError || readToolResultText(t, result) != test.want {
			t.Fatalf("offset %v ToolResult = %#v, want %q", test.offset, result, test.want)
		}
	}
}

func TestReadToolReportsDeterministicLineAndByteTruncation(t *testing.T) {
	cwd := t.TempDir()
	lineLimited := make([]string, 2500)
	for index := range lineLimited {
		lineLimited[index] = fmt.Sprintf("Line %d", index+1)
	}
	byteLimited := make([]string, 500)
	for index := range byteLimited {
		byteLimited[index] = fmt.Sprintf("Line %d: %s", index+1, strings.Repeat("x", 200))
	}
	files := map[string]string{
		"lines.txt": strings.Join(lineLimited, "\n"),
		"bytes.txt": strings.Join(byteLimited, "\n"),
		"long.txt":  strings.Repeat("é", agent.DefaultMaxBytes/2+1),
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(cwd, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("line limit", func(t *testing.T) {
		result := runReadTool(t, context.Background(), cwd, map[string]any{"path": "lines.txt"})
		text := readToolResultText(t, result)
		if !strings.HasSuffix(text, "Line 2000\n\n[Showing lines 1-2000 of 2500. Use offset=2001 to continue.]") || strings.Contains(text, "Line 2001") {
			t.Fatalf("line-truncated output has wrong boundary: %q", text[len(text)-120:])
		}
		want := map[string]any{
			"truncation": map[string]any{
				"content": strings.Join(lineLimited[:2000], "\n"), "truncated": true, "truncatedBy": "lines",
				"totalLines": float64(2500), "totalBytes": float64(len([]byte(files["lines.txt"]))),
				"outputLines": float64(2000), "outputBytes": float64(len([]byte(strings.Join(lineLimited[:2000], "\n")))),
				"lastLinePartial": false, "firstLineExceedsLimit": false,
				"maxLines": float64(agent.DefaultMaxLines), "maxBytes": float64(agent.DefaultMaxBytes),
			},
		}
		if got := readToolResultDetails(t, result); !reflect.DeepEqual(got, want) {
			t.Fatalf("details = %#v, want %#v", got, want)
		}
	})

	t.Run("byte limit", func(t *testing.T) {
		result := runReadTool(t, context.Background(), cwd, map[string]any{"path": "bytes.txt"})
		text := readToolResultText(t, result)
		details := readToolResultDetails(t, result)["truncation"].(map[string]any)
		outputLines := int(details["outputLines"].(float64))
		wantNotice := fmt.Sprintf("[Showing lines 1-%d of 500 (50.0KB limit). Use offset=%d to continue.]", outputLines, outputLines+1)
		if details["truncatedBy"] != "bytes" || details["outputBytes"].(float64) > agent.DefaultMaxBytes || !strings.HasSuffix(text, wantNotice) {
			t.Fatalf("byte truncation output/details = %q / %#v", text[len(text)-120:], details)
		}
	})

	t.Run("oversized first line", func(t *testing.T) {
		result := runReadTool(t, context.Background(), cwd, map[string]any{"path": "long.txt"})
		want := "[Line 1 is 50.0KB, exceeds 50.0KB limit. Use bash: sed -n '1p' long.txt | head -c 51200]"
		if got := readToolResultText(t, result); got != want {
			t.Fatalf("long-line output = %q, want %q", got, want)
		}
		details := readToolResultDetails(t, result)["truncation"].(map[string]any)
		if details["truncatedBy"] != "bytes" || details["outputLines"] != float64(0) || details["firstLineExceedsLimit"] != true {
			t.Fatalf("long-line details = %#v", details)
		}
	})
}

func TestReadToolTurnsReadFailuresIntoCompatibleToolResults(t *testing.T) {
	cwd := t.TempDir()
	if err := os.WriteFile(filepath.Join(cwd, "short.txt"), []byte("Line 1\nLine 2\nLine 3"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(cwd, "directory"), 0o700); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		arguments  map[string]any
		operations codingagent.ReadOperations
		want       string
		contains   bool
	}{
		{name: "missing file", arguments: map[string]any{"path": "missing.txt"}, want: "no such file", contains: true},
		{name: "directory", arguments: map[string]any{"path": "directory"}, want: "directory", contains: true},
		{name: "offset beyond end", arguments: map[string]any{"path": "short.txt", "offset": 100}, want: "Offset 100 is beyond end of file (3 lines total)"},
		{name: "access failure", arguments: map[string]any{"path": "remote.txt"}, operations: readOperationsStub{accessErr: errors.New("access failed")}, want: "access failed"},
		{name: "image detection failure", arguments: map[string]any{"path": "remote.txt"}, operations: readOperationsStub{detectErr: errors.New("detect failed")}, want: "detect failed"},
		{name: "read failure", arguments: map[string]any{"path": "remote.txt"}, operations: readOperationsStub{readErr: errors.New("read failed")}, want: "read failed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options := []codingagent.ReadToolOptions{}
			if test.operations != nil {
				options = append(options, codingagent.ReadToolOptions{Operations: test.operations})
			}
			result := runReadTool(t, context.Background(), cwd, test.arguments, options...)
			if !result.IsError {
				t.Fatalf("ToolResult = %#v, want error", result)
			}
			got := readToolResultText(t, result)
			if (test.contains && !strings.Contains(strings.ToLower(got), strings.ToLower(test.want))) || (!test.contains && got != test.want) {
				t.Fatalf("error text = %q, want %q (contains=%t)", got, test.want, test.contains)
			}
			if details, ok := result.Details.Value(); !ok || !reflect.DeepEqual(details, map[string]any{}) {
				t.Fatalf("error details = %#v, want empty object", result.Details)
			}
		})
	}
}

func TestReadToolTurnsContextCancellationIntoCompatibleToolResult(t *testing.T) {
	cause := errors.New("cancel read")
	ctx, cancel := context.WithCancelCause(context.Background())
	operations := readOperationsStub{
		onRead: func(ctx context.Context) ([]byte, error) {
			cancel(cause)
			<-ctx.Done()
			return nil, context.Cause(ctx)
		},
	}
	created, err := promptReadTool(t, ctx, t.TempDir(), map[string]any{"path": "remote.txt"}, codingagent.ReadToolOptions{Operations: operations})
	if !errors.Is(err, cause) {
		t.Fatalf("Prompt() error = %v, want cancellation cause %v", err, cause)
	}
	messages := created.State().Messages
	if len(messages) != 3 {
		t.Fatalf("transcript = %#v, want user/Assistant/ToolResult", messages)
	}
	result, ok := messages[2].(ai.ToolResultMessage)
	if !ok || !result.IsError || readToolResultText(t, result) != "Operation aborted" {
		t.Fatalf("canceled read ToolResult = %#v", messages[2])
	}
}

func TestReadToolLeavesImageReadingExplicitlyUnimplemented(t *testing.T) {
	cwd := t.TempDir()
	image := append([]byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 13}, []byte("IHDR")...)
	if err := os.WriteFile(filepath.Join(cwd, "image.bin"), image, 0o600); err != nil {
		t.Fatal(err)
	}

	result := runReadTool(t, context.Background(), cwd, map[string]any{"path": "image.bin"})
	if !result.IsError || readToolResultText(t, result) != "codingagent.ReadTool.Image: not implemented" {
		t.Fatalf("image ToolResult = %#v", result)
	}
}

func TestReadToolContinuesFauxAgentWithRealFileContent(t *testing.T) {
	cwd := t.TempDir()
	const sentinel = "issue-54-real-read-sentinel"
	if err := os.WriteFile(filepath.Join(cwd, "sentinel.txt"), []byte(sentinel), 0o600); err != nil {
		t.Fatal(err)
	}
	tool, err := codingagent.CreateReadTool(cwd)
	if err != nil {
		t.Fatal(err)
	}
	call, err := ai.FauxToolCall("read", map[string]any{"path": "sentinel.txt"}, ai.FauxToolCallOptions{ID: ai.Some("read-sentinel")})
	if err != nil {
		t.Fatal(err)
	}
	toolResponse, err := ai.FauxAssistantMessage(ai.FauxAssistantBlocks(call), ai.FauxAssistantMessageOptions{
		StopReason: ai.Some(ai.StopReasonToolUse), Timestamp: ai.Some(int64(2)),
	})
	if err != nil {
		t.Fatal(err)
	}
	const finalText = "The file contains issue-54-real-read-sentinel."
	finalResponse, err := ai.FauxAssistantMessage(ai.FauxAssistantText(finalText), ai.FauxAssistantMessageOptions{Timestamp: ai.Some(int64(4))})
	if err != nil {
		t.Fatal(err)
	}
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	core.SetResponses([]ai.FauxResponseStep{
		toolResponse,
		ai.FauxResponseFactory(func(input ai.Context, _ *ai.SimpleStreamOptions, _ *ai.FauxProviderState, _ ai.Model) (ai.AssistantMessage, error) {
			if len(input.Messages) != 3 {
				t.Fatalf("continuation context = %#v, want user/Assistant/ToolResult", input.Messages)
			}
			result, ok := input.Messages[2].(ai.ToolResultMessage)
			if !ok || result.IsError || result.ToolCallID != "read-sentinel" || readToolResultText(t, result) != sentinel {
				t.Fatalf("continuation ToolResult = %#v", input.Messages[2])
			}
			return finalResponse, nil
		}),
	})
	model, _ := core.GetModel()
	created, err := agent.NewAgent(agent.AgentOptions{
		InitialState:   &agent.AgentInitialState{Model: model, Tools: []agent.ErasedAgentTool{tool}},
		StreamFunction: agent.StreamFunction(core.StreamSimple),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := created.Prompt(context.Background(), ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("read the sentinel"), Timestamp: 1}); err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}
	messages := created.State().Messages
	if len(messages) != 4 {
		t.Fatalf("transcript = %#v, want four messages", messages)
	}
	final, ok := messages[3].(ai.AssistantMessage)
	if !ok || len(final.Content) != 1 || final.Content[0].(ai.TextContent).Text != finalText {
		t.Fatalf("final Assistant message = %#v", messages[3])
	}
}

type readOperationsStub struct {
	accessErr error
	detectErr error
	readErr   error
	mimeType  *string
	content   []byte
	onAccess  func(string)
	onRead    func(context.Context) ([]byte, error)
}

type textOnlyReadOperationsStub struct{ content []byte }

func (textOnlyReadOperationsStub) Access(context.Context, string) error { return nil }

func (stub textOnlyReadOperationsStub) ReadFile(context.Context, string) ([]byte, error) {
	return stub.content, nil
}

func (stub readOperationsStub) Access(_ context.Context, path string) error {
	if stub.onAccess != nil {
		stub.onAccess(path)
	}
	return stub.accessErr
}

func (stub readOperationsStub) DetectImageMimeType(context.Context, string) (*string, error) {
	return stub.mimeType, stub.detectErr
}

func (stub readOperationsStub) ReadFile(ctx context.Context, _ string) ([]byte, error) {
	if stub.onRead != nil {
		return stub.onRead(ctx)
	}
	return stub.content, stub.readErr
}

func runReadTool(t *testing.T, ctx context.Context, cwd string, arguments map[string]any, options ...codingagent.ReadToolOptions) ai.ToolResultMessage {
	t.Helper()
	created, err := promptReadTool(t, ctx, cwd, arguments, options...)
	if err != nil {
		t.Fatalf("Prompt() error = %v", err)
	}
	messages := created.State().Messages
	if len(messages) != 3 {
		t.Fatalf("transcript = %#v, want user/Assistant/ToolResult", messages)
	}
	result, ok := messages[2].(ai.ToolResultMessage)
	if !ok {
		t.Fatalf("final message = %T, want ai.ToolResultMessage", messages[2])
	}
	return result
}

func promptReadTool(t *testing.T, ctx context.Context, cwd string, arguments map[string]any, options ...codingagent.ReadToolOptions) (*agent.Agent, error) {
	t.Helper()
	tool, err := codingagent.CreateReadTool(cwd, options...)
	if err != nil {
		t.Fatalf("CreateReadTool() error = %v", err)
	}
	call, err := ai.FauxToolCall("read", arguments, ai.FauxToolCallOptions{ID: ai.Some("read-call")})
	if err != nil {
		t.Fatal(err)
	}
	response, err := ai.FauxAssistantMessage(ai.FauxAssistantBlocks(call), ai.FauxAssistantMessageOptions{
		StopReason: ai.Some(ai.StopReasonToolUse), Timestamp: ai.Some(int64(2)),
	})
	if err != nil {
		t.Fatal(err)
	}
	core, err := ai.CreateFauxCore(ai.RegisterFauxProviderOptions{})
	if err != nil {
		t.Fatal(err)
	}
	core.SetResponses([]ai.FauxResponseStep{response})
	model, _ := core.GetModel()
	created, err := agent.NewAgent(agent.AgentOptions{
		InitialState:   &agent.AgentInitialState{Model: model, Tools: []agent.ErasedAgentTool{tool}},
		StreamFunction: agent.StreamFunction(core.StreamSimple),
		ShouldStopAfterTurn: func(context.Context, agent.ShouldStopAfterTurnContext) (bool, error) {
			return true, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	err = created.Prompt(ctx, ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("read it"), Timestamp: 1})
	return created, err
}

func readToolResultText(t *testing.T, result ai.ToolResultMessage) string {
	t.Helper()
	if len(result.Content) != 1 {
		t.Fatalf("ToolResult content = %#v, want one text block", result.Content)
	}
	text, ok := result.Content[0].(ai.TextContent)
	if !ok {
		t.Fatalf("ToolResult content = %T, want ai.TextContent", result.Content[0])
	}
	return text.Text
}

func readToolResultDetails(t *testing.T, result ai.ToolResultMessage) map[string]any {
	t.Helper()
	details, ok := result.Details.Value()
	if !ok {
		t.Fatalf("ToolResult details = %#v, want value", result.Details)
	}
	object, ok := details.(map[string]any)
	if !ok {
		t.Fatalf("ToolResult details = %T, want map[string]any", details)
	}
	return object
}
