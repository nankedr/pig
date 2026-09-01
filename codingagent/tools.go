package codingagent

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
	"golang.org/x/text/unicode/norm"
)

const (
	DefaultMaxLines = agent.DefaultMaxLines
	DefaultMaxBytes = agent.DefaultMaxBytes
)

type TruncationLimit = agent.TruncationLimit

const (
	TruncationByLines = agent.TruncationByLines
	TruncationByBytes = agent.TruncationByBytes
)

type TruncationOptions = agent.TruncationOptions

type TruncationResult = agent.TruncationResult

func TruncateHead(content string, options ...TruncationOptions) TruncationResult {
	limits := TruncationOptions{}
	if len(options) != 0 {
		limits = options[0]
	}
	return agent.TruncateHead(content, limits)
}

func TruncateTail(content string, options ...TruncationOptions) TruncationResult {
	limits := TruncationOptions{}
	if len(options) != 0 {
		limits = options[0]
	}
	return agent.TruncateTail(content, limits)
}

type LineTruncation = agent.LineTruncation

func TruncateLine(line string, maxCharacters ...int) LineTruncation {
	limit := 0
	if len(maxCharacters) != 0 {
		limit = maxCharacters[0]
	}
	return agent.TruncateLine(line, limit)
}

func FormatSize(bytes int64) string {
	return agent.FormatSize(bytes)
}

type EditDiffResult struct {
	Diff             string
	FirstChangedLine int
}

type lineChange struct {
	kind       byte
	text       string
	terminated bool
}

type diffLine struct {
	raw        string
	text       string
	terminated bool
}

type lineChangeNode struct {
	change   lineChange
	previous *lineChangeNode
}

const maxLineDiffWork = 1 << 20

func splitDiffLines(content string) []diffLine {
	if content == "" {
		return nil
	}
	lines := make([]diffLine, 0, strings.Count(content, "\n")+1)
	for len(content) != 0 {
		newline := strings.IndexByte(content, '\n')
		if newline == -1 {
			lines = append(lines, diffLine{raw: content, text: content})
			break
		}
		raw := content[:newline+1]
		lines = append(lines, diffLine{raw: raw, text: raw[:len(raw)-1], terminated: true})
		content = content[newline+1:]
	}
	return lines
}

func lineChanges(oldContent, newContent string) []lineChange {
	oldLines := splitDiffLines(oldContent)
	newLines := splitDiffLines(newContent)

	type editPath struct {
		oldPosition int
		lastChange  *lineChangeNode
	}
	appendChange := func(path editPath, kind byte, line diffLine) editPath {
		path.lastChange = &lineChangeNode{
			change:   lineChange{kind: kind, text: line.text, terminated: line.terminated},
			previous: path.lastChange,
		}
		return path
	}
	extractCommon := func(path editPath, diagonal int) (editPath, int) {
		newPosition := path.oldPosition - diagonal
		for path.oldPosition+1 < len(oldLines) && newPosition+1 < len(newLines) &&
			oldLines[path.oldPosition+1].raw == newLines[newPosition+1].raw {
			path.oldPosition++
			newPosition++
			line := oldLines[path.oldPosition]
			path = appendChange(path, ' ', line)
		}
		return path, newPosition
	}

	initial, newPosition := extractCommon(editPath{oldPosition: -1}, 0)
	if initial.oldPosition+1 >= len(oldLines) && newPosition+1 >= len(newLines) {
		return collectLineChanges(initial.lastChange)
	}
	paths := map[int]editPath{0: initial}
	work := 0
	for editDistance := 1; editDistance <= len(oldLines)+len(newLines); editDistance++ {
		for diagonal := -editDistance; diagonal <= editDistance; diagonal += 2 {
			work++
			if work > maxLineDiffWork {
				return replacementLineChanges(oldLines, newLines)
			}
			removePath, canRemovePath := paths[diagonal-1]
			addPath, canAddPath := paths[diagonal+1]
			canAdd := canAddPath && addPath.oldPosition-diagonal >= 0 && addPath.oldPosition-diagonal < len(newLines)
			canRemove := canRemovePath && removePath.oldPosition+1 < len(oldLines)
			if !canAdd && !canRemove {
				delete(paths, diagonal)
				continue
			}

			var path editPath
			if !canRemove || (canAdd && removePath.oldPosition < addPath.oldPosition) {
				newIndex := addPath.oldPosition - diagonal
				path = appendChange(addPath, '+', newLines[newIndex])
			} else {
				removePath.oldPosition++
				path = appendChange(removePath, '-', oldLines[removePath.oldPosition])
			}
			path, newPosition = extractCommon(path, diagonal)
			if path.oldPosition+1 >= len(oldLines) && newPosition+1 >= len(newLines) {
				return collectLineChanges(path.lastChange)
			}
			paths[diagonal] = path
		}
	}
	return nil
}

