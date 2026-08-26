package pigaicli_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/internal/pigaicli"
)

const wantHelp = "Usage: pig-ai <command> [provider]\n\nCommands:\n" +
	"  login [provider]  Login to an OAuth provider\n" +
	"  list              List available providers\n\n" +
	"Options:\n" +
	"  --auth-path <file>  Use an explicit credential file\n\n" +
	"Providers:\n" +
	"  anthropic            Anthropic\n" +
	"  github-copilot       GitHub Copilot\n" +
	"  kimi-coding          Kimi For Coding\n" +
	"  openai-codex         OpenAI Codex\n" +
	"  openrouter           OpenRouter\n" +
	"  radius               Radius\n" +
	"  xai                  xAI\n"

func TestRunWithoutArgumentsPrintsPigHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := pigaicli.Run(nil, &stdout, &stderr)

	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	if got := stdout.String(); got != wantHelp {
		t.Fatalf("stdout = %q, want %q", got, wantHelp)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
}

func TestRunHelpAliasesPrintPigHelp(t *testing.T) {
	for _, alias := range []string{"help", "--help", "-h"} {
		t.Run(alias, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			err := pigaicli.Run([]string{alias}, &stdout, &stderr)

			if err != nil {
				t.Fatalf("Run() error = %v, want nil", err)
			}
			if got := stdout.String(); got != wantHelp {
				t.Fatalf("stdout = %q, want Pig help", got)
			}
			if got := stderr.String(); got != "" {
				t.Fatalf("stderr = %q, want empty", got)
			}
		})
	}
}

