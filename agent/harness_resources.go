package agent

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"
)

type PromptTemplateDiagnosticCode string

const (
	PromptTemplateDiagnosticFileInfoFailed PromptTemplateDiagnosticCode = "file_info_failed"
	PromptTemplateDiagnosticListFailed     PromptTemplateDiagnosticCode = "list_failed"
	PromptTemplateDiagnosticReadFailed     PromptTemplateDiagnosticCode = "read_failed"
	PromptTemplateDiagnosticParseFailed    PromptTemplateDiagnosticCode = "parse_failed"
)

type PromptTemplateDiagnostic struct {
	Type    string
	Code    PromptTemplateDiagnosticCode
	Message string
	Path    string
}

type PromptTemplateLoadResult struct {
	PromptTemplates []PromptTemplate
	Diagnostics     []PromptTemplateDiagnostic
}

type SourcedPromptTemplate struct {
	PromptTemplate PromptTemplate
	Source         any
}

type SourcedPromptTemplateDiagnostic struct {
	PromptTemplateDiagnostic
	Source any
}

type SourcedPromptTemplateLoadResult struct {
	PromptTemplates []SourcedPromptTemplate
	Diagnostics     []SourcedPromptTemplateDiagnostic
}

func LoadPromptTemplates(context.Context, ExecutionEnv, []string) (PromptTemplateLoadResult, error) {
	return PromptTemplateLoadResult{}, newNotImplemented("LoadPromptTemplates")
}

func LoadSourcedPromptTemplates(context.Context, ExecutionEnv, []string) (SourcedPromptTemplateLoadResult, error) {
	return SourcedPromptTemplateLoadResult{}, newNotImplemented("LoadSourcedPromptTemplates")
}

func ParseCommandArgs(value string) []string {
	var args []string
	var current strings.Builder
	var quote rune
	flush := func() {
		if current.Len() > 0 {
			args = append(args, current.String())
			current.Reset()
		}
	}
	for _, character := range value {
		if quote != 0 {
			if character == quote {
				quote = 0
			} else {
				current.WriteRune(character)
			}
			continue
		}
		switch character {
		case '\'', '"':
			quote = character
		case ' ', '\t':
			flush()
		default:
			current.WriteRune(character)
		}
	}
	flush()
	return args
}

func SubstituteArgs(content string, args []string) string {
	var result strings.Builder
	for index := 0; index < len(content); {
		if content[index] != '$' {
			result.WriteByte(content[index])
			index++
			continue
		}
		switch {
		case strings.HasPrefix(content[index:], "$ARGUMENTS"):
			result.WriteString(strings.Join(args, " "))
			index += len("$ARGUMENTS")
		case strings.HasPrefix(content[index:], "$@"):
			result.WriteString(strings.Join(args, " "))
			index += 2
		case strings.HasPrefix(content[index:], "${@:"):
			end := strings.IndexByte(content[index:], '}')
			if end < 0 {
				result.WriteByte(content[index])
				index++
				continue
			}
			parts := strings.Split(content[index+4:index+end], ":")
			start, err := strconv.Atoi(parts[0])
			if err != nil {
				result.WriteString(content[index : index+end+1])
				index += end + 1
				continue
			}
			start--
			if start < 0 {
				start = 0
			}
			finish := len(args)
			if len(parts) == 2 {
				if length, err := strconv.Atoi(parts[1]); err == nil && start+length < finish {
					finish = start + length
				}
			}
			if start < len(args) {
				result.WriteString(strings.Join(args[start:finish], " "))
			}
			index += end + 1
		case index+1 < len(content) && content[index+1] >= '0' && content[index+1] <= '9':
			end := index + 1
			for end < len(content) && content[end] >= '0' && content[end] <= '9' {
				end++
			}
			position, _ := strconv.Atoi(content[index+1 : end])
			if position > 0 && position <= len(args) {
				result.WriteString(args[position-1])
			}
			index = end
		default:
			result.WriteByte('$')
			index++
		}
	}
	return result.String()
}

func FormatPromptTemplateInvocation(template PromptTemplate, args []string) string {
	return SubstituteArgs(template.Content, args)
}

type SkillDiagnosticCode string

const (
	SkillDiagnosticFileInfoFailed  SkillDiagnosticCode = "file_info_failed"
	SkillDiagnosticListFailed      SkillDiagnosticCode = "list_failed"
	SkillDiagnosticReadFailed      SkillDiagnosticCode = "read_failed"
	SkillDiagnosticParseFailed     SkillDiagnosticCode = "parse_failed"
	SkillDiagnosticInvalidMetadata SkillDiagnosticCode = "invalid_metadata"
)