func collectLineChanges(last *lineChangeNode) []lineChange {
	length := 0
	for node := last; node != nil; node = node.previous {
		length++
	}
	changes := make([]lineChange, length)
	for index := length - 1; index >= 0; index-- {
		changes[index] = last.change
		last = last.previous
	}
	return changes
}

func replacementLineChanges(oldLines, newLines []diffLine) []lineChange {
	changes := make([]lineChange, 0, len(oldLines)+len(newLines))
	for _, line := range oldLines {
		changes = append(changes, lineChange{kind: '-', text: line.text, terminated: line.terminated})
	}
	for _, line := range newLines {
		changes = append(changes, lineChange{kind: '+', text: line.text, terminated: line.terminated})
	}
	return changes
}

func GenerateDiffString(oldContent, newContent string, contextLines ...int) EditDiffResult {
	context := 4
	if len(contextLines) != 0 {
		context = max(0, contextLines[0])
	}
	changes := lineChanges(oldContent, newContent)
	changed := make([]bool, len(changes))
	firstChanged, newLine := 0, 1
	for i, change := range changes {
		if change.kind != ' ' {
			changed[i] = true
			if firstChanged == 0 {
				firstChanged = newLine
			}
		}
		if change.kind != '-' {
			newLine++
		}
	}
	if firstChanged == 0 {
		return EditDiffResult{}
	}
	show := make([]bool, len(changes))
	for i := range changes {
		if !changed[i] {
			continue
		}
		for j := max(0, i-context); j <= min(len(changes)-1, i+context); j++ {
			show[j] = true
		}
	}
	oldLine, newLine := 1, 1
	width := len(fmt.Sprint(max(len(strings.Split(oldContent, "\n")), len(strings.Split(newContent, "\n")))))
	var lines []string
	inGap := false
	for i, change := range changes {
		if !show[i] {
			if !inGap && len(lines) != 0 {
				lines = append(lines, " "+strings.Repeat(" ", width)+" ...")
			}
			inGap = true
		} else {
			inGap = false
			lineNumber := oldLine
			if change.kind == '+' {
				lineNumber = newLine
			}
			lines = append(lines, fmt.Sprintf("%c%*d %s", change.kind, width, lineNumber, change.text))
		}
		if change.kind != '+' {
			oldLine++
		}
		if change.kind != '-' {
			newLine++
		}
	}
	return EditDiffResult{Diff: strings.Join(lines, "\n"), FirstChangedLine: firstChanged}
}

func GenerateUnifiedPatch(path, oldContent, newContent string, contextLines ...int) string {
	context := 4
	if len(contextLines) != 0 {
		context = max(0, contextLines[0])
	}
	changes := lineChanges(oldContent, newContent)
	var output strings.Builder
	fmt.Fprintf(&output, "--- %s\n+++ %s\n", path, path)

	for _, bounds := range unifiedHunkBounds(changes, context) {
		oldStart, newStart := linePositionsAt(changes, bounds[0])
		oldCount, newCount := 0, 0
		for _, change := range changes[bounds[0]:bounds[1]] {
			if change.kind != '+' {
				oldCount++
			}
			if change.kind != '-' {
				newCount++
			}
		}
		if oldCount == 0 {
			oldStart--
		}
		if newCount == 0 {
			newStart--
		}
		fmt.Fprintf(&output, "@@ -%d,%d +%d,%d @@\n", oldStart, oldCount, newStart, newCount)
		for _, change := range changes[bounds[0]:bounds[1]] {
			output.WriteByte(change.kind)
			output.WriteString(change.text)
			output.WriteByte('\n')
			if !change.terminated {
				output.WriteString("\\ No newline at end of file\n")
			}
		}
	}
	return output.String()
}

