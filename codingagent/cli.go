package codingagent

import (
	"context"
	"fmt"
	"regexp"
	"strings"
)

const pigHelp = `pig - AI coding assistant

Usage:
  pig [options] [@files...] [messages...]

Commands:
  pig install <source> [-l]     Install extension source and add to settings
  pig remove <source> [-l]      Remove extension source from settings
  pig uninstall <source> [-l]   Alias for remove
  pig update [source|self|pig]  Update pig, extensions, or model catalogs
  pig list                      List installed extensions from settings
  pig config [-l]               Open TUI to enable/disable package resources (Tab switches scope)
  pig auth <command>            Print credentials or check provider readiness
  pig <command> --help          Show help for install/remove/uninstall/update/list/config/auth

Options:
  --provider <name>              Provider name (Headless text currently requires deepseek)
  --model <id>                   Exact model ID (required for Headless text)
  --api-key <key>                Explicit API key (overrides DEEPSEEK_API_KEY)
  --system-prompt <text>         System prompt (default: coding assistant prompt)
  --append-system-prompt <text>  Append text or file contents to the system prompt (can be used multiple times)
  --mode <mode>                  Output mode: text (default), json, or rpc; see availability below
  --print, -p                    Run the Headless text mode, process prompts, and exit
  --continue, -c                 Continue previous session
  --resume, -r                   Select a session to resume
  --session <path|id>            Use specific session file or partial UUID
  --session-id <id>              Use exact project session ID, creating it if missing
  --fork <path|id>               Fork specific session file or partial UUID into a new session
  --session-dir <dir>            Directory for session storage and lookup
  --no-session                   Don't save session (ephemeral)
  --name, -n <name>              Set session display name
  --models <patterns>            Comma-separated model patterns for Ctrl+P cycling
                                 Supports globs (anthropic/*, *sonnet*) and fuzzy matching
  --no-tools, -nt                Disable all tools by default (built-in and extension)
  --no-builtin-tools, -nbt       Disable built-in tools by default but keep extension/custom tools enabled
  --tools, -t <tools>            Comma-separated allowlist of tool names to enable
                                 Applies to built-in, extension, and custom tools
  --exclude-tools, -xt <tools>   Comma-separated denylist of tool names to disable
                                 Applies to built-in, extension, and custom tools
  --thinking <level>             Set thinking level: off, minimal, low, medium, high, xhigh, max
  --extension, -e <path>         Load an extension file (can be used multiple times)
  --no-extensions, -ne           Disable extension discovery (explicit -e paths still work)
  --skill <path>                 Load a skill file or directory (can be used multiple times)
  --no-skills, -ns               Disable skills discovery and loading
  --prompt-template <path>       Load a prompt template file or directory (can be used multiple times)
  --no-prompt-templates, -np     Disable prompt template discovery and loading
  --theme <path>                 Load a theme file or directory (can be used multiple times)
  --no-themes                    Disable theme discovery and loading
  --no-context-files, -nc        Disable AGENTS.md and CLAUDE.md discovery and loading
  --export <file>                Export session file to HTML and exit
  --list-models [search]         List available models (with optional fuzzy search)
  --verbose                      Force verbose startup (overrides quietStartup setting)
  --tui-mode <mode>              TUI mode: regular (default) or fullscreen
  --approve, -a                  Trust project-local files for this run
  --no-approve, -na              Ignore project-local files for this run
  --offline                      Disable startup and control-plane network operations (same as PIG_OFFLINE=1)
  --help, -h                     Show this help
  --version, -v                  Show version number

Modes:
  interactive  text mode with terminal stdin and stdout; not implemented
  print        --print or non-terminal stdin/stdout; implemented with an in-memory Session
  json         --mode json; not implemented (reserved for newline-delimited events)
  rpc          --mode rpc; not implemented (reserved for a local JSONL subprocess interface)

Current Headless Text Availability:
  Requires --provider deepseek and an exact --model ID. Credentials come from
  --api-key or DEEPSEEK_API_KEY. The read Tool is available; other built-in
  Tools, persisted Sessions, resources, extensions, JSON, and RPC remain
  explicit Capability Stubs. Successful output contains only final Assistant
  text. SIGINT cancels the run and exits 130.

Extension flags:
  Unknown long flags are reserved for the Extension Surface. Until the extension
  runtime milestone, using one returns a structured not-implemented error.

Examples:
  # Print a provider API key for an external client
  pig auth print-api-key --provider openai

  # Print an OAuth bearer token for an external client (refreshes if expired)
  pig auth print-bearer-token --provider openai-codex

  # Run Headless text with the standard DeepSeek environment credential
  export DEEPSEEK_API_KEY=...
  pig --provider deepseek --model deepseek-v4-flash -p "Explain this package"

  # Supply the prompt on stdin (non-terminal I/O selects Headless text)
  printf 'Summarize go.mod' | pig --provider deepseek --model deepseek-v4-flash

  # Allow only the implemented read Tool
  pig --provider deepseek --model deepseek-v4-flash --tools read -p "Read go.mod"

  # Disable Tools for a text-only request
  pig --provider deepseek --model deepseek-v4-flash --no-tools -p "Say hello"

Environment Variables:
  ANTHROPIC_AUTH_TOKEN             - Anthropic bearer auth token
  ANTHROPIC_API_KEY                - Anthropic Claude API key
  ANTHROPIC_OAUTH_TOKEN            - Anthropic OAuth token (alternative to API key)
  ANT_LING_API_KEY                 - Ant Ling API key
  OPENAI_API_KEY                   - OpenAI GPT API key
  AZURE_OPENAI_API_KEY             - Azure OpenAI API key
  AZURE_OPENAI_BASE_URL            - Azure OpenAI/Cognitive Services base URL (e.g. https://{resource}.openai.azure.com)
  AZURE_OPENAI_RESOURCE_NAME       - Azure OpenAI resource name (alternative to base URL)
  AZURE_OPENAI_API_VERSION         - Azure OpenAI API version (default: v1)
  AZURE_OPENAI_DEPLOYMENT_NAME_MAP - Azure OpenAI model=deployment map (comma-separated)
  DEEPSEEK_API_KEY                 - DeepSeek API key
  NVIDIA_API_KEY                   - NVIDIA NIM API key
  GEMINI_API_KEY                   - Google Gemini API key
  GROQ_API_KEY                     - Groq API key
  CEREBRAS_API_KEY                 - Cerebras API key
  XAI_API_KEY                      - xAI Grok API key
  FIREWORKS_API_KEY                - Fireworks API key
  TOGETHER_API_KEY                 - Together AI API key
  BASETEN_API_KEY                  - Baseten API key
  OPENROUTER_API_KEY               - OpenRouter API key
  AI_GATEWAY_API_KEY               - Vercel AI Gateway API key
  ZAI_API_KEY                      - ZAI Coding Plan API key (Global)
  ZAI_CODING_CN_API_KEY            - ZAI Coding Plan API key (China)
  MISTRAL_API_KEY                  - Mistral API key
  MINIMAX_API_KEY                  - MiniMax API key
  MOONSHOT_API_KEY                 - Moonshot AI API key
  OPENCODE_API_KEY                 - OpenCode Zen/OpenCode Go API key
  KIMI_API_KEY                     - Kimi For Coding API key
  CLOUDFLARE_API_KEY               - Cloudflare API token (Workers AI and AI Gateway)
  CLOUDFLARE_ACCOUNT_ID            - Cloudflare account id (required for both)
  CLOUDFLARE_GATEWAY_ID            - Cloudflare AI Gateway slug (required for AI Gateway)
  QWEN_TOKEN_PLAN_API_KEY          - Qwen Token Plan API key (international region)
  QWEN_TOKEN_PLAN_CN_API_KEY       - Qwen Token Plan API key (China region)
  XIAOMI_API_KEY                   - Xiaomi MiMo API key (api.xiaomimimo.com billing)
  XIAOMI_TOKEN_PLAN_CN_API_KEY     - Xiaomi MiMo Token Plan API key (China region)
  XIAOMI_TOKEN_PLAN_AMS_API_KEY    - Xiaomi MiMo Token Plan API key (Amsterdam region)
  XIAOMI_TOKEN_PLAN_SGP_API_KEY    - Xiaomi MiMo Token Plan API key (Singapore region)
  AWS_PROFILE                      - AWS profile for Amazon Bedrock
  AWS_ACCESS_KEY_ID                - AWS access key for Amazon Bedrock
  AWS_SECRET_ACCESS_KEY            - AWS secret key for Amazon Bedrock
  AWS_BEARER_TOKEN_BEDROCK         - Bedrock API key (bearer token)
  AWS_REGION                       - AWS region for Amazon Bedrock (e.g., us-east-1)
  PIG_CODING_AGENT_DIR             - Config directory (default: ~/.pig/agent)
  PIG_CODING_AGENT_SESSION_DIR     - Session storage directory (overridden by --session-dir)
  PIG_PACKAGE_DIR                  - Override package directory (for Nix/Guix store paths)
  PIG_DEEPSEEK_BASE_URL            - DeepSeek endpoint override for deterministic local verification
  PIG_OFFLINE                      - Disable startup and control-plane network operations when set to 1/true/yes
  PIG_TELEMETRY                    - Override install telemetry when set to 1/true/yes or 0/false/no
  PIG_SHARE_VIEWER_URL             - Base URL for /share command

Built-in Tool Names:
  read   - Read file contents (implemented)
  bash   - Execute bash commands (not implemented)
  edit   - Edit files with find/replace (not implemented)
  write  - Write files (creates/overwrites; not implemented)
  grep   - Search file contents (not implemented)
  find   - Find files by glob pattern (not implemented)
  ls     - List directory contents (not implemented)

Exit Status:
  0    Static help/version or a Headless text run completed successfully
  1    Invalid arguments, Provider failure, or an unavailable capability
  130  Headless text was interrupted with SIGINT
`