type SkillDiagnostic struct {
	Type    string
	Code    SkillDiagnosticCode
	Message string
	Path    string
}

type SkillLoadResult struct {
	Skills      []Skill
	Diagnostics []SkillDiagnostic
}

type SourcedSkill struct {
	Skill  Skill
	Source any
}

type SourcedSkillDiagnostic struct {
	SkillDiagnostic
	Source any
}

type SourcedSkillLoadResult struct {
	Skills      []SourcedSkill
	Diagnostics []SourcedSkillDiagnostic
}

func LoadSkills(context.Context, ExecutionEnv, []string) (SkillLoadResult, error) {
	return SkillLoadResult{}, newNotImplemented("LoadSkills")
}

func LoadSourcedSkills(context.Context, ExecutionEnv, []string) (SourcedSkillLoadResult, error) {
	return SourcedSkillLoadResult{}, newNotImplemented("LoadSourcedSkills")
}

func FormatSkillInvocation(skill Skill, additionalInstructions string) string {
	block := fmt.Sprintf("<skill name=\"%s\" location=\"%s\">\nReferences are relative to %s.\n\n%s\n</skill>", skill.Name, skill.FilePath, filepath.Dir(skill.FilePath), skill.Content)
	if additionalInstructions == "" {
		return block
	}
	return block + "\n\n" + additionalInstructions
}

func FormatSkillsForSystemPrompt(skills []Skill) string {
	lines := []string{
		"The following skills provide specialized instructions for specific tasks.",
		"Read the full skill file when the task matches its description.",
		"When a skill file references a relative path, resolve it against the skill directory (parent of SKILL.md / dirname of the path) and use that absolute path in tool commands.",
		"", "<available_skills>",
	}
	visible := 0
	for _, skill := range skills {
		if skill.DisableModelInvocation {
			continue
		}
		visible++
		lines = append(lines, "  <skill>", "    <name>"+escapeXML(skill.Name)+"</name>", "    <description>"+escapeXML(skill.Description)+"</description>", "    <location>"+escapeXML(skill.FilePath)+"</location>", "  </skill>")
	}
	if visible == 0 {
		return ""
	}
	return strings.Join(append(lines, "</available_skills>"), "\n")
}

func escapeXML(value string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;").Replace(value)
}

const (
	DefaultMaxLines   = 2000
	DefaultMaxBytes   = 50 * 1024
	GrepMaxLineLength = 500
)

type TruncationLimit string

const (
	TruncationByLines TruncationLimit = "lines"
	TruncationByBytes TruncationLimit = "bytes"
)

type TruncationResult struct {
	Content               string
	Truncated             bool
	TruncatedBy           TruncationLimit
	TotalLines            int
	TotalBytes            int
	OutputLines           int
	OutputBytes           int
	LastLinePartial       bool
	FirstLineExceedsLimit bool
	MaxLines              int
	MaxBytes              int
}

type TruncationOptions struct {
	MaxLines int
	MaxBytes int
}

func normalizeTruncationOptions(options TruncationOptions) TruncationOptions {
	if options.MaxLines == 0 {
		options.MaxLines = DefaultMaxLines
	}
	if options.MaxBytes == 0 {
		options.MaxBytes = DefaultMaxBytes
	}
	return options
}

