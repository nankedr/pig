package codingagent

import (
	"context"
	"errors"
	"testing"
)

func TestExperimentalCLIPinnedParsingEdges(t *testing.T) {
	tests := []struct {
		name      string
		arguments []string
		wantStub  string
		wantError string
	}{
		{name: "legacy remainder prevents later experimental parsing", arguments: []string{"--model", "claude-sonnet", "--listen=unix:///tmp/second.sock"}, wantStub: "experimental.pig"},
		{name: "later command name is a legacy message", arguments: []string{"--cwd", "/workspace", "server"}, wantStub: "experimental.pig"},
		{name: "root help remains legacy input", arguments: []string{"--help"}, wantStub: "experimental.pig"},
		{name: "duplicate listen", arguments: []string{"--listen", "unix:///tmp/pig.sock", "--listen=unix:///tmp/admin.sock"}, wantError: "--listen may only be specified once"},
		{name: "duplicate connect", arguments: []string{"client", "--connect", "unix:///tmp/pig.sock", "--connect=unix:///tmp/other.sock"}, wantError: "--connect may only be specified once"},
		{name: "duplicate token", arguments: []string{"--auth-token", "first", "--auth-token", "second"}, wantError: "--auth-token may only be specified once"},
		{name: "empty experimental value", arguments: []string{"--listen="}, wantError: "--listen requires a value"},
		{name: "root connect is client only", arguments: []string{"--connect="}, wantError: "--connect is only valid for client mode"},
		{name: "client remainder suppresses later auth validation", arguments: []string{"client", "--listen", "ws://localhost:8080", "--auth-token", "secret", "--auth-token-file", "token"}, wantError: "The experimental client command does not support existing CLI options yet"},
		{name: "server help is unsupported legacy input", arguments: []string{"server", "--help"}, wantError: "The experimental server command does not support existing CLI options yet"},
		{name: "legacy diagnostic precedes unsupported input", arguments: []string{"client", "--tui-mode", "wrong"}, wantError: `Invalid TUI mode "wrong". Valid values: regular, fullscreen`},
		{name: "authority forbidden", arguments: []string{"--listen", "unix://relative.sock"}, wantError: "Unix transport address must not include an authority"},
		{name: "query forbidden", arguments: []string{"--listen", "unix:///tmp/pig.sock?wrong=value"}, wantError: `Invalid --listen address "unix:///tmp/pig.sock?wrong=value"`},
		{name: "fragment forbidden", arguments: []string{"--listen", "unix:///tmp/pig.sock#fragment"}, wantError: `Invalid --listen address "unix:///tmp/pig.sock#fragment"`},
		{name: "noncanonical slashes forbidden", arguments: []string{"--listen", "unix:/tmp/pig.sock"}, wantError: `Invalid --listen address "unix:/tmp/pig.sock"`},
		{name: "noncanonical dot segment forbidden", arguments: []string{"--listen", "unix:///tmp/../pig.sock"}, wantError: `Invalid --listen address "unix:///tmp/../pig.sock"`},
		{name: "unescaped unicode forbidden", arguments: []string{"--listen", "unix:///tmp/pig-猪.sock"}, wantError: `Invalid --listen address "unix:///tmp/pig-猪.sock"`},
		{name: "malformed escape forbidden", arguments: []string{"--listen", "unix:///tmp/%GG.sock"}, wantError: `Invalid --listen address "unix:///tmp/%GG.sock"`},
		{name: "invalid utf8 escape forbidden", arguments: []string{"--listen", "unix:///tmp/%FF.sock"}, wantError: `Invalid --listen address "unix:///tmp/%FF.sock"`},
		{name: "nul forbidden", arguments: []string{"--listen", "unix:///tmp/%00pig.sock"}, wantError: `Invalid --listen address "unix:///tmp/%00pig.sock"`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := runExperimentalCLI(context.Background(), test.arguments)
			if test.wantError != "" {
				var argumentError *CLIArgumentError
				if !errors.As(err, &argumentError) || argumentError.Message != test.wantError {
					t.Fatalf("runExperimentalCLI(%q) error = %#v, want CLIArgumentError %q", test.arguments, err, test.wantError)
				}
				return
			}
			var unavailable *NotImplementedError
			if !errors.As(err, &unavailable) || unavailable.Operation != test.wantStub {
				t.Fatalf("runExperimentalCLI(%q) error = %#v, want %s", test.arguments, err, test.wantStub)
			}
		})
	}
}

func TestExperimentalCLIPropagatesCancellationBeforeRuntimeDispatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := runExperimentalCLI(ctx, []string{"server", "--listen", "unix:///tmp/pig.sock"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("runExperimentalCLI() error = %v, want context.Canceled", err)
	}
}

func TestExperimentalCLIInventoriesDormantCommandsWithoutStartingAServer(t *testing.T) {
	for _, test := range []struct {
		arguments []string
		operation string
	}{
		{operation: "experimental.pig"},
		{arguments: []string{"--listen", "unix:///tmp/pig.sock"}, operation: "experimental.pig"},
		{arguments: []string{"server"}, operation: "experimental.server"},
		{arguments: []string{"server", "--listen", "unix:///tmp/pig.sock"}, operation: "experimental.server"},
		{arguments: []string{"client", "--connect", "unix:///tmp/pig.sock"}, operation: "experimental.client"},
	} {
		err := runExperimentalCLI(context.Background(), test.arguments)
		var unavailable *NotImplementedError
		if !errors.As(err, &unavailable) || unavailable.Module != "codingagent" || unavailable.Operation != test.operation {
			t.Fatalf("runExperimentalCLI(%q) error = %#v, want codingagent.%s", test.arguments, err, test.operation)
		}
	}
}

func TestExperimentalCLIValidatesDormantInputBeforeReturningAStub(t *testing.T) {
	for _, test := range []struct {
		arguments []string
		want      string
	}{
		{arguments: []string{"--auth-token", "secret", "--auth-token-file", "token"}, want: "--auth-token and --auth-token-file are mutually exclusive"},
		{arguments: []string{"--listen", "ws://localhost:8080"}, want: `Unsupported --listen transport "ws:"`},
		{arguments: []string{"--listen", "relative.sock"}, want: `Invalid --listen address "relative.sock"`},
		{arguments: []string{"client", "--connect"}, want: "--connect requires a value"},
		{arguments: []string{"client", "--listen", "unix:///tmp/pig.sock"}, want: "The experimental client command does not support existing CLI options yet"},
		{arguments: []string{"server", "--model", "model"}, want: "The experimental server command does not support existing CLI options yet"},
	} {
		err := runExperimentalCLI(context.Background(), test.arguments)
		var argumentError *CLIArgumentError
		if !errors.As(err, &argumentError) || argumentError.Message != test.want {
			t.Fatalf("runExperimentalCLI(%q) error = %#v, want CLIArgumentError %q", test.arguments, err, test.want)
		}
	}
}