const installHelp = `Usage:
  pig install <source> [-l] [--approve|--no-approve]

Install a package and add it to settings.

Options:
  -l, --local       Install project-locally (.pig/settings.json)
  -a, --approve     Trust project-local files for this command
  -na, --no-approve Ignore project-local files for this command

Examples:
  pig install npm:@foo/bar
  pig install git:github.com/user/repo
  pig install git:git@github.com:user/repo
  pig install https://github.com/user/repo
  pig install ssh://git@github.com/user/repo
  pig install ./local/path
`

const removeHelp = `Usage:
  pig remove <source> [-l] [--approve|--no-approve]

Remove a package and its source from settings.
Alias: pig uninstall <source> [-l]

Options:
  -l, --local       Remove from project settings (.pig/settings.json)
  -a, --approve     Trust project-local files for this command
  -na, --no-approve Ignore project-local files for this command

Examples:
  pig remove npm:@foo/bar
  pig uninstall npm:@foo/bar
`

const updateHelp = `Usage:
  pig update [source|self|pig] [--self|--extensions|--models|--all] [--extension <source>] [--approve|--no-approve] [--force]

Update Pig, installed packages, or model catalogs.

Options:
  --self                  Update Pig only (default when no target is given)
  --extensions            Update installed packages only
  --models                Refresh model catalogs only
  --all                   Update Pig and installed packages
  --extension <source>    Update one package only
  -a, --approve           Trust project-local files for this command
  -na, --no-approve       Ignore project-local files for this command
  --force                 Reinstall Pig even if the current version is latest

Short forms:
  pig update                Update Pig only
  pig update --all          Update Pig and all extensions
  pig update --models       Refresh model catalogs only
  pig update <source>       Update one package
  pig update pig            Update Pig only (self works as alias to pig)
`

