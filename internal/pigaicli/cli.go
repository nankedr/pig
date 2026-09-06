package pigaicli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/internal/statepath"
)

const commandHelp = `Usage: pig-ai <command> [provider]

Commands:
  login [provider]  Login to an OAuth provider
  list              List available providers

Options:
  --auth-path <file>  Use an explicit credential file

Providers:
  anthropic            Anthropic
  github-copilot       GitHub Copilot
  kimi-coding          Kimi For Coding
  openai-codex         OpenAI Codex
  openrouter           OpenRouter
  radius               Radius
  xai                  xAI
`

// ErrInvalidArgument identifies an invocation that cannot be dispatched.
var ErrInvalidArgument = errors.New("invalid argument")

// ArgumentError describes a deterministic command-line parameter error.
type ArgumentError struct {
	Message string
}

func (e *ArgumentError) Error() string {
	if e == nil {
		return ""
	}
	return "Error: " + e.Message
}

func (*ArgumentError) Unwrap() error {
	return ErrInvalidArgument
}

// Run dispatches help and retains explicit M11 failures for credential commands.
func Run(args []string, stdout, stderr io.Writer) error {
	args, authPath, err := parseAuthPath(args)
	if err != nil {
		return err
	}
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		_, err := fmt.Fprint(stdout, commandHelp)
		return err
	}
	if args[0] == "list" || args[0] == "login" {
		if _, err := ResolveAuthPath(authPath); err != nil {
			return err
		}
	}
	if args[0] == "list" {
		return &ai.NotImplementedError{Module: "ai", Operation: "CLI.List"}
	}
	if args[0] == "login" {
		if len(args) >= 2 {
			if !isCommandProvider(args[1]) {
				return &ArgumentError{Message: "Unknown provider: " + args[1]}
			}
		}
		return &ai.NotImplementedError{Module: "ai", Operation: "CLI.Login"}
	}
	return &ArgumentError{Message: "Unknown command: " + args[0]}
}

func isCommandProvider(id string) bool {
	for _, provider := range ai.BuiltinProviders() {
		if string(provider.ID()) == id && provider.Auth().OAuth != nil {
			return true
		}
	}
	return false
}

func parseAuthPath(args []string) ([]string, string, error) {
	authPath := ""
	remaining := make([]string, 0, len(args))
	found := false
	for i := 0; i < len(args); i++ {
		if args[i] != "--auth-path" {
			remaining = append(remaining, args[i])
			continue
		}
		if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			if found {
				return nil, "", &ArgumentError{Message: "--auth-path may only be specified once"}
			}
			found = true
			authPath = args[i+1]
			i++
			continue
		}
		return nil, "", &ArgumentError{Message: "--auth-path requires a value"}
	}
	return remaining, authPath, nil
}

func ResolveAuthPath(path string) (string, error) { return statepath.AuthPath(path) }
