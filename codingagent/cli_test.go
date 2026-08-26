package codingagent_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/nankedr/pig/codingagent"
)

func TestRunCLIRoutesProductModesToExplicitCapabilityStubs(t *testing.T) {
	tests := []struct {
		name      string
		arguments []string
		stdinTTY  bool
		stdoutTTY bool
		operation string
	}{
		{name: "interactive default", stdinTTY: true, stdoutTTY: true, operation: "mode.interactive"},
		{name: "text remains interactive on a terminal", arguments: []string{"--mode", "text"}, stdinTTY: true, stdoutTTY: true, operation: "mode.interactive"},
		{name: "print flag", arguments: []string{"--print"}, stdinTTY: true, stdoutTTY: true, operation: "mode.print.text"},
		{name: "piped stdin", stdinTTY: false, stdoutTTY: true, operation: "mode.print.text"},
		{name: "redirected stdout", stdinTTY: true, stdoutTTY: false, operation: "mode.print.text"},
		{name: "json", arguments: []string{"--mode", "json"}, stdinTTY: true, stdoutTTY: true, operation: "mode.json"},
		{name: "rpc", arguments: []string{"--mode", "rpc"}, stdinTTY: true, stdoutTTY: true, operation: "mode.rpc"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := codingagent.RunCLI(context.Background(), codingagent.CLIInvocation{
				Arguments:        test.arguments,
				StdinIsTerminal:  test.stdinTTY,
				StdoutIsTerminal: test.stdoutTTY,
			})
			if result != (codingagent.CLIResult{}) {
				t.Fatalf("RunCLI result = %#v, want zero result", result)
			}
			if !errors.Is(err, codingagent.ErrNotImplemented) {
				t.Fatalf("RunCLI error = %v, want ErrNotImplemented", err)
			}
			var unavailable *codingagent.NotImplementedError
			if !errors.As(err, &unavailable) {
				t.Fatalf("RunCLI error = %T, want *NotImplementedError", err)
			}
			if unavailable.Module != "codingagent" || unavailable.Operation != test.operation {
				t.Fatalf("NotImplementedError = %#v, want codingagent.%s", unavailable, test.operation)
			}
		})
	}
}

func TestRunCLIPropagatesCancellationBeforeRuntimeDispatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	for _, arguments := range [][]string{
		nil,
		{"install", "npm:@example/pkg"},
		{"--extension", "extension.ts"},
	} {
		result, err := codingagent.RunCLI(ctx, codingagent.CLIInvocation{
			Arguments:        arguments,
			StdinIsTerminal:  true,
			StdoutIsTerminal: true,
		})
		if result != (codingagent.CLIResult{}) {
			t.Fatalf("RunCLI(%q) result = %#v, want zero result", arguments, result)
		}
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("RunCLI(%q) error = %v, want context.Canceled", arguments, err)
		}
	}
}

func TestRunCLIRejectsArgumentErrorsBeforeMetadataAndRuntimeRoutes(t *testing.T) {
	result, err := codingagent.RunCLI(context.Background(), codingagent.CLIInvocation{
		Arguments:        []string{"--mode", "invalid", "--help"},
		StdinIsTerminal:  true,
		StdoutIsTerminal: true,
	})
	if result != (codingagent.CLIResult{}) {
		t.Fatalf("RunCLI result = %#v, want zero result", result)
	}
	var argumentError *codingagent.CLIArgumentError
	if !errors.As(err, &argumentError) {
		t.Fatalf("RunCLI error = %T (%v), want *CLIArgumentError", err, err)
	}
	want := `Error: Invalid mode "invalid". Valid values: text, json, rpc`
	if err.Error() != want {
		t.Fatalf("RunCLI error = %q, want %q", err, want)
	}
	if errors.Is(err, codingagent.ErrNotImplemented) {
		t.Fatalf("argument error %v unexpectedly matches ErrNotImplemented", err)
	}
}