func unifiedHunkBounds(changes []lineChange, context int) [][2]int {
	var hunks [][2]int
	for index := 0; index < len(changes); {
		for index < len(changes) && changes[index].kind == ' ' {
			index++
		}
		if index == len(changes) {
			break
		}
		start := max(0, index-context)
		lastChange := index
		for index++; index < len(changes); index++ {
			if changes[index].kind != ' ' {
				if index-lastChange-1 > context*2 {
					break
				}
				lastChange = index
			}
		}
		hunks = append(hunks, [2]int{start, min(len(changes), lastChange+context+1)})
	}
	return hunks
}

func linePositionsAt(changes []lineChange, index int) (oldLine, newLine int) {
	oldLine, newLine = 1, 1
	for _, change := range changes[:index] {
		if change.kind != '+' {
			oldLine++
		}
		if change.kind != '-' {
			newLine++
		}
	}
	return oldLine, newLine
}

type BashToolInput struct {
	Command string
	Timeout *float64
}

type BashToolDetails struct {
	Truncation     *TruncationResult
	FullOutputPath string
}

type BashExecOptions struct {
	OnData  func([]byte)
	Timeout *float64
	Env     map[string]string
}

type BashExecResult struct {
	ExitCode *int
}

type BashOperations interface {
	Exec(context.Context, string, string, BashExecOptions) (BashExecResult, error)
}

type BashSpawnContext struct {
	Command string
	CWD     string
	Env     map[string]string
}

type BashSpawnHook func(BashSpawnContext) BashSpawnContext

type BashToolOptions struct {
	Operations               BashOperations
	CommandPrefix            string
	ShellPath                string
	ExposeSessionEnvironment *bool
	SpawnHook                BashSpawnHook
}

type Edit struct {
	OldText string
	NewText string
}

type EditToolInput struct {
	Path  string
	Edits []Edit
}

type EditToolDetails struct {
	Diff             string
	Patch            string
	FirstChangedLine int
}

type EditOperations interface {
	Access(context.Context, string) error
	ReadFile(context.Context, string) ([]byte, error)
	WriteFile(context.Context, string, []byte) error
}

type EditToolOptions struct{ Operations EditOperations }

type FindToolInput struct {
	Pattern string
	Path    string
	Limit   *int
}

type FindToolDetails struct {
	Truncation         *TruncationResult
	ResultLimitReached *int
}

type FindGlobOptions struct {
	Ignore []string
	Limit  int
}

type FindOperations interface {
	Exists(context.Context, string) (bool, error)
	Glob(context.Context, string, string, FindGlobOptions) ([]string, error)
}

type FindToolOptions struct{ Operations FindOperations }

type GrepToolInput struct {
	Pattern    string
	Path       string
	Glob       string
	IgnoreCase bool
	Literal    bool
	Context    *int
	Limit      *int
}

type GrepToolDetails struct {
	Truncation        *TruncationResult
	MatchLimitReached *int
	LinesTruncated    bool
}

type GrepOperations interface {
	IsDirectory(context.Context, string) (bool, error)
	ReadFile(context.Context, string) (string, error)
}

type GrepToolOptions struct{ Operations GrepOperations }

type LsToolInput struct {
	Path  string
	Limit *int
}

type LsToolDetails struct {
	Truncation        *TruncationResult
	EntryLimitReached *int
}

type LsFileInfo interface{ IsDirectory() bool }

type LsOperations interface {
	Exists(context.Context, string) (bool, error)
	Readdir(context.Context, string) ([]string, error)
	Stat(context.Context, string) (LsFileInfo, error)
}

type LsToolOptions struct{ Operations LsOperations }

type ReadToolInput struct {
	Path   string
	Offset *int
	Limit  *int
}

type ReadToolDetails struct {
	Truncation *TruncationResult
}

type ReadOperations interface {
	Access(context.Context, string) error
	ReadFile(context.Context, string) ([]byte, error)
}

// ReadImageOperations is the optional image-classification capability for
// ReadOperations. Implementations that only serve text files need not provide
// it.
type ReadImageOperations interface {
	ReadOperations
	DetectImageMimeType(context.Context, string) (*string, error)
}

type ReadToolOptions struct {
	AutoResizeImages *bool
	Operations       ReadOperations
}

