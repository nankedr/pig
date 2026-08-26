package codingagent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/tui"
)

// ConfigDirName is Pig's product-owned configuration directory. It is kept
// separate from Pi state as required by the repository identity contract.
const ConfigDirName = ".pig"

// Version is the fallback version exposed by the package-level SDK scaffold.
const Version = "0.0.0"

// Mode selects one of the local Coding Agent process interfaces. Remote
// sessions use client/protocol instead and are deliberately not a Mode.
type Mode string

const (
	ModeText Mode = "text"
	ModeJSON Mode = "json"
	ModeRPC  Mode = "rpc"
)

// ArgDiagnostic records a recoverable warning or a fatal argument error.
type ArgDiagnostic struct {
	Type    string
	Message string
}

type listModelsOptionState uint8

const (
	listModelsAbsent listModelsOptionState = iota
	listModelsAll
	listModelsSearch
)

// ListModelsOption preserves Pi's optional true-or-string --list-models
// value. Its zero value represents an absent option.
type ListModelsOption struct {
	state  listModelsOptionState
	search string
}

// IsSet reports whether --list-models was provided.
func (o ListModelsOption) IsSet() bool {
	return o.state != listModelsAbsent
}

// IsAll reports whether --list-models was provided without a search string.
func (o ListModelsOption) IsAll() bool {
	return o.state == listModelsAll
}

// Search returns the search string supplied to --list-models. An explicit
// empty string is returned with ok set to true.
func (o ListModelsOption) Search() (search string, ok bool) {
	if o.state != listModelsSearch {
		return "", false
	}
	return o.search, true
}

func allListModels() ListModelsOption {
	return ListModelsOption{state: listModelsAll}
}

func searchListModels(search string) ListModelsOption {
	return ListModelsOption{state: listModelsSearch, search: search}
}

// Args is the parsed root-command argument surface. Pointer fields preserve
// absence independently from explicit false or empty values.
type Args struct {
	APIKey               *string
	AppendSystemPrompt   []string
	Continue             bool
	Diagnostics          []ArgDiagnostic
	ExcludeTools         []string
	Export               *string
	Extensions           []string
	FileArgs             []string
	Fork                 *string
	Help                 bool
	ListModels           ListModelsOption
	Messages             []string
	Mode                 Mode
	Model                *string
	Models               []string
	Name                 *string
	NoBuiltinTools       bool
	NoContextFiles       bool
	NoExtensions         bool
	NoPromptTemplates    bool
	NoSession            bool
	NoSkills             bool
	NoThemes             bool
	NoTools              bool
	Offline              bool
	Print                bool
	ProjectTrustOverride *bool
	PromptTemplates      []string
	Provider             *string
	Resume               bool
	Session              *string
	SessionDir           *string
	SessionID            *string
	Skills               []string
	SystemPrompt         *string
	Themes               []string
	Thinking             agent.ThinkingLevel
	Tools                []string
	TUIMode              tui.TUIMode
	UnknownFlags         map[string]*string
	Verbose              bool
	Version              bool
}