func TestRunCLIReportsInvalidThinkingAsAWarningAndContinues(t *testing.T) {
	result, err := codingagent.RunCLI(context.Background(), codingagent.CLIInvocation{
		Arguments: []string{"--thinking", "invalid"},
	})
	wantStderr := "Warning: Invalid thinking level \"invalid\". Valid values: off, minimal, low, medium, high, xhigh, max\n"
	if result.Stderr != wantStderr {
		t.Fatalf("RunCLI stderr = %q, want %q", result.Stderr, wantStderr)
	}
	var unavailable *codingagent.NotImplementedError
	if !errors.As(err, &unavailable) {
		t.Fatalf("RunCLI error = %T (%v), want *NotImplementedError", err, err)
	}
	if unavailable.Module != "codingagent" || unavailable.Operation != "mode.print.text" {
		t.Fatalf("NotImplementedError = %#v, want codingagent.mode.print.text", unavailable)
	}
}

func TestRunCLIRoutesDeclaredCommandsAndExtensionInputs(t *testing.T) {
	tests := []struct {
		name      string
		arguments []string
		operation string
	}{
		{name: "install", arguments: []string{"install", "npm:@example/pkg"}, operation: "command.install"},
		{name: "remove", arguments: []string{"remove", "npm:@example/pkg"}, operation: "command.remove"},
		{name: "uninstall alias", arguments: []string{"uninstall", "npm:@example/pkg"}, operation: "command.remove"},
		{name: "update", arguments: []string{"update"}, operation: "command.update"},
		{name: "list", arguments: []string{"list"}, operation: "command.list"},
		{name: "config", arguments: []string{"config"}, operation: "command.config"},
		{name: "auth check", arguments: []string{"auth", "check", "--provider", "openai"}, operation: "command.auth.check"},
		{name: "auth api key", arguments: []string{"auth", "print-api-key", "--provider", "openai"}, operation: "command.auth.print-api-key"},
		{name: "auth bearer token", arguments: []string{"auth", "print-bearer-token", "--provider", "openai"}, operation: "command.auth.print-bearer-token"},
		{name: "explicit extension", arguments: []string{"--extension", "extension.ts"}, operation: "extension.discovery"},
		{name: "unknown long flag", arguments: []string{"--plan"}, operation: "extension.flag.plan"},
		{name: "unknown long flag with value", arguments: []string{"--review=deep"}, operation: "extension.flag.review"},
		{name: "first unknown long flag wins", arguments: []string{"--zeta", "--alpha"}, operation: "extension.flag.zeta"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := codingagent.RunCLI(context.Background(), codingagent.CLIInvocation{
				Arguments:        test.arguments,
				StdinIsTerminal:  true,
				StdoutIsTerminal: true,
			})
			if result != (codingagent.CLIResult{}) {
				t.Fatalf("RunCLI result = %#v, want zero result", result)
			}
			var unavailable *codingagent.NotImplementedError
			if !errors.As(err, &unavailable) {
				t.Fatalf("RunCLI error = %T (%v), want *NotImplementedError", err, err)
			}
			if unavailable.Module != "codingagent" || unavailable.Operation != test.operation {
				t.Fatalf("NotImplementedError = %#v, want codingagent.%s", unavailable, test.operation)
			}
		})
	}
}

func TestRunCLIRendersDeclaredSubcommandHelpWithoutStartingCapabilities(t *testing.T) {
	tests := []struct {
		name      string
		arguments []string
		want      string
	}{
		{name: "install", arguments: []string{"install", "--help"}, want: "Usage:\n  pig install <source>"},
		{name: "remove", arguments: []string{"remove", "-h"}, want: "Usage:\n  pig remove <source>"},
		{name: "uninstall alias", arguments: []string{"uninstall", "--help"}, want: "Alias: pig uninstall <source>"},
		{name: "update", arguments: []string{"update", "--help"}, want: "Usage:\n  pig update [source|self|pig]"},
		{name: "list", arguments: []string{"list", "--help"}, want: "Usage:\n  pig list"},
		{name: "config", arguments: []string{"config", "--help"}, want: "Usage:\n  pig config"},
		{name: "auth default", arguments: []string{"auth"}, want: "Usage:\n  pig auth print-api-key"},
		{name: "auth help", arguments: []string{"auth", "help"}, want: "pig auth print-api-key"},
		{name: "auth nested help", arguments: []string{"auth", "check", "--help"}, want: "pig auth print-bearer-token"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := codingagent.RunCLI(context.Background(), codingagent.CLIInvocation{Arguments: test.arguments})
			if err != nil {
				t.Fatalf("RunCLI error = %v, want nil", err)
			}
			if !strings.Contains(result.Stdout, test.want) {
				t.Fatalf("stdout = %q, want it to contain %q", result.Stdout, test.want)
			}
		})
	}
}