type WriteToolInput struct {
	Path    string
	Content string
}

type WriteOperations interface {
	Mkdir(context.Context, string) error
	WriteFile(context.Context, string, []byte) error
}

type WriteToolOptions struct{ Operations WriteOperations }

type ToolsOptions struct {
	Bash  BashToolOptions
	Edit  EditToolOptions
	Find  FindToolOptions
	Grep  GrepToolOptions
	Ls    LsToolOptions
	Read  ReadToolOptions
	Write WriteToolOptions
}

type ToolDefinition struct {
	ConstrainedSampling ai.ConstrainedSampling
	Description         string
	Execute             ExtensionHandler
	ExecutionMode       agent.ToolExecutionMode
	Label               string
	Name                string
	Parameters          json.RawMessage
	PrepareArguments    ExtensionHandler
	PromptGuidelines    []string
	PromptSnippet       string
	RenderCall          ExtensionHandler
	RenderResult        ExtensionHandler
	RenderShell         string
}

func CreateBashToolDefinition(string, ...BashToolOptions) (ToolDefinition, error) {
	return ToolDefinition{}, notImplemented("CreateBashToolDefinition")
}

func CreateEditToolDefinition(string, ...EditToolOptions) (ToolDefinition, error) {
	return ToolDefinition{}, notImplemented("CreateEditToolDefinition")
}

func CreateFindToolDefinition(string, ...FindToolOptions) (ToolDefinition, error) {
	return ToolDefinition{}, notImplemented("CreateFindToolDefinition")
}

func CreateGrepToolDefinition(string, ...GrepToolOptions) (ToolDefinition, error) {
	return ToolDefinition{}, notImplemented("CreateGrepToolDefinition")
}

func CreateLsToolDefinition(string, ...LsToolOptions) (ToolDefinition, error) {
	return ToolDefinition{}, notImplemented("CreateLsToolDefinition")
}

func CreateReadToolDefinition(string, ...ReadToolOptions) (ToolDefinition, error) {
	return ToolDefinition{}, notImplemented("CreateReadToolDefinition")
}

func CreateWriteToolDefinition(string, ...WriteToolOptions) (ToolDefinition, error) {
	return ToolDefinition{}, notImplemented("CreateWriteToolDefinition")
}

func CreateBashTool(cwd string, options ...BashToolOptions) (agent.ErasedAgentTool, error) {
	_, _ = cwd, options
	return agent.ErasedAgentTool{}, notImplemented("CreateBashTool")
}
func CreateEditTool(cwd string, options ...EditToolOptions) (agent.ErasedAgentTool, error) {
	_, _ = cwd, options
	return agent.ErasedAgentTool{}, notImplemented("CreateEditTool")
}
func CreateFindTool(cwd string, options ...FindToolOptions) (agent.ErasedAgentTool, error) {
	_, _ = cwd, options
	return agent.ErasedAgentTool{}, notImplemented("CreateFindTool")
}
func CreateGrepTool(cwd string, options ...GrepToolOptions) (agent.ErasedAgentTool, error) {
	_, _ = cwd, options
	return agent.ErasedAgentTool{}, notImplemented("CreateGrepTool")
}
func CreateLsTool(cwd string, options ...LsToolOptions) (agent.ErasedAgentTool, error) {
	_, _ = cwd, options
	return agent.ErasedAgentTool{}, notImplemented("CreateLsTool")
}
func CreateReadTool(cwd string, options ...ReadToolOptions) (agent.ErasedAgentTool, error) {
	configured := ReadToolOptions{}
	if len(options) != 0 {
		configured = options[0]
	}
	operations := configured.Operations
	if operations == nil {
		operations = localReadOperations{}
	}
	return agent.EraseAgentTool(agent.AgentTool[readToolExecutionInput, *ReadToolDetails]{
		Tool: ai.Tool{
			Name:        "read",
			Description: fmt.Sprintf("Read the contents of a file. Supports text files and images (jpg, png, gif, webp, bmp). Images are sent as attachments. For text files, output is truncated to %d lines or %dKB (whichever is hit first). Use offset/limit for large files. When you need the full file, continue with offset until complete.", DefaultMaxLines, DefaultMaxBytes/1024),
			Parameters:  json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","description":"Path to the file to read (relative or absolute)"},"offset":{"type":"number","description":"Line number to start reading from (1-indexed)"},"limit":{"type":"number","description":"Maximum number of lines to read"}},"required":["path"]}`),
		},
		Label: "read",
		DecodeValidated: func(value ai.JSONValue) readToolExecutionInput {
			object := value.(map[string]any)
			input := readToolExecutionInput{Path: object["path"].(string)}
			if offset, ok := readToolNumber(object["offset"]); ok {
				input.Offset = &offset
			}
			if limit, ok := readToolNumber(object["limit"]); ok {
				input.Limit = &limit
			}
			return input
		},
		Execute: func(ctx context.Context, _ string, input readToolExecutionInput, _ agent.AgentToolUpdateCallback[*ReadToolDetails]) (agent.AgentToolResult[*ReadToolDetails], error) {
			return executeReadTool(ctx, cwd, operations, input)
		},
	})
}