const listHelp = `Usage:
  pig list [--approve|--no-approve]

List installed packages from user and project settings.

Options:
  -a, --approve      Trust project-local files for this command
  -na, --no-approve  Ignore project-local files for this command
`

const configHelp = `Usage:
  pig config [-l] [--approve|--no-approve]

Open the resource configuration TUI to enable or disable package resources.
Without -l, starts in global settings (~/.pig/agent/settings.json).
Press Tab in the TUI to switch between global and project-local modes.

Options:
  -l, --local       Edit project overrides (.pig/settings.json)
  -a, --approve     Trust project-local files for this command with -l
  -na, --no-approve Ignore project-local files for this command with -l
`

const authHelp = `Usage:
  pig auth print-api-key [--provider <provider>] [--model <model>]
  pig auth print-bearer-token [--provider <provider>] [--model <model>] [--min-expiry <duration>]
  pig auth check [--provider <provider>] [--model <model>] [--json] [--credentials] [--no-refresh]

Auth commands require at least one of --provider or --model. Checks refresh expired OAuth credentials by default; --no-refresh prevents this. --credentials emits the credential, or includes it in JSON output.
`

// CLIContractMode describes one effective product mode and how it is selected.
type CLIContractMode struct {
	Name      string
	Selection string
	Operation string
}