// ParseArgs performs only deterministic argument classification. It never
// reads configuration, credentials, the working directory, or extension code.
func ParseArgs(arguments []string) Args {
	result := Args{
		Messages:     []string{},
		FileArgs:     []string{},
		UnknownFlags: map[string]*string{},
		Diagnostics:  []ArgDiagnostic{},
	}
	value := func(index *int, flag string) (string, bool) {
		if *index+1 >= len(arguments) {
			result.Diagnostics = append(result.Diagnostics, ArgDiagnostic{Type: "error", Message: flag + " requires a value"})
			return "", false
		}
		*index = *index + 1
		return arguments[*index], true
	}
	for i := 0; i < len(arguments); i++ {
		arg := arguments[i]
		switch arg {
		case "--help", "-h":
			result.Help = true
		case "--version", "-v":
			result.Version = true
		case "--continue", "-c":
			result.Continue = true
		case "--resume", "-r":
			result.Resume = true
		case "--no-session":
			result.NoSession = true
		case "--no-tools", "-nt":
			result.NoTools = true
		case "--no-builtin-tools", "-nbt":
			result.NoBuiltinTools = true
		case "--no-extensions", "-ne":
			result.NoExtensions = true
		case "--no-skills", "-ns":
			result.NoSkills = true
		case "--no-prompt-templates", "-np":
			result.NoPromptTemplates = true
		case "--no-themes":
			result.NoThemes = true
		case "--no-context-files", "-nc":
			result.NoContextFiles = true
		case "--verbose":
			result.Verbose = true
		case "--offline":
			result.Offline = true
		case "--approve", "-a":
			approved := true
			result.ProjectTrustOverride = &approved
		case "--no-approve", "-na":
			approved := false
			result.ProjectTrustOverride = &approved
		case "--print", "-p":
			result.Print = true
			if i+1 < len(arguments) && !strings.HasPrefix(arguments[i+1], "@") && (!strings.HasPrefix(arguments[i+1], "-") || strings.HasPrefix(arguments[i+1], "---")) {
				i++
				result.Messages = append(result.Messages, arguments[i])
			}
		case "--list-models":
			if i+1 < len(arguments) && !strings.HasPrefix(arguments[i+1], "-") && !strings.HasPrefix(arguments[i+1], "@") {
				i++
				result.ListModels = searchListModels(arguments[i])
			} else {
				result.ListModels = allListModels()
			}
		case "--mode":
			if raw, ok := value(&i, arg); ok {
				switch Mode(raw) {
				case ModeText, ModeJSON, ModeRPC:
					result.Mode = Mode(raw)
				default:
					result.Diagnostics = append(result.Diagnostics, ArgDiagnostic{
						Type:    "error",
						Message: `Invalid mode "` + raw + `". Valid values: text, json, rpc`,
					})
				}
			}
		case "--tui-mode":
			mode := ""
			hasMode := i+1 < len(arguments)
			if hasMode {
				mode = arguments[i+1]
			}
			if mode == string(tui.TUIModeRegular) || mode == string(tui.TUIModeFullscreen) {
				i++
				result.TUIMode = tui.TUIMode(mode)
			} else if !hasMode || strings.HasPrefix(mode, "-") {
				result.Diagnostics = append(result.Diagnostics, ArgDiagnostic{Type: "error", Message: "--tui-mode requires regular or fullscreen"})
			} else {
				i++
				result.Diagnostics = append(result.Diagnostics, ArgDiagnostic{Type: "error", Message: `Invalid TUI mode "` + mode + `". Valid values: regular, fullscreen`})
			}
		case "--thinking":
			if raw, ok := value(&i, arg); ok {
				level := agent.ThinkingLevel(raw)
				if validThinkingLevel(level) {
					result.Thinking = level
				} else {
					result.Diagnostics = append(result.Diagnostics, ArgDiagnostic{
						Type:    "warning",
						Message: `Invalid thinking level "` + raw + `". Valid values: off, minimal, low, medium, high, xhigh, max`,
					})
				}
			}
		case "--provider", "--model", "--api-key", "--system-prompt", "--name", "-n", "--session", "--session-id", "--fork", "--session-dir", "--export":
			valueFlag := arg
			if arg == "-n" {
				valueFlag = "--name"
			}
			raw, ok := value(&i, valueFlag)
			if !ok {
				break
			}
			v := raw
			switch arg {
			case "--provider":
				result.Provider = &v
			case "--model":
				result.Model = &v
			case "--api-key":
				result.APIKey = &v
			case "--system-prompt":
				result.SystemPrompt = &v
			case "--name", "-n":
				result.Name = &v
			case "--session":
				result.Session = &v
			case "--session-id":
				result.SessionID = &v
			case "--fork":
				result.Fork = &v
			case "--session-dir":
				result.SessionDir = &v
			case "--export":
				result.Export = &v
			}
		case "--append-system-prompt", "--extension", "-e", "--skill", "--prompt-template", "--theme":
			if raw, ok := value(&i, arg); ok {
				switch arg {
				case "--append-system-prompt":
					result.AppendSystemPrompt = append(result.AppendSystemPrompt, raw)
				case "--extension", "-e":
					result.Extensions = append(result.Extensions, raw)
				case "--skill":
					result.Skills = append(result.Skills, raw)
				case "--prompt-template":
					result.PromptTemplates = append(result.PromptTemplates, raw)
				case "--theme":
					result.Themes = append(result.Themes, raw)
				}
			}
		case "--models", "--tools", "-t", "--exclude-tools", "-xt":
			if raw, ok := value(&i, arg); ok {
				values := splitCommaSeparated(raw, arg == "--models")
				switch arg {
				case "--models":
					result.Models = values
				case "--tools", "-t":
					result.Tools = values
				default:
					result.ExcludeTools = values
				}
			}
		default:
			switch {
			case strings.HasPrefix(arg, "@"):
				result.FileArgs = append(result.FileArgs, strings.TrimPrefix(arg, "@"))
			case strings.HasPrefix(arg, "--"):
				name := strings.TrimPrefix(arg, "--")
				if key, raw, ok := strings.Cut(name, "="); ok {
					result.UnknownFlags[key] = &raw
				} else if i+1 < len(arguments) && !strings.HasPrefix(arguments[i+1], "-") && !strings.HasPrefix(arguments[i+1], "@") {
					i++
					raw := arguments[i]
					result.UnknownFlags[name] = &raw
				} else {
					result.UnknownFlags[name] = nil
				}
			case strings.HasPrefix(arg, "-"):
				result.Diagnostics = append(result.Diagnostics, ArgDiagnostic{Type: "error", Message: "Unknown option: " + arg})
			default:
				result.Messages = append(result.Messages, arg)
			}
		}
	}
	return result
}