func TestRunCLIRootHelpInventoriesTheCompleteStaticContract(t *testing.T) {
	result, err := codingagent.RunCLI(context.Background(), codingagent.CLIInvocation{Arguments: []string{"--help"}})
	if err != nil {
		t.Fatalf("RunCLI error = %v, want nil", err)
	}
	for _, section := range []string{"Commands:", "Options:", "Modes:", "Environment Variables:", "Built-in Tool Names:", "Exit Status:"} {
		if !strings.Contains(result.Stdout, section) {
			t.Errorf("root help does not contain section %q", section)
		}
	}
	for _, command := range []string{
		"pig install <source> [-l]",
		"pig remove <source> [-l]",
		"pig uninstall <source> [-l]",
		"pig update [source|self|pig]",
		"pig list",
		"pig config [-l]",
		"pig auth <command>",
	} {
		if !strings.Contains(result.Stdout, command) {
			t.Errorf("root help does not contain command %q", command)
		}
	}
	for _, option := range []string{
		"--provider <name>", "--model <pattern>", "--api-key <key>",
		"--system-prompt <text>", "--append-system-prompt <text>",
		"--mode <mode>", "--print, -p", "--continue, -c", "--resume, -r",
		"--session <path|id>", "--session-id <id>", "--fork <path|id>",
		"--session-dir <dir>", "--no-session", "--name, -n <name>",
		"--models <patterns>", "--no-tools, -nt", "--no-builtin-tools, -nbt",
		"--tools, -t <tools>", "--exclude-tools, -xt <tools>", "--thinking <level>",
		"--extension, -e <path>", "--no-extensions, -ne", "--skill <path>",
		"--no-skills, -ns", "--prompt-template <path>", "--no-prompt-templates, -np",
		"--theme <path>", "--no-themes", "--no-context-files, -nc",
		"--export <file>", "--list-models [search]", "--verbose",
		"--tui-mode <mode>", "--approve, -a", "--no-approve, -na",
		"--offline", "--help, -h", "--version, -v",
	} {
		if !strings.Contains(result.Stdout, option) {
			t.Errorf("root help does not contain option %q", option)
		}
	}
	for _, identity := range []string{"~/.pig/agent", "PIG_CODING_AGENT_DIR", "PIG_CODING_AGENT_SESSION_DIR", "PIG_PACKAGE_DIR", "PIG_OFFLINE", "PIG_TELEMETRY", "PIG_SHARE_VIEWER_URL"} {
		if !strings.Contains(result.Stdout, identity) {
			t.Errorf("root help does not contain Pig identity %q", identity)
		}
	}
	for _, line := range []string{
		"--offline                      Disable startup and control-plane network operations (same as PIG_OFFLINE=1)",
		"PIG_OFFLINE                      - Disable startup and control-plane network operations when set to 1/true/yes",
	} {
		if !strings.Contains(result.Stdout, line) {
			t.Errorf("root help does not contain exact offline contract %q", line)
		}
	}
	for _, forbidden := range []string{" PI_", "~/.pi/", "pi.dev", "radius.pi.dev", "update pi"} {
		if strings.Contains(result.Stdout, forbidden) {
			t.Errorf("root help contains forbidden Pi product identity %q", forbidden)
		}
	}
}