// CLIContract is the static, side-effect-free product entry contract.
type CLIContract struct {
	Modes      []CLIContractMode
	ExitStatus string
}

// StaticCLIContract returns an independently owned snapshot of the effective
// modes and exit semantics exposed by this milestone.
func StaticCLIContract() CLIContract {
	return CLIContract{
		Modes: []CLIContractMode{
			{Name: "interactive", Selection: "text mode with terminal stdin and stdout", Operation: "mode.interactive"},
			{Name: "print", Selection: "--print or non-terminal stdin/stdout", Operation: "mode.print.text"},
			{Name: "json", Selection: "--mode json", Operation: "mode.json"},
			{Name: "rpc", Selection: "--mode rpc", Operation: "mode.rpc"},
		},
		ExitStatus: "0 for static help/version or successful Headless text; 1 for argument, Provider, and unavailable-capability errors; 130 for interrupted Headless text",
	}
}

// CLIInvocation contains the process facts needed to select a product mode.
// It carries no ambient filesystem, environment, or network capability.
type CLIInvocation struct {
	Arguments        []string
	StdinIsTerminal  bool
	StdoutIsTerminal bool
}

// CLIResult carries static stdout and nonfatal diagnostics. Runtime operations
// return a structured capability error until their milestone.
type CLIResult struct {
	Stderr string
	Stdout string
}

// CLIArgumentError identifies invalid command-line input without classifying
// it as an unavailable capability.
type CLIArgumentError struct {
	Message string
}

func (e *CLIArgumentError) Error() string {
	if e == nil {
		return ""
	}
	return "Error: " + e.Message
}

// RunCLI dispatches the product command without exiting the process.
func RunCLI(ctx context.Context, invocation CLIInvocation) (CLIResult, error) {
	if err := ctx.Err(); err != nil {
		return CLIResult{}, err
	}
	if help, ok := declaredCommandHelp(invocation.Arguments); ok {
		return CLIResult{Stdout: help}, nil
	}
	if operation, ok, err := parseDeclaredCommand(invocation.Arguments); ok {
		if err != nil {
			return CLIResult{}, err
		}
		return CLIResult{}, notImplemented(operation)
	}
	parsed := ParseArgs(invocation.Arguments)
	result := CLIResult{}
	for _, diagnostic := range parsed.Diagnostics {
		if diagnostic.Type == "error" {
			return result, &CLIArgumentError{Message: diagnostic.Message}
		}
		if diagnostic.Type == "warning" {
			result.Stderr += "Warning: " + diagnostic.Message + "\n"
		}
	}
	if parsed.Version {
		result.Stdout = Version + "\n"
		return result, nil
	}
	if parsed.Export != nil {
		return result, notImplemented("session.export")
	}
	if err := validateRootConstraints(parsed); err != nil {
		return result, err
	}
	if parsed.Help {
		result.Stdout = pigHelp
		return result, nil
	}
	if len(parsed.Extensions) != 0 {
		return result, notImplemented("extension.discovery")
	}
	if len(parsed.UnknownFlags) != 0 {
		return result, notImplemented("extension.flag." + firstUnknownFlag(invocation.Arguments, parsed.UnknownFlags))
	}
	if parsed.ListModels.IsSet() {
		return result, notImplemented("models.list")
	}
	switch {
	case parsed.Mode == ModeRPC:
		return result, notImplemented("mode.rpc")
	case parsed.Mode == ModeJSON:
		return result, notImplemented("mode.json")
	case parsed.Print || !invocation.StdinIsTerminal || !invocation.StdoutIsTerminal:
		return result, notImplemented("mode.print.text")
	default:
		return result, notImplemented("mode.interactive")
	}
}