func TestRunHelpListsStaticOAuthProviders(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := pigaicli.Run([]string{"help"}, &stdout, &stderr)

	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	const wantProviders = "\nProviders:\n" +
		"  anthropic            Anthropic\n" +
		"  github-copilot       GitHub Copilot\n" +
		"  kimi-coding          Kimi For Coding\n" +
		"  openai-codex         OpenAI Codex\n" +
		"  openrouter           OpenRouter\n" +
		"  radius               Radius\n" +
		"  xai                  xAI\n"
	if got := stdout.String(); !bytes.Contains([]byte(got), []byte(wantProviders)) {
		t.Fatalf("stdout = %q, want static provider section %q", got, wantProviders)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
}

func TestRunHelpDocumentsExplicitAuthPath(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := pigaicli.Run([]string{"--help"}, &stdout, &stderr)

	if err != nil {
		t.Fatalf("Run() error = %v, want nil", err)
	}
	const want = "\nOptions:\n  --auth-path <file>  Use an explicit credential file\n"
	if got := stdout.String(); !bytes.Contains([]byte(got), []byte(want)) {
		t.Fatalf("stdout = %q, want option section %q", got, want)
	}
	if got := stderr.String(); got != "" {
		t.Fatalf("stderr = %q, want empty", got)
	}
}

func TestRunListReturnsStructuredCapabilityStub(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := pigaicli.Run([]string{"list"}, &stdout, &stderr)

	if !errors.Is(err, ai.ErrNotImplemented) {
		t.Fatalf("Run() error = %v, want ErrNotImplemented", err)
	}
	var capability *ai.NotImplementedError
	if !errors.As(err, &capability) {
		t.Fatalf("Run() error = %T, want *ai.NotImplementedError", err)
	}
	if capability.Module != "ai" || capability.Operation != "CLI.List" {
		t.Fatalf("NotImplementedError = %#v, want ai.CLI.List", capability)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("output = stdout %q, stderr %q; want none", stdout.String(), stderr.String())
	}
}

func TestRunLoginReturnsStructuredCapabilityStub(t *testing.T) {
	for _, args := range [][]string{{"login"}, {"login", "anthropic"}} {
		var stdout, stderr bytes.Buffer

		err := pigaicli.Run(args, &stdout, &stderr)

		if !errors.Is(err, ai.ErrNotImplemented) {
			t.Fatalf("Run(%q) error = %v, want ErrNotImplemented", args, err)
		}
		var capability *ai.NotImplementedError
		if !errors.As(err, &capability) {
			t.Fatalf("Run(%q) error = %T, want *ai.NotImplementedError", args, err)
		}
		if capability.Module != "ai" || capability.Operation != "CLI.Login" {
			t.Fatalf("NotImplementedError = %#v, want ai.CLI.Login", capability)
		}
		if stdout.Len() != 0 || stderr.Len() != 0 {
			t.Fatalf("output = stdout %q, stderr %q; want none", stdout.String(), stderr.String())
		}
	}
}

func TestRunRejectsUnknownLoginProvider(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := pigaicli.Run([]string{"login", "not-a-provider"}, &stdout, &stderr)

	if !errors.Is(err, pigaicli.ErrInvalidArgument) {
		t.Fatalf("Run() error = %v, want ErrInvalidArgument", err)
	}
	if got := err.Error(); got != "Error: Unknown provider: not-a-provider" {
		t.Fatalf("Run() error = %q, want unknown-provider error", got)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("output = stdout %q, stderr %q; want none", stdout.String(), stderr.String())
	}
}

func TestRunAcceptsAuthPathBeforeOrAfterCommand(t *testing.T) {
	for _, args := range [][]string{
		{"--auth-path", "/explicit/auth.json", "list"},
		{"list", "--auth-path", "/explicit/auth.json"},
		{"--auth-path", "/explicit/auth.json", "login", "anthropic"},
		{"login", "--auth-path", "/explicit/auth.json", "anthropic"},
		{"login", "anthropic", "--auth-path", "/explicit/auth.json"},
	} {
		var stdout, stderr bytes.Buffer

		err := pigaicli.Run(args, &stdout, &stderr)

		if !errors.Is(err, ai.ErrNotImplemented) {
			t.Fatalf("Run(%q) error = %v, want ErrNotImplemented", args, err)
		}
		if stdout.Len() != 0 || stderr.Len() != 0 {
			t.Fatalf("output = stdout %q, stderr %q; want none", stdout.String(), stderr.String())
		}
	}
}

func TestRunRejectsMissingAuthPathValue(t *testing.T) {
	for _, args := range [][]string{
		{"--auth-path"},
		{"list", "--auth-path"},
		{"login", "anthropic", "--auth-path"},
	} {
		var stdout, stderr bytes.Buffer

		err := pigaicli.Run(args, &stdout, &stderr)

		if !errors.Is(err, pigaicli.ErrInvalidArgument) {
			t.Fatalf("Run(%q) error = %v, want ErrInvalidArgument", args, err)
		}
		var argumentError *pigaicli.ArgumentError
		if !errors.As(err, &argumentError) {
			t.Fatalf("Run(%q) error = %T, want *pigaicli.ArgumentError", args, err)
		}
		if got := argumentError.Error(); got != "Error: --auth-path requires a value" {
			t.Fatalf("ArgumentError.Error() = %q", got)
		}
		if stdout.Len() != 0 || stderr.Len() != 0 {
			t.Fatalf("output = stdout %q, stderr %q; want none", stdout.String(), stderr.String())
		}
	}
}

func TestRunRejectsOptionAsAuthPathValue(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := pigaicli.Run([]string{"list", "--auth-path", "--unknown"}, &stdout, &stderr)

	if !errors.Is(err, pigaicli.ErrInvalidArgument) {
		t.Fatalf("Run() error = %v, want ErrInvalidArgument", err)
	}
	if got := err.Error(); got != "Error: --auth-path requires a value" {
		t.Fatalf("Run() error = %q, want missing-value error", got)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("output = stdout %q, stderr %q; want none", stdout.String(), stderr.String())
	}
}

func TestRunRejectsMoreThanOneAuthPath(t *testing.T) {
	var stdout, stderr bytes.Buffer

	err := pigaicli.Run([]string{"--auth-path", "first.json", "list", "--auth-path", "second.json"}, &stdout, &stderr)

	if !errors.Is(err, pigaicli.ErrInvalidArgument) {
		t.Fatalf("Run() error = %v, want ErrInvalidArgument", err)
	}
	var argumentError *pigaicli.ArgumentError
	if !errors.As(err, &argumentError) {
		t.Fatalf("Run() error = %T, want *pigaicli.ArgumentError", err)
	}
	if got := argumentError.Error(); got != "Error: --auth-path may only be specified once" {
		t.Fatalf("ArgumentError.Error() = %q", got)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("output = stdout %q, stderr %q; want none", stdout.String(), stderr.String())
	}
}

func TestRunRejectsUnsupportedCommands(t *testing.T) {
	for _, command := range []string{"--version", "logout", "unknown"} {
		t.Run(command, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			err := pigaicli.Run([]string{command}, &stdout, &stderr)

			if !errors.Is(err, pigaicli.ErrInvalidArgument) {
				t.Fatalf("Run() error = %v, want ErrInvalidArgument", err)
			}
			var argumentError *pigaicli.ArgumentError
			if !errors.As(err, &argumentError) {
				t.Fatalf("Run() error = %T, want *pigaicli.ArgumentError", err)
			}
			if got, want := argumentError.Error(), "Error: Unknown command: "+command; got != want {
				t.Fatalf("ArgumentError.Error() = %q, want %q", got, want)
			}
			if stdout.Len() != 0 || stderr.Len() != 0 {
				t.Fatalf("output = stdout %q, stderr %q; want none", stdout.String(), stderr.String())
			}
		})
	}
}

func TestRunPreservesPinnedTrailingArgumentBehavior(t *testing.T) {
	for _, args := range [][]string{
		{"list", "anthropic"},
		{"list", "--unknown"},
		{"login", "anthropic", "extra"},
		{"login", "anthropic", "--unknown"},
	} {
		var stdout, stderr bytes.Buffer

		err := pigaicli.Run(args, &stdout, &stderr)

		if !errors.Is(err, ai.ErrNotImplemented) {
			t.Fatalf("Run(%q) error = %v, want ErrNotImplemented", args, err)
		}
		if stdout.Len() != 0 || stderr.Len() != 0 {
			t.Fatalf("output = stdout %q, stderr %q; want none", stdout.String(), stderr.String())
		}
	}
}