func TestRunCLIValidatesDeclaredCommandGrammarBeforeReturningAStub(t *testing.T) {
	tests := []struct {
		name      string
		arguments []string
		want      string
	}{
		{name: "install source required", arguments: []string{"install"}, want: "Missing install source."},
		{name: "remove source required", arguments: []string{"remove"}, want: "Missing remove source."},
		{name: "install rejects extra argument", arguments: []string{"install", "one", "two"}, want: "Unexpected argument two."},
		{name: "remove rejects update option", arguments: []string{"remove", "pkg", "--force"}, want: `Unknown option --force for "remove".`},
		{name: "list rejects positional argument", arguments: []string{"list", "extra"}, want: "Unexpected argument extra."},
		{name: "config rejects unknown option", arguments: []string{"config", "--force"}, want: `Unknown option --force for "config".`},
		{name: "update extension value required", arguments: []string{"update", "--extension"}, want: "Missing value for --extension."},
		{name: "update all conflict", arguments: []string{"update", "--all", "--models"}, want: "--all cannot be combined with --self, --extensions, --models, or --extension"},
		{name: "update positional conflict", arguments: []string{"update", "pkg", "--self"}, want: "positional update targets cannot be combined with --self, --extensions, or --all"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := codingagent.RunCLI(context.Background(), codingagent.CLIInvocation{Arguments: test.arguments})
			if result != (codingagent.CLIResult{}) {
				t.Fatalf("RunCLI result = %#v, want zero result", result)
			}
			var argumentError *codingagent.CLIArgumentError
			if !errors.As(err, &argumentError) {
				t.Fatalf("RunCLI error = %T (%v), want *CLIArgumentError", err, err)
			}
			if argumentError.Message != test.want {
				t.Fatalf("CLIArgumentError.Message = %q, want %q", argumentError.Message, test.want)
			}
		})
	}
}

func TestRunCLIValidatesAuthCommandGrammarBeforeReturningAStub(t *testing.T) {
	tests := []struct {
		name      string
		arguments []string
		want      string
	}{
		{name: "unknown command", arguments: []string{"auth", "login"}, want: `Unknown auth command "login". Use "pig auth print-api-key", "pig auth print-bearer-token", or "pig auth check".`},
		{name: "check requires selector", arguments: []string{"auth", "check"}, want: "Auth checks require --provider <provider> or --model <model>"},
		{name: "print requires selector", arguments: []string{"auth", "print-api-key"}, want: "Credential printing requires --provider <provider> or --model <model>"},
		{name: "check rejects unknown option", arguments: []string{"auth", "check", "--wat"}, want: `Unknown option --wat for "auth check".`},
		{name: "api key rejects check option", arguments: []string{"auth", "print-api-key", "--provider", "openai", "--json"}, want: "--json is only supported by auth check"},
		{name: "bearer duration format", arguments: []string{"auth", "print-bearer-token", "--provider", "openai", "--min-expiry", "30"}, want: "--min-expiry must use a duration such as 30m or 1h"},
		{name: "auth rejects messages", arguments: []string{"auth", "check", "--provider", "openai", "message"}, want: "Auth commands only accept --provider and --model"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := codingagent.RunCLI(context.Background(), codingagent.CLIInvocation{Arguments: test.arguments})
			var argumentError *codingagent.CLIArgumentError
			if !errors.As(err, &argumentError) {
				t.Fatalf("RunCLI error = %T (%v), want *CLIArgumentError", err, err)
			}
			if argumentError.Message != test.want {
				t.Fatalf("CLIArgumentError.Message = %q, want %q", argumentError.Message, test.want)
			}
		})
	}
}