func declaredCommandHelp(arguments []string) (string, bool) {
	if len(arguments) == 0 {
		return "", false
	}
	hasHelp := false
	for _, argument := range arguments[1:] {
		if argument == "--help" || argument == "-h" {
			hasHelp = true
		}
	}
	switch arguments[0] {
	case "install":
		return installHelp, hasHelp
	case "remove", "uninstall":
		return removeHelp, hasHelp
	case "update":
		return updateHelp, hasHelp
	case "list":
		return listHelp, hasHelp
	case "config":
		return configHelp, hasHelp
	case "auth":
		return authHelp, len(arguments) == 1 || arguments[1] == "help" || hasHelp
	default:
		return "", false
	}
}

func parseDeclaredCommand(arguments []string) (string, bool, error) {
	if len(arguments) == 0 {
		return "", false, nil
	}
	switch arguments[0] {
	case "install", "remove", "uninstall", "update", "list":
		operation, err := parsePackageCommand(arguments)
		return operation, true, err
	case "config":
		return "command.config", true, validateConfigCommand(arguments[1:])
	case "auth":
		operation, err := parseAuthCommand(arguments)
		return operation, true, err
	default:
		return "", false, nil
	}
}

func parsePackageCommand(arguments []string) (string, error) {
	command := arguments[0]
	operationCommand := command
	if operationCommand == "uninstall" {
		operationCommand = "remove"
	}
	operation := "command." + operationCommand
	rest := arguments[1:]
	var source, invalidOption, invalidArgument, missingValue, conflict string
	var selfFlag, extensionsFlag, modelsFlag, allFlag bool
	var extensionSource string
	for index := 0; index < len(rest); index++ {
		arg := rest[index]
		switch arg {
		case "-l", "--local":
			if command != "install" && command != "remove" && command != "uninstall" {
				setFirst(&invalidOption, arg)
			}
		case "--self":
			if command != "update" {
				setFirst(&invalidOption, arg)
			} else {
				selfFlag = true
			}
		case "--extensions":
			if command != "update" {
				setFirst(&invalidOption, arg)
			} else {
				extensionsFlag = true
			}
		case "--models":
			if command != "update" {
				setFirst(&invalidOption, arg)
			} else {
				modelsFlag = true
			}
		case "--all":
			if command != "update" {
				setFirst(&invalidOption, arg)
			} else {
				allFlag = true
			}
		case "--force":
			if command != "update" {
				setFirst(&invalidOption, arg)
			}
		case "-a", "--approve", "-na", "--no-approve":
		case "--extension":
			if command != "update" {
				setFirst(&invalidOption, arg)
			} else if index+1 >= len(rest) || strings.HasPrefix(rest[index+1], "-") {
				setFirst(&missingValue, arg)
			} else {
				index++
				if extensionSource != "" {
					setFirst(&conflict, "--extension can only be provided once")
				} else {
					extensionSource = rest[index]
				}
			}
		default:
			if strings.HasPrefix(arg, "-") {
				setFirst(&invalidOption, arg)
			} else if command != "list" && source == "" {
				source = arg
			} else {
				setFirst(&invalidArgument, arg)
			}
		}
	}
	if command == "update" {
		if allFlag && (selfFlag || extensionsFlag || modelsFlag || extensionSource != "") {
			setFirst(&conflict, "--all cannot be combined with --self, --extensions, --models, or --extension")
		}
		if allFlag && source != "" {
			setFirst(&conflict, "--all cannot be combined with a positional source")
		}
		if modelsFlag && (selfFlag || extensionsFlag || allFlag || extensionSource != "") {
			setFirst(&conflict, "--models cannot be combined with --self, --extensions, --all, or --extension")
		}
		if modelsFlag && source != "" {
			setFirst(&conflict, "--models cannot be combined with a positional source")
		}
		if extensionSource != "" && (selfFlag || extensionsFlag || allFlag) {
			setFirst(&conflict, "--extension cannot be combined with --self, --extensions, or --all")
		}
		if extensionSource != "" && source != "" {
			setFirst(&conflict, "--extension cannot be combined with a positional source")
		}
		if source != "" && source != "self" && source != "pig" && (extensionsFlag || selfFlag || allFlag) {
			setFirst(&conflict, "positional update targets cannot be combined with --self, --extensions, or --all")
		}
	}
	switch {
	case invalidOption != "":
		return "", argumentErrorf("Unknown option %s for %q.", invalidOption, operationCommand)
	case missingValue != "":
		return "", argumentErrorf("Missing value for %s.", missingValue)
	case invalidArgument != "":
		return "", argumentErrorf("Unexpected argument %s.", invalidArgument)
	case conflict != "":
		return "", &CLIArgumentError{Message: conflict}
	case (command == "install" || command == "remove" || command == "uninstall") && source == "":
		return "", argumentErrorf("Missing %s source.", operationCommand)
	}
	return operation, nil
}