func splitCountedLines(content string) []string {
	if content == "" {
		return nil
	}
	lines := strings.Split(content, "\n")
	if strings.HasSuffix(content, "\n") {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func TruncateHead(content string, options TruncationOptions) TruncationResult {
	options = normalizeTruncationOptions(options)
	lines := splitCountedLines(content)
	result := TruncationResult{Content: content, TotalLines: len(lines), TotalBytes: len([]byte(content)), OutputLines: len(lines), OutputBytes: len([]byte(content)), MaxLines: options.MaxLines, MaxBytes: options.MaxBytes}
	if result.TotalLines <= options.MaxLines && result.TotalBytes <= options.MaxBytes {
		return result
	}
	result.Truncated = true
	result.TruncatedBy = TruncationByLines
	if len(lines) > 0 && len([]byte(lines[0])) > options.MaxBytes {
		result.Content, result.OutputLines, result.OutputBytes = "", 0, 0
		result.TruncatedBy, result.FirstLineExceedsLimit = TruncationByBytes, true
		return result
	}
	kept := make([]string, 0, min(len(lines), options.MaxLines))
	for _, line := range lines {
		if len(kept) >= options.MaxLines {
			break
		}
		candidate := len([]byte(line))
		if len(kept) > 0 {
			candidate++
		}
		if result.OutputBytes = len([]byte(strings.Join(kept, "\n"))); result.OutputBytes+candidate > options.MaxBytes {
			result.TruncatedBy = TruncationByBytes
			break
		}
		kept = append(kept, line)
	}
	result.Content = strings.Join(kept, "\n")
	result.OutputLines = len(kept)
	result.OutputBytes = len([]byte(result.Content))
	return result
}

func TruncateTail(content string, options TruncationOptions) TruncationResult {
	options = normalizeTruncationOptions(options)
	lines := splitCountedLines(content)
	result := TruncationResult{Content: content, TotalLines: len(lines), TotalBytes: len([]byte(content)), OutputLines: len(lines), OutputBytes: len([]byte(content)), MaxLines: options.MaxLines, MaxBytes: options.MaxBytes}
	if result.TotalLines <= options.MaxLines && result.TotalBytes <= options.MaxBytes {
		return result
	}
	result.Truncated, result.TruncatedBy = true, TruncationByLines
	kept := make([]string, 0, min(len(lines), options.MaxLines))
	for index := len(lines) - 1; index >= 0 && len(kept) < options.MaxLines; index-- {
		candidate := len([]byte(lines[index]))
		if len(kept) > 0 {
			candidate++
		}
		used := len([]byte(strings.Join(kept, "\n")))
		if used+candidate > options.MaxBytes {
			result.TruncatedBy = TruncationByBytes
			if len(kept) == 0 {
				kept = append(kept, utf8Tail(lines[index], options.MaxBytes))
				result.LastLinePartial = true
			}
			break
		}
		kept = append([]string{lines[index]}, kept...)
	}
	result.Content = strings.Join(kept, "\n")
	result.OutputLines = len(kept)
	result.OutputBytes = len([]byte(result.Content))
	return result
}

func utf8Tail(value string, maxBytes int) string {
	if maxBytes <= 0 {
		return ""
	}
	bytes := []byte(value)
	if len(bytes) <= maxBytes {
		return value
	}
	start := len(bytes) - maxBytes
	for start < len(bytes) && !utf8.RuneStart(bytes[start]) {
		start++
	}
	return string(bytes[start:])
}

type LineTruncation struct {
	Text         string
	WasTruncated bool
}

func TruncateLine(line string, maxCharacters int) LineTruncation {
	if maxCharacters == 0 {
		maxCharacters = GrepMaxLineLength
	}
	runes := []rune(line)
	if len(runes) <= maxCharacters {
		return LineTruncation{Text: line}
	}
	return LineTruncation{Text: string(runes[:maxCharacters]) + "... [truncated]", WasTruncated: true}
}

func FormatSize(bytes int64) string {
	switch {
	case bytes < 1024:
		return fmt.Sprintf("%dB", bytes)
	case bytes < 1024*1024:
		return fmt.Sprintf("%.1fKB", float64(bytes)/1024)
	default:
		return fmt.Sprintf("%.1fMB", float64(bytes)/(1024*1024))
	}
}

type ShellCaptureProgress struct {
	Output         string
	Truncation     TruncationResult
	FullOutputPath string
	LastLineBytes  int
}

type ShellCaptureOptions struct {
	ShellExecOptions
	OnChunk               func(string, func() ShellCaptureProgress)
	ReturnExecutionErrors bool
}

type ShellCaptureResult struct {
	ShellCaptureProgress
	ExitCode       int
	ExitCodeSet    bool
	Cancelled      bool
	Truncated      bool
	ExecutionError *ExecutionError
}

func SanitizeBinaryOutput(value string) string {
	return strings.Map(func(character rune) rune {
		if character == '\t' || character == '\n' || character == '\r' {
			return character
		}
		if character <= 0x1f || character >= 0xfff9 && character <= 0xfffb {
			return -1
		}
		return character
	}, value)
}

func ExecuteShellWithCapture(context.Context, ExecutionEnv, string, ShellCaptureOptions) (Result[ShellCaptureResult], error) {
	return Result[ShellCaptureResult]{}, newNotImplemented("ExecuteShellWithCapture")
}