func TestRunCLIValidatesRootModeAndSessionConstraints(t *testing.T) {
	tests := []struct {
		name      string
		arguments []string
		want      string
	}{
		{name: "RPC rejects files", arguments: []string{"--mode", "rpc", "@prompt.md"}, want: "@file arguments are not supported in RPC mode"},
		{name: "fork conflicts with session", arguments: []string{"--fork", "old.jsonl", "--session", "new.jsonl"}, want: "--fork cannot be combined with --session"},
		{name: "fork reports all conflicts", arguments: []string{"--fork", "old.jsonl", "--continue", "--resume", "--no-session"}, want: "--fork cannot be combined with --continue, --resume, --no-session"},
		{name: "session ID conflicts with resume", arguments: []string{"--session-id", "session-1", "--resume"}, want: "--session-id cannot be combined with --resume"},
		{name: "empty name", arguments: []string{"--name", ""}, want: "--name requires a non-empty value"},
		{name: "whitespace name", arguments: []string{"--name", " \t "}, want: "--name requires a non-empty value"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := codingagent.RunCLI(context.Background(), codingagent.CLIInvocation{Arguments: test.arguments})
			var argumentError *codingagent.CLIArgumentError
			if !errors.As(err, &argumentError) || argumentError.Message != test.want {
				t.Fatalf("RunCLI error = %#v, want CLIArgumentError %q", err, test.want)
			}
		})
	}
}

func TestRunCLIValidatesCustomSessionID(t *testing.T) {
	const invalidSessionIDError = "Error: Session id must be non-empty, contain only alphanumeric characters, '-', '_', and '.', and start and end with an alphanumeric character"
	tests := []struct {
		name    string
		id      string
		wantErr string
	}{
		{name: "single alphanumeric character", id: "a"},
		{name: "interior punctuation", id: "abc-123_def.456"},
		{name: "empty", id: "", wantErr: invalidSessionIDError},
		{name: "leading hyphen", id: "-abc", wantErr: invalidSessionIDError},
		{name: "trailing hyphen", id: "abc-", wantErr: invalidSessionIDError},
		{name: "leading underscore", id: "_abc", wantErr: invalidSessionIDError},
		{name: "trailing underscore", id: "abc_", wantErr: invalidSessionIDError},
		{name: "leading period", id: ".abc", wantErr: invalidSessionIDError},
		{name: "trailing period", id: "abc.", wantErr: invalidSessionIDError},
		{name: "forward slash", id: "abc/def", wantErr: invalidSessionIDError},
		{name: "backslash", id: `abc\def`, wantErr: invalidSessionIDError},
		{name: "space", id: "abc def", wantErr: invalidSessionIDError},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := codingagent.RunCLI(context.Background(), codingagent.CLIInvocation{
				Arguments:        []string{"--session-id", test.id},
				StdinIsTerminal:  true,
				StdoutIsTerminal: true,
			})
			if test.wantErr == "" {
				var unavailable *codingagent.NotImplementedError
				if !errors.As(err, &unavailable) || unavailable.Operation != "mode.interactive" {
					t.Fatalf("RunCLI error = %#v, want codingagent.mode.interactive stub", err)
				}
				return
			}

			var argumentError *codingagent.CLIArgumentError
			if !errors.As(err, &argumentError) {
				t.Fatalf("RunCLI error = %T (%v), want *CLIArgumentError", err, err)
			}
			if err.Error() != test.wantErr {
				t.Fatalf("RunCLI error = %q, want %q", err, test.wantErr)
			}
		})
	}
}

func TestRunCLIValidatesRootConstraintsBeforeDownstreamRoutes(t *testing.T) {
	const invalidSessionID = "Session id must be non-empty, contain only alphanumeric characters, '-', '_', and '.', and start and end with an alphanumeric character"
	tests := []struct {
		name      string
		arguments []string
		want      string
	}{
		{name: "help after session ID", arguments: []string{"--session-id", "-bad", "--help"}, want: invalidSessionID},
		{name: "help after name", arguments: []string{"--name", " ", "--help"}, want: "--name requires a non-empty value"},
		{name: "model list after session ID", arguments: []string{"--session-id", "-bad", "--list-models"}, want: invalidSessionID},
		{name: "model list after name", arguments: []string{"--name", " ", "--list-models"}, want: "--name requires a non-empty value"},
		{name: "explicit extension after session ID", arguments: []string{"--session-id", "-bad", "--extension", "extension.ts"}, want: invalidSessionID},
		{name: "explicit extension after name", arguments: []string{"--name", " ", "--extension", "extension.ts"}, want: "--name requires a non-empty value"},
		{name: "extension flag after session ID", arguments: []string{"--session-id", "-bad", "--plan"}, want: invalidSessionID},
		{name: "extension flag after name", arguments: []string{"--name", " ", "--plan"}, want: "--name requires a non-empty value"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := codingagent.RunCLI(context.Background(), codingagent.CLIInvocation{Arguments: test.arguments})
			if result != (codingagent.CLIResult{}) {
				t.Fatalf("RunCLI result = %#v, want zero result", result)
			}
			var argumentError *codingagent.CLIArgumentError
			if !errors.As(err, &argumentError) || argumentError.Message != test.want {
				t.Fatalf("RunCLI error = %#v, want CLIArgumentError %q", err, test.want)
			}
		})
	}
}