func validThinkingLevel(level agent.ThinkingLevel) bool {
	switch level {
	case "off", "minimal", "low", "medium", "high", "xhigh", "max":
		return true
	default:
		return false
	}
}

func splitCommaSeparated(value string, keepEmpty bool) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); keepEmpty || part != "" {
			result = append(result, part)
		}
	}
	return result
}

// MainOptions supplies in-process extension factories. The factories are data
// only until the extension runtime milestone.
type MainOptions struct {
	ExtensionFactories []InlineExtension
}

// InlineExtension is the opaque root-level extension factory carrier. Its
// executable ABI remains deliberately unfrozen until the extension milestone.
type InlineExtension ExtensionFactory

// Main is the package-level product entry point. MainOptions remain inert until
// the extension runtime milestone.
func Main(ctx context.Context, arguments []string, _ ...MainOptions) error {
	result, err := RunCLI(ctx, CLIInvocation{
		Arguments:        arguments,
		StdinIsTerminal:  isTerminalFile(os.Stdin),
		StdoutIsTerminal: isTerminalFile(os.Stdout),
	})
	if result.Stderr != "" {
		_, _ = fmt.Fprint(os.Stderr, result.Stderr)
	}
	if result.Stdout != "" {
		if _, writeErr := fmt.Fprint(os.Stdout, result.Stdout); writeErr != nil {
			return errors.New("Error: Failed to write stdout.")
		}
	}
	return err
}

// Package and user configuration paths are ambient filesystem capabilities.
// They fail before reading environment variables, the home directory, or disk.
func GetAgentDir() (string, error) { return "", notImplemented("GetAgentDir") }
func GetDocsPath() (string, error) { return "", notImplemented("GetDocsPath") }
func GetExamplesPath() (string, error) {
	return "", notImplemented("GetExamplesPath")
}
func GetPackageDir() (string, error) { return "", notImplemented("GetPackageDir") }
func GetReadmePath() (string, error) { return "", notImplemented("GetReadmePath") }

// ConvertedImage is the result of converting image data for terminal use.
type ConvertedImage struct {
	Data     string
	MIMEType string
}

// ImageResizeOptions records optional image constraints without selecting an
// image backend or starting a worker.
type ImageResizeOptions struct {
	MaxWidth    *int
	MaxHeight   *int
	MaxBytes    *int
	JPEGQuality *int
}

type ResizedImage struct {
	Data           string
	Height         int
	MIMEType       string
	OriginalHeight int
	OriginalWidth  int
	WasResized     bool
	Width          int
}

func (r ResizedImage) MimeType() string { return r.MIMEType }

type ShellConfig struct {
	Shell            string
	Args             []string
	CommandTransport string
}

func CopyToClipboard(string) error {
	return notImplemented("CopyToClipboard")
}

func ConvertToPNG(string, string) (*ConvertedImage, error) {
	return nil, notImplemented("ConvertToPNG")
}

func ResizeImage([]byte, string, ...ImageResizeOptions) (*ResizedImage, error) {
	return nil, notImplemented("ResizeImage")
}

// FormatDimensionNote is a pure projection and is safe before image runtime
// support. It mirrors Pi's two-decimal coordinate scaling note.
func FormatDimensionNote(result ResizedImage) *string {
	if !result.WasResized || result.Width == 0 {
		return nil
	}
	note := "[Image: original " + strconv.Itoa(result.OriginalWidth) + "x" + strconv.Itoa(result.OriginalHeight) +
		", displayed at " + strconv.Itoa(result.Width) + "x" + strconv.Itoa(result.Height) +
		fmt.Sprintf(". Multiply coordinates by %.2f to map to original image.]", float64(result.OriginalWidth)/float64(result.Width))
	return &note
}

func GetShellConfig(...string) (ShellConfig, error) {
	return ShellConfig{}, notImplemented("GetShellConfig")
}