// readToolExecutionInput retains the number-valued JSON Schema semantics at
// the typed execution boundary. The exported ReadToolInput remains the stable
// Go-facing carrier for callers that work with integral line numbers.
type readToolExecutionInput struct {
	Path   string
	Offset *float64
	Limit  *float64
}

func (details ReadToolDetails) MarshalJSON() ([]byte, error) {
	payload := map[string]any{}
	if details.Truncation != nil {
		payload["truncation"] = readToolTruncationJSON(*details.Truncation)
	}
	return json.Marshal(payload)
}

func readToolTruncationJSON(truncation TruncationResult) map[string]any {
	return map[string]any{
		"content":               truncation.Content,
		"truncated":             truncation.Truncated,
		"truncatedBy":           truncation.TruncatedBy,
		"totalLines":            truncation.TotalLines,
		"totalBytes":            truncation.TotalBytes,
		"outputLines":           truncation.OutputLines,
		"outputBytes":           truncation.OutputBytes,
		"lastLinePartial":       truncation.LastLinePartial,
		"firstLineExceedsLimit": truncation.FirstLineExceedsLimit,
		"maxLines":              truncation.MaxLines,
		"maxBytes":              truncation.MaxBytes,
	}
}

func readToolNumber(value any) (float64, bool) {
	switch value := value.(type) {
	case float64:
		return value, true
	case float32:
		return float64(value), true
	case json.Number:
		number, err := value.Float64()
		return number, err == nil
	case int:
		return float64(value), true
	case int8:
		return float64(value), true
	case int16:
		return float64(value), true
	case int32:
		return float64(value), true
	case int64:
		return float64(value), true
	case uint:
		return float64(value), true
	case uint8:
		return float64(value), true
	case uint16:
		return float64(value), true
	case uint32:
		return float64(value), true
	case uint64:
		return float64(value), true
	default:
		return 0, false
	}
}
func CreateWriteTool(cwd string, options ...WriteToolOptions) (agent.ErasedAgentTool, error) {
	_, _ = cwd, options
	return agent.ErasedAgentTool{}, notImplemented("CreateWriteTool")
}

func CreateCodingTools(string, ...ToolsOptions) ([]agent.ErasedAgentTool, error) {
	return nil, notImplemented("CreateCodingTools")
}

func CreateReadOnlyTools(string, ...ToolsOptions) ([]agent.ErasedAgentTool, error) {
	return nil, notImplemented("CreateReadOnlyTools")
}

type localReadOperations struct{}

