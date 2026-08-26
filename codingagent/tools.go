package codingagent

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
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

type ReadToolDetails struct{ Truncation *TruncationResult }

type ReadOperations interface {
	Access(context.Context, string) error
	DetectImageMimeType(context.Context, string) (*string, error)
	ReadFile(context.Context, string) ([]byte, error)
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
	_, _ = cwd, options
	return agent.ErasedAgentTool{}, notImplemented("CreateReadTool")
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