func validateConfigCommand(arguments []string) error {
	for _, argument := range arguments {
		switch argument {
		case "-l", "--local", "-a", "--approve", "-na", "--no-approve":
		default:
			if strings.HasPrefix(argument, "-") {
				return argumentErrorf("Unknown option %s for %q.", argument, "config")
			}
			return argumentErrorf("Unexpected argument %s.", argument)
		}
	}
	return nil
}

var authDuration = regexp.MustCompile(`(?i)^[0-9]+(?:ms|s|m|h)$`)

func parseAuthCommand(arguments []string) (string, error) {
	if len(arguments) < 2 {
		return "", nil
	}
	kind := arguments[1]
	operation := ""
	switch kind {
	case "check":
		operation = "command.auth.check"
	case "print-api-key":
		operation = "command.auth.print-api-key"
	case "print-bearer-token":
		operation = "command.auth.print-bearer-token"
	default:
		return "", argumentErrorf(`Unknown auth command %q. Use "pig auth print-api-key", "pig auth print-bearer-token", or "pig auth check".`, kind)
	}
	var parserArgs []string
	for index := 2; index < len(arguments); index++ {
		argument := arguments[index]
		switch argument {
		case "--json", "--credentials", "--no-refresh":
			if kind != "check" {
				return "", argumentErrorf("%s is only supported by auth check", argument)
			}
		case "--min-expiry":
			if kind != "print-bearer-token" {
				return "", argumentErrorf("--min-expiry is only supported by print-bearer-token")
			}
			if index+1 >= len(arguments) || !authDuration.MatchString(arguments[index+1]) {
				return "", argumentErrorf("--min-expiry must use a duration such as 30m or 1h")
			}
			index++
		default:
			parserArgs = append(parserArgs, argument)
		}
	}
	parsed := ParseArgs(parserArgs)
	for _, diagnostic := range parsed.Diagnostics {
		if diagnostic.Type == "error" {
			return "", &CLIArgumentError{Message: diagnostic.Message}
		}
	}
	if len(parsed.UnknownFlags) > 0 {
		return "", argumentErrorf(`Unknown option --%s for "auth %s".`, firstUnknownFlag(parserArgs, parsed.UnknownFlags), kind)
	}
	if parsed.APIKey != nil || len(parsed.Messages) > 0 || len(parsed.FileArgs) > 0 || hasDisallowedAuthArgs(parsed) {
		return "", argumentErrorf("Auth commands only accept --provider and --model")
	}
	provider := parsed.Provider != nil && strings.TrimSpace(*parsed.Provider) != ""
	model := parsed.Model != nil && strings.TrimSpace(*parsed.Model) != ""
	if !provider && !model {
		if kind == "check" {
			return "", argumentErrorf("Auth checks require --provider <provider> or --model <model>")
		}
		return "", argumentErrorf("Credential printing requires --provider <provider> or --model <model>")
	}
	return operation, nil
}