func (localReadOperations) Access(ctx context.Context, path string) error {
	if err := context.Cause(ctx); err != nil {
		return err
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	return file.Close()
}

func (localReadOperations) DetectImageMimeType(ctx context.Context, path string) (*string, error) {
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	header := make([]byte, 4100)
	count, err := file.Read(header)
	if err != nil && err != io.EOF {
		return nil, err
	}
	mimeType := supportedImageMIMEType(header[:count])
	if mimeType == "" {
		return nil, nil
	}
	return &mimeType, nil
}

func (localReadOperations) ReadFile(ctx context.Context, path string) ([]byte, error) {
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	return content, nil
}

var (
	readToolAMPMPath             = regexp.MustCompile(`(?i) (AM|PM)\.`)
	readToolEncodedPathSeparator = regexp.MustCompile(`(?i)%2f`)
	readToolEncodedWinSeparator  = regexp.MustCompile(`(?i)%5c`)
)

func resolveReadPath(input, cwd string) (string, error) {
	normalized := strings.Map(func(character rune) rune {
		switch {
		case character == '\u00a0', character >= '\u2000' && character <= '\u200a', character == '\u202f', character == '\u205f', character == '\u3000':
			return ' '
		default:
			return character
		}
	}, input)
	if strings.HasPrefix(normalized, "@") {
		normalized = normalized[1:]
	}
	if normalized == "~" || strings.HasPrefix(normalized, "~/") || runtime.GOOS == "windows" && strings.HasPrefix(normalized, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if normalized == "~" {
			normalized = home
		} else {
			normalized = filepath.Join(home, normalized[2:])
		}
	}
	if runtime.GOOS == "windows" {
		normalized = normalizeReadToolWindowsShellPath(normalized)
	}
	if strings.HasPrefix(normalized, "file://") {
		filePath, err := readToolFileURLToPath(normalized)
		if err != nil {
			return "", err
		}
		normalized = filePath
	}
	if !filepath.IsAbs(normalized) {
		normalized = filepath.Join(cwd, normalized)
	}
	resolved, err := filepath.Abs(normalized)
	if err != nil {
		return "", err
	}
	if readToolPathExists(resolved) {
		return resolved, nil
	}
	variants := []string{
		readToolAMPMPath.ReplaceAllString(resolved, "\u202f$1."),
		norm.NFD.String(resolved),
		strings.ReplaceAll(resolved, "'", "\u2019"),
		strings.ReplaceAll(norm.NFD.String(resolved), "'", "\u2019"),
	}
	for _, variant := range variants {
		if variant != resolved && readToolPathExists(variant) {
			return variant, nil
		}
	}
	return resolved, nil
}

func readToolFileURLToPath(input string) (string, error) {
	parsed, err := url.Parse(input)
	if err != nil {
		return "", err
	}
	if parsed.User != nil || parsed.Port() != "" {
		return "", fmt.Errorf("invalid file URL")
	}
	host := parsed.Hostname()
	if runtime.GOOS != "windows" && host != "" && !strings.EqualFold(host, "localhost") {
		return "", fmt.Errorf("File URL host must be \"localhost\" or empty on %s", runtime.GOOS)
	}
	escapedPath := parsed.EscapedPath()
	if readToolEncodedPathSeparator.MatchString(escapedPath) || runtime.GOOS == "windows" && readToolEncodedWinSeparator.MatchString(escapedPath) {
		separators := "/"
		if runtime.GOOS == "windows" {
			separators = "/ or \\"
		}
		return "", fmt.Errorf("File URL path must not include encoded %s characters", separators)
	}
	decodedPath, err := url.PathUnescape(escapedPath)
	if err != nil {
		return "", err
	}
	if decodedPath == "" {
		decodedPath = "/"
	}
	if runtime.GOOS != "windows" {
		return filepath.FromSlash(decodedPath), nil
	}
	if host != "" && !strings.EqualFold(host, "localhost") {
		return `\\` + host + filepath.FromSlash(decodedPath), nil
	}
	path := filepath.FromSlash(decodedPath)
	if len(path) >= 3 && path[0] == '\\' && path[2] == ':' {
		path = path[1:]
	}
	return path, nil
}

func normalizeReadToolWindowsShellPath(path string) string {
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || strings.Contains(path, `\`) {
		return path
	}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) > 1 && (parts[0] == "mnt" || parts[0] == "cygdrive") {
		parts = parts[1:]
	}
	if len(parts) == 0 || len(parts[0]) != 1 || (parts[0][0] < 'a' || parts[0][0] > 'z') && (parts[0][0] < 'A' || parts[0][0] > 'Z') {
		return path
	}
	drive := strings.ToUpper(parts[0]) + `:\`
	return drive + strings.Join(parts[1:], `\`)
}

func readToolPathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func readToolSliceIndex(index float64, length int) int {
	switch {
	case math.IsNaN(index), math.IsInf(index, -1):
		return 0
	case math.IsInf(index, 1), index >= float64(length):
		return length
	case index <= -float64(length):
		return 0
	case index < 0:
		return max(length+int(math.Trunc(index)), 0)
	default:
		return int(math.Trunc(index))
	}
}

func formatReadToolNumber(value float64) string {
	switch {
	case math.IsNaN(value):
		return "NaN"
	case math.IsInf(value, 1):
		return "Infinity"
	case math.IsInf(value, -1):
		return "-Infinity"
	case value == 0:
		return "0"
	}
	magnitude := math.Abs(value)
	if magnitude >= 1e-6 && magnitude < 1e21 {
		return strconv.FormatFloat(value, 'f', -1, 64)
	}
	formatted := strconv.FormatFloat(value, 'e', -1, 64)
	mantissa, exponent, _ := strings.Cut(formatted, "e")
	sign := ""
	if strings.HasPrefix(exponent, "+") || strings.HasPrefix(exponent, "-") {
		sign, exponent = exponent[:1], exponent[1:]
	}
	exponent = strings.TrimLeft(exponent, "0")
	if exponent == "" {
		exponent = "0"
	}
	return mantissa + "e" + sign + exponent
}

func executeReadTool(ctx context.Context, cwd string, operations ReadOperations, input readToolExecutionInput) (agent.AgentToolResult[*ReadToolDetails], error) {
	path, err := resolveReadPath(input.Path, cwd)
	if err != nil {
		return agent.AgentToolResult[*ReadToolDetails]{}, err
	}
	if err := operations.Access(ctx, path); err != nil {
		return agent.AgentToolResult[*ReadToolDetails]{}, err
	}
	if imageOperations, ok := operations.(ReadImageOperations); ok {
		mimeType, err := imageOperations.DetectImageMimeType(ctx, path)
		if err != nil {
			return agent.AgentToolResult[*ReadToolDetails]{}, err
		}
		if mimeType != nil {
			return agent.AgentToolResult[*ReadToolDetails]{}, notImplemented("ReadTool.Image")
		}
	}
	content, err := operations.ReadFile(ctx, path)
	if err != nil {
		return agent.AgentToolResult[*ReadToolDetails]{}, err
	}

	allLines := strings.Split(string(content), "\n")
	startLine := float64(0)
	if input.Offset != nil && *input.Offset != 0 {
		startLine = math.Max(0, *input.Offset-1)
	}
	if startLine >= float64(len(allLines)) {
		return agent.AgentToolResult[*ReadToolDetails]{}, fmt.Errorf("Offset %s is beyond end of file (%d lines total)", formatReadToolNumber(*input.Offset), len(allLines))
	}
	startIndex := readToolSliceIndex(startLine, len(allLines))
	selectedLines := allLines[startIndex:]
	var userLimitEnd *float64
	if input.Limit != nil {
		endLine := math.Min(startLine+*input.Limit, float64(len(allLines)))
		endIndex := readToolSliceIndex(endLine, len(allLines))
		if endIndex < startIndex {
			endIndex = startIndex
		}
		selectedLines = allLines[startIndex:endIndex]
		userLimitEnd = &endLine
	}
	truncation := TruncateHead(strings.Join(selectedLines, "\n"))
	text := truncation.Content
	var details *ReadToolDetails
	startLineDisplay := startLine + 1
	switch {
	case truncation.FirstLineExceedsLimit:
		text = fmt.Sprintf("[Line %s is %s, exceeds %s limit. Use bash: sed -n '%sp' %s | head -c %d]",
			formatReadToolNumber(startLineDisplay), FormatSize(int64(len([]byte(allLines[startIndex])))), FormatSize(DefaultMaxBytes), formatReadToolNumber(startLineDisplay), input.Path, DefaultMaxBytes)
		details = readToolDetails(truncation)
	case truncation.Truncated:
		endLineDisplay := startLineDisplay + float64(truncation.OutputLines) - 1
		nextOffset := endLineDisplay + 1
		if truncation.TruncatedBy == TruncationByLines {
			text += fmt.Sprintf("\n\n[Showing lines %s-%s of %d. Use offset=%s to continue.]", formatReadToolNumber(startLineDisplay), formatReadToolNumber(endLineDisplay), len(allLines), formatReadToolNumber(nextOffset))
		} else {
			text += fmt.Sprintf("\n\n[Showing lines %s-%s of %d (%s limit). Use offset=%s to continue.]", formatReadToolNumber(startLineDisplay), formatReadToolNumber(endLineDisplay), len(allLines), FormatSize(DefaultMaxBytes), formatReadToolNumber(nextOffset))
		}
		details = readToolDetails(truncation)
	case userLimitEnd != nil && *userLimitEnd < float64(len(allLines)):
		remaining := float64(len(allLines)) - *userLimitEnd
		nextOffset := *userLimitEnd + 1
		text += fmt.Sprintf("\n\n[%s more lines in file. Use offset=%s to continue.]", formatReadToolNumber(remaining), formatReadToolNumber(nextOffset))
	}
	return agent.AgentToolResult[*ReadToolDetails]{
		Content: []ai.ToolResultContent{ai.TextContent{Type: ai.ContentTypeText, Text: text}},
		Details: details,
	}, nil
}

func readToolDetails(truncation TruncationResult) *ReadToolDetails {
	return &ReadToolDetails{Truncation: &truncation}
}

func supportedImageMIMEType(content []byte) string {
	switch {
	case validStaticPNG(content):
		return "image/png"
	case bytes.HasPrefix(content, []byte{0xff, 0xd8, 0xff}) && (len(content) < 4 || content[3] != 0xf7):
		return "image/jpeg"
	case bytes.HasPrefix(content, []byte("GIF")):
		return "image/gif"
	case len(content) >= 12 && bytes.Equal(content[:4], []byte("RIFF")) && bytes.Equal(content[8:12], []byte("WEBP")):
		return "image/webp"
	case validBMP(content):
		return "image/bmp"
	default:
		return ""
	}
}

func validStaticPNG(content []byte) bool {
	if len(content) < 16 || !bytes.HasPrefix(content, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}) || binary.BigEndian.Uint32(content[8:12]) != 13 || !bytes.Equal(content[12:16], []byte("IHDR")) {
		return false
	}
	for offset := 8; offset+8 <= len(content); {
		length := int64(binary.BigEndian.Uint32(content[offset : offset+4]))
		chunkType := content[offset+4 : offset+8]
		if bytes.Equal(chunkType, []byte("acTL")) {
			return false
		}
		if bytes.Equal(chunkType, []byte("IDAT")) {
			return true
		}
		next := int64(offset) + 12 + length
		if next <= int64(offset) || next > int64(len(content)) {
			return true
		}
		offset = int(next)
	}
	return true
}

func validBMP(content []byte) bool {
	if len(content) < 26 || !bytes.HasPrefix(content, []byte("BM")) {
		return false
	}
	fileSize := binary.LittleEndian.Uint32(content[2:6])
	pixelOffset := binary.LittleEndian.Uint32(content[10:14])
	dibSize := binary.LittleEndian.Uint32(content[14:18])
	if fileSize != 0 && fileSize < 26 || uint64(pixelOffset) < uint64(14)+uint64(dibSize) || fileSize != 0 && pixelOffset >= fileSize {
		return false
	}
	var planes, bitsPerPixel uint16
	switch {
	case dibSize == 12:
		planes, bitsPerPixel = binary.LittleEndian.Uint16(content[22:24]), binary.LittleEndian.Uint16(content[24:26])
	case dibSize >= 40 && dibSize <= 124 && len(content) >= 30:
		planes, bitsPerPixel = binary.LittleEndian.Uint16(content[26:28]), binary.LittleEndian.Uint16(content[28:30])
	default:
		return false
	}
	return planes == 1 && (bitsPerPixel == 1 || bitsPerPixel == 4 || bitsPerPixel == 8 || bitsPerPixel == 16 || bitsPerPixel == 24 || bitsPerPixel == 32)
}

type localBashOperations struct{}

func (localBashOperations) Exec(context.Context, string, string, BashExecOptions) (BashExecResult, error) {
	return BashExecResult{}, notImplemented("BashOperations.Exec")
}

func CreateLocalBashOperations(...BashToolOptions) BashOperations { return localBashOperations{} }

func WithFileMutationQueue[T any](ctx context.Context, filePath string, fn func(context.Context) (T, error)) (T, error) {
	_, _, _ = ctx, filePath, fn
	var zero T
	return zero, notImplemented("WithFileMutationQueue")
}

func SortedToolNames(definitions []ToolDefinition) []string {
	names := make([]string, len(definitions))
	for i := range definitions {
		names[i] = definitions[i].Name
	}
	sort.Strings(names)
	return names
}