func TestRunCLIVersionAndExportBypassRootConstraints(t *testing.T) {
	tests := []struct {
		name          string
		arguments     []string
		wantStdout    string
		wantOperation string
	}{
		{name: "version bypasses invalid session ID", arguments: []string{"--session-id", "-bad", "--version"}, wantStdout: codingagent.Version + "\n"},
		{name: "version bypasses blank name", arguments: []string{"--name", " ", "--version"}, wantStdout: codingagent.Version + "\n"},
		{name: "export bypasses invalid session ID", arguments: []string{"--session-id", "-bad", "--export", "session.jsonl"}, wantOperation: "session.export"},
		{name: "export bypasses blank name", arguments: []string{"--name", " ", "--export", "session.jsonl"}, wantOperation: "session.export"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := codingagent.RunCLI(context.Background(), codingagent.CLIInvocation{Arguments: test.arguments})
			if test.wantStdout != "" {
				if err != nil || result.Stdout != test.wantStdout {
					t.Fatalf("RunCLI = (%#v, %v), want stdout %q and nil error", result, err, test.wantStdout)
				}
				return
			}

			var unavailable *codingagent.NotImplementedError
			if !errors.As(err, &unavailable) || unavailable.Operation != test.wantOperation {
				t.Fatalf("RunCLI error = %#v, want codingagent.%s", err, test.wantOperation)
			}
		})
	}
}

func TestRunCLIRoutesOneShotRootOperationsToDedicatedStubs(t *testing.T) {
	for _, test := range []struct {
		arguments []string
		operation string
	}{
		{arguments: []string{"--list-models"}, operation: "models.list"},
		{arguments: []string{"--export", "session.jsonl"}, operation: "session.export"},
	} {
		_, err := codingagent.RunCLI(context.Background(), codingagent.CLIInvocation{Arguments: test.arguments})
		var unavailable *codingagent.NotImplementedError
		if !errors.As(err, &unavailable) || unavailable.Operation != test.operation {
			t.Fatalf("RunCLI(%q) error = %#v, want codingagent.%s", test.arguments, err, test.operation)
		}
	}
}

func TestCLIContractDeclaresModesAndExitSemantics(t *testing.T) {
	wantModes := []codingagent.CLIContractMode{
		{Name: "interactive", Selection: "text mode with terminal stdin and stdout", Operation: "mode.interactive"},
		{Name: "print", Selection: "--print or non-terminal stdin/stdout", Operation: "mode.print.text"},
		{Name: "json", Selection: "--mode json", Operation: "mode.json"},
		{Name: "rpc", Selection: "--mode rpc", Operation: "mode.rpc"},
	}
	if got := codingagent.StaticCLIContract().Modes; !reflect.DeepEqual(got, wantModes) {
		t.Fatalf("StaticCLIContract().Modes = %#v, want %#v", got, wantModes)
	}
	if got, want := codingagent.StaticCLIContract().ExitStatus, "0 for static help/version; 1 for argument errors and unavailable capabilities"; got != want {
		t.Fatalf("StaticCLIContract().ExitStatus = %q, want %q", got, want)
	}
}