func hasDisallowedAuthArgs(parsed Args) bool {
	return parsed.Help || parsed.Version || parsed.Continue || parsed.Resume || parsed.NoSession || parsed.NoTools ||
		parsed.NoBuiltinTools || parsed.NoExtensions || parsed.NoSkills || parsed.NoPromptTemplates || parsed.NoThemes ||
		parsed.NoContextFiles || parsed.Verbose || parsed.Offline || parsed.Print || parsed.ListModels.IsSet() ||
		parsed.Mode != "" || parsed.SystemPrompt != nil || len(parsed.AppendSystemPrompt) > 0 || parsed.Name != nil ||
		parsed.Session != nil || parsed.SessionID != nil || parsed.Fork != nil || parsed.SessionDir != nil || parsed.Export != nil ||
		len(parsed.Extensions) > 0 || len(parsed.Skills) > 0 || len(parsed.PromptTemplates) > 0 || len(parsed.Themes) > 0 ||
		len(parsed.Models) > 0 || len(parsed.Tools) > 0 || len(parsed.ExcludeTools) > 0 || parsed.Thinking != "" ||
		parsed.TUIMode != "" || parsed.ProjectTrustOverride != nil
}

var customSessionID = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9])?$`)

func validateRootConstraints(parsed Args) error {
	if parsed.Mode == ModeRPC && len(parsed.FileArgs) > 0 {
		return argumentErrorf("@file arguments are not supported in RPC mode")
	}
	if parsed.Fork != nil {
		conflicts := selectedFlags(
			selectedFlag{selected: parsed.Session != nil, name: "--session"},
			selectedFlag{selected: parsed.Continue, name: "--continue"},
			selectedFlag{selected: parsed.Resume, name: "--resume"},
			selectedFlag{selected: parsed.NoSession, name: "--no-session"},
		)
		if len(conflicts) > 0 {
			return argumentErrorf("--fork cannot be combined with %s", strings.Join(conflicts, ", "))
		}
	}
	if parsed.SessionID != nil {
		conflicts := selectedFlags(
			selectedFlag{selected: parsed.Session != nil, name: "--session"},
			selectedFlag{selected: parsed.Continue, name: "--continue"},
			selectedFlag{selected: parsed.Resume, name: "--resume"},
		)
		if len(conflicts) > 0 {
			return argumentErrorf("--session-id cannot be combined with %s", strings.Join(conflicts, ", "))
		}
		if !customSessionID.MatchString(*parsed.SessionID) {
			return argumentErrorf("Session id must be non-empty, contain only alphanumeric characters, '-', '_', and '.', and start and end with an alphanumeric character")
		}
	}
	if parsed.Name != nil && strings.TrimSpace(*parsed.Name) == "" {
		return argumentErrorf("--name requires a non-empty value")
	}
	return nil
}

type selectedFlag struct {
	selected bool
	name     string
}

func selectedFlags(values ...selectedFlag) []string {
	flags := make([]string, 0, len(values))
	for _, value := range values {
		if value.selected {
			flags = append(flags, value.name)
		}
	}
	return flags
}

func setFirst(target *string, value string) {
	if *target == "" {
		*target = value
	}
}

func argumentErrorf(format string, values ...any) *CLIArgumentError {
	return &CLIArgumentError{Message: fmt.Sprintf(format, values...)}
}

func firstUnknownFlag(arguments []string, unknown map[string]*string) string {
	for _, argument := range arguments {
		if !strings.HasPrefix(argument, "--") {
			continue
		}
		name, _, _ := strings.Cut(strings.TrimPrefix(argument, "--"), "=")
		if _, ok := unknown[name]; ok {
			return name
		}
	}
	return "unknown"
}
