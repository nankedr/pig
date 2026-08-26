package codingagent

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var updateIssue33CLIGolden = flag.Bool("update-issue33-cli-golden", false, "regenerate the Issue #33 CLI snapshots")

func TestIssue33StaticCLISnapshots(t *testing.T) {
	cases := []struct {
		name      string
		arguments []string
		file      string
	}{
		{name: "root long", arguments: []string{"--help"}, file: "pig_help.golden.txt"},
		{name: "root short", arguments: []string{"-h"}, file: "pig_help.golden.txt"},
		{name: "install long", arguments: []string{"install", "--help"}, file: "pig_install_help.golden.txt"},
		{name: "install short", arguments: []string{"install", "-h"}, file: "pig_install_help.golden.txt"},
		{name: "remove long", arguments: []string{"remove", "--help"}, file: "pig_remove_help.golden.txt"},
		{name: "remove short", arguments: []string{"remove", "-h"}, file: "pig_remove_help.golden.txt"},
		{name: "uninstall long", arguments: []string{"uninstall", "--help"}, file: "pig_remove_help.golden.txt"},
		{name: "uninstall short", arguments: []string{"uninstall", "-h"}, file: "pig_remove_help.golden.txt"},
		{name: "update long", arguments: []string{"update", "--help"}, file: "pig_update_help.golden.txt"},
		{name: "update short", arguments: []string{"update", "-h"}, file: "pig_update_help.golden.txt"},
		{name: "list long", arguments: []string{"list", "--help"}, file: "pig_list_help.golden.txt"},
		{name: "list short", arguments: []string{"list", "-h"}, file: "pig_list_help.golden.txt"},
		{name: "config long", arguments: []string{"config", "--help"}, file: "pig_config_help.golden.txt"},
		{name: "config short", arguments: []string{"config", "-h"}, file: "pig_config_help.golden.txt"},
		{name: "auth default", arguments: []string{"auth"}, file: "pig_auth_help.golden.txt"},
		{name: "auth help command", arguments: []string{"auth", "help"}, file: "pig_auth_help.golden.txt"},
		{name: "auth long", arguments: []string{"auth", "--help"}, file: "pig_auth_help.golden.txt"},
		{name: "auth short", arguments: []string{"auth", "-h"}, file: "pig_auth_help.golden.txt"},
		{name: "auth check", arguments: []string{"auth", "check", "--help"}, file: "pig_auth_help.golden.txt"},
		{name: "auth check short", arguments: []string{"auth", "check", "-h"}, file: "pig_auth_help.golden.txt"},
		{name: "auth api key", arguments: []string{"auth", "print-api-key", "--help"}, file: "pig_auth_help.golden.txt"},
		{name: "auth api key short", arguments: []string{"auth", "print-api-key", "-h"}, file: "pig_auth_help.golden.txt"},
		{name: "auth bearer token", arguments: []string{"auth", "print-bearer-token", "--help"}, file: "pig_auth_help.golden.txt"},
		{name: "auth bearer token short", arguments: []string{"auth", "print-bearer-token", "-h"}, file: "pig_auth_help.golden.txt"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			result, err := RunCLI(context.Background(), CLIInvocation{Arguments: test.arguments})
			if err != nil {
				t.Fatalf("RunCLI(%q): %v", test.arguments, err)
			}
			path := filepath.Join("testdata", test.file)
			if *updateIssue33CLIGolden {
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(result.Stdout), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if result.Stdout != string(want) {
				t.Fatalf("CLI snapshot drifted; regenerate with -update-issue33-cli-golden\n--- got ---\n%s\n--- want ---\n%s", result.Stdout, want)
			}
		})
	}
}

func TestIssue33CLIRoutingSnapshot(t *testing.T) {
	tests := []struct {
		name      string
		arguments []string
		stdinTTY  bool
		stdoutTTY bool
	}{
		// Static metadata and the two bare words that deliberately remain messages.
		{name: "version long", arguments: []string{"--version"}},
		{name: "version short", arguments: []string{"-v"}},
		{name: "version bypasses root constraints", arguments: []string{"--session-id", "-bad", "--version"}},
		{name: "bare help is a message", arguments: []string{"help"}, stdinTTY: true, stdoutTTY: true},
		{name: "bare version is a message", arguments: []string{"version"}, stdinTTY: true, stdoutTTY: true},

		// Every effective product mode and selection rule.
		{name: "interactive", stdinTTY: true, stdoutTTY: true},
		{name: "explicit text", arguments: []string{"--mode", "text"}, stdinTTY: true, stdoutTTY: true},
		{name: "print long", arguments: []string{"--print"}, stdinTTY: true, stdoutTTY: true},
		{name: "print short", arguments: []string{"-p"}, stdinTTY: true, stdoutTTY: true},
		{name: "piped stdin", stdoutTTY: true},
		{name: "redirected stdout", stdinTTY: true},
		{name: "json", arguments: []string{"--mode", "json"}, stdinTTY: true, stdoutTTY: true},
		{name: "rpc", arguments: []string{"--mode", "rpc"}, stdinTTY: true, stdoutTTY: true},

		// Every ordinary root flag and short alias not represented above.
		{name: "provider", arguments: []string{"--provider", "openai"}, stdinTTY: true, stdoutTTY: true},
		{name: "model", arguments: []string{"--model", "openai/gpt"}, stdinTTY: true, stdoutTTY: true},
		{name: "api key", arguments: []string{"--api-key", "test-key"}, stdinTTY: true, stdoutTTY: true},
		{name: "system prompt", arguments: []string{"--system-prompt", "prompt"}, stdinTTY: true, stdoutTTY: true},
		{name: "append system prompt", arguments: []string{"--append-system-prompt", "appendix"}, stdinTTY: true, stdoutTTY: true},
		{name: "continue long", arguments: []string{"--continue"}, stdinTTY: true, stdoutTTY: true},
		{name: "continue short", arguments: []string{"-c"}, stdinTTY: true, stdoutTTY: true},
		{name: "resume long", arguments: []string{"--resume"}, stdinTTY: true, stdoutTTY: true},
		{name: "resume short", arguments: []string{"-r"}, stdinTTY: true, stdoutTTY: true},
		{name: "session", arguments: []string{"--session", "session.jsonl"}, stdinTTY: true, stdoutTTY: true},
		{name: "session id", arguments: []string{"--session-id", "session-1"}, stdinTTY: true, stdoutTTY: true},
		{name: "session id with interior punctuation", arguments: []string{"--session-id", "abc-123_def.456"}, stdinTTY: true, stdoutTTY: true},
		{name: "fork", arguments: []string{"--fork", "old.jsonl"}, stdinTTY: true, stdoutTTY: true},
		{name: "session dir", arguments: []string{"--session-dir", "sessions"}, stdinTTY: true, stdoutTTY: true},
		{name: "no session", arguments: []string{"--no-session"}, stdinTTY: true, stdoutTTY: true},
		{name: "name long", arguments: []string{"--name", "review"}, stdinTTY: true, stdoutTTY: true},
		{name: "name short", arguments: []string{"-n", "review"}, stdinTTY: true, stdoutTTY: true},
		{name: "models", arguments: []string{"--models", "openai/*,anthropic/*"}, stdinTTY: true, stdoutTTY: true},
		{name: "no tools long", arguments: []string{"--no-tools"}, stdinTTY: true, stdoutTTY: true},
		{name: "no tools short", arguments: []string{"-nt"}, stdinTTY: true, stdoutTTY: true},
		{name: "no builtin tools long", arguments: []string{"--no-builtin-tools"}, stdinTTY: true, stdoutTTY: true},
		{name: "no builtin tools short", arguments: []string{"-nbt"}, stdinTTY: true, stdoutTTY: true},
		{name: "tools long", arguments: []string{"--tools", "read,bash"}, stdinTTY: true, stdoutTTY: true},
		{name: "tools short", arguments: []string{"-t", "read,bash"}, stdinTTY: true, stdoutTTY: true},
		{name: "exclude tools long", arguments: []string{"--exclude-tools", "bash"}, stdinTTY: true, stdoutTTY: true},
		{name: "exclude tools short", arguments: []string{"-xt", "bash"}, stdinTTY: true, stdoutTTY: true},
		{name: "thinking", arguments: []string{"--thinking", "high"}, stdinTTY: true, stdoutTTY: true},
		{name: "no extensions long", arguments: []string{"--no-extensions"}, stdinTTY: true, stdoutTTY: true},
		{name: "no extensions short", arguments: []string{"-ne"}, stdinTTY: true, stdoutTTY: true},
		{name: "skill", arguments: []string{"--skill", "skill.md"}, stdinTTY: true, stdoutTTY: true},
		{name: "no skills long", arguments: []string{"--no-skills"}, stdinTTY: true, stdoutTTY: true},
		{name: "no skills short", arguments: []string{"-ns"}, stdinTTY: true, stdoutTTY: true},
		{name: "prompt template", arguments: []string{"--prompt-template", "prompt.md"}, stdinTTY: true, stdoutTTY: true},
		{name: "no prompt templates long", arguments: []string{"--no-prompt-templates"}, stdinTTY: true, stdoutTTY: true},
		{name: "no prompt templates short", arguments: []string{"-np"}, stdinTTY: true, stdoutTTY: true},
		{name: "theme", arguments: []string{"--theme", "theme.json"}, stdinTTY: true, stdoutTTY: true},
		{name: "no themes", arguments: []string{"--no-themes"}, stdinTTY: true, stdoutTTY: true},
		{name: "no context files long", arguments: []string{"--no-context-files"}, stdinTTY: true, stdoutTTY: true},
		{name: "no context files short", arguments: []string{"-nc"}, stdinTTY: true, stdoutTTY: true},
		{name: "verbose", arguments: []string{"--verbose"}, stdinTTY: true, stdoutTTY: true},
		{name: "tui regular", arguments: []string{"--tui-mode", "regular"}, stdinTTY: true, stdoutTTY: true},
		{name: "tui fullscreen", arguments: []string{"--tui-mode", "fullscreen"}, stdinTTY: true, stdoutTTY: true},
		{name: "approve long", arguments: []string{"--approve"}, stdinTTY: true, stdoutTTY: true},
		{name: "approve short", arguments: []string{"-a"}, stdinTTY: true, stdoutTTY: true},
		{name: "no approve long", arguments: []string{"--no-approve"}, stdinTTY: true, stdoutTTY: true},
		{name: "no approve short", arguments: []string{"-na"}, stdinTTY: true, stdoutTTY: true},
		{name: "offline", arguments: []string{"--offline"}, stdinTTY: true, stdoutTTY: true},
		{name: "file argument", arguments: []string{"@prompt.md"}, stdinTTY: true, stdoutTTY: true},
		{name: "message argument", arguments: []string{"explain this"}, stdinTTY: true, stdoutTTY: true},

		// Every declared package/config command and advertised invocation form.
		{name: "install", arguments: []string{"install", "npm:@example/pkg"}},
		{name: "install short options", arguments: []string{"install", "npm:@example/pkg", "-l", "-a"}},
		{name: "install long options", arguments: []string{"install", "npm:@example/pkg", "--local", "--approve"}},
		{name: "install no approve short", arguments: []string{"install", "npm:@example/pkg", "-na"}},
		{name: "install no approve long", arguments: []string{"install", "npm:@example/pkg", "--no-approve"}},
		{name: "remove", arguments: []string{"remove", "npm:@example/pkg"}},
		{name: "remove short options", arguments: []string{"remove", "npm:@example/pkg", "-l", "-a"}},
		{name: "remove long options", arguments: []string{"remove", "npm:@example/pkg", "--local", "--no-approve"}},
		{name: "uninstall", arguments: []string{"uninstall", "npm:@example/pkg"}},
		{name: "uninstall options", arguments: []string{"uninstall", "npm:@example/pkg", "-l", "-na"}},
		{name: "update", arguments: []string{"update"}},
		{name: "update source", arguments: []string{"update", "npm:@example/pkg"}},
		{name: "update self target", arguments: []string{"update", "self"}},
		{name: "update pig target", arguments: []string{"update", "pig"}},
		{name: "update self flag", arguments: []string{"update", "--self"}},
		{name: "update extensions", arguments: []string{"update", "--extensions"}},
		{name: "update models", arguments: []string{"update", "--models"}},
		{name: "update all", arguments: []string{"update", "--all"}},
		{name: "update extension", arguments: []string{"update", "--extension", "npm:@example/pkg"}},
		{name: "update force", arguments: []string{"update", "--force"}},
		{name: "update approve short", arguments: []string{"update", "-a"}},
		{name: "update approve long", arguments: []string{"update", "--approve"}},
		{name: "update no approve short", arguments: []string{"update", "-na"}},
		{name: "update no approve long", arguments: []string{"update", "--no-approve"}},
		{name: "list", arguments: []string{"list"}},
		{name: "list approve short", arguments: []string{"list", "-a"}},
		{name: "list approve long", arguments: []string{"list", "--approve"}},
		{name: "list no approve short", arguments: []string{"list", "-na"}},
		{name: "list no approve long", arguments: []string{"list", "--no-approve"}},
		{name: "config", arguments: []string{"config"}},
		{name: "config short options", arguments: []string{"config", "-l", "-a"}},
		{name: "config long options", arguments: []string{"config", "--local", "--approve"}},
		{name: "config no approve short", arguments: []string{"config", "-na"}},
		{name: "config no approve long", arguments: []string{"config", "--no-approve"}},

		// Every auth operation, selector, and operation-specific option.
		{name: "auth check", arguments: []string{"auth", "check", "--provider", "openai"}},
		{name: "auth check model", arguments: []string{"auth", "check", "--model", "openai/gpt"}},
		{name: "auth check json", arguments: []string{"auth", "check", "--provider", "openai", "--json"}},
		{name: "auth check credentials", arguments: []string{"auth", "check", "--provider", "openai", "--credentials"}},
		{name: "auth check no refresh", arguments: []string{"auth", "check", "--provider", "openai", "--no-refresh"}},
		{name: "auth api key", arguments: []string{"auth", "print-api-key", "--provider", "openai"}},
		{name: "auth api key model", arguments: []string{"auth", "print-api-key", "--model", "openai/gpt"}},
		{name: "auth bearer token", arguments: []string{"auth", "print-bearer-token", "--provider", "openai"}},
		{name: "auth bearer token model", arguments: []string{"auth", "print-bearer-token", "--model", "openai/gpt"}},
		{name: "auth bearer token expiry", arguments: []string{"auth", "print-bearer-token", "--provider", "openai", "--min-expiry", "30m"}},

		// Extension Surface and one-shot operations.
		{name: "extension discovery long", arguments: []string{"--extension", "extension.ts"}},
		{name: "extension discovery short", arguments: []string{"-e", "extension.ts"}},
		{name: "extension flag bare", arguments: []string{"--plan"}},
		{name: "extension flag separate value", arguments: []string{"--review", "deep"}},
		{name: "extension flag equals value", arguments: []string{"--review=deep"}},
		{name: "first extension flag wins", arguments: []string{"--zeta", "--alpha"}},
		{name: "session export", arguments: []string{"--export", "session.jsonl"}},
		{name: "session export bypasses root constraints", arguments: []string{"--name", " ", "--export", "session.jsonl"}},
		{name: "model list all", arguments: []string{"--list-models"}},
		{name: "model list search", arguments: []string{"--list-models", "openai"}},

		// Distinct root, package/config, and auth argument-error boundaries.
		{name: "invalid mode", arguments: []string{"--mode", "invalid"}},
		{name: "missing mode", arguments: []string{"--mode"}},
		{name: "invalid mode precedes help", arguments: []string{"--mode", "invalid", "--help"}},
		{name: "missing tui mode", arguments: []string{"--tui-mode"}},
		{name: "invalid tui mode", arguments: []string{"--tui-mode", "invalid"}},
		{name: "missing thinking", arguments: []string{"--thinking"}},
		{name: "invalid thinking", arguments: []string{"--thinking", "invalid"}},
		{name: "unknown short option", arguments: []string{"-z"}},
		{name: "rpc rejects file", arguments: []string{"--mode", "rpc", "@prompt.md"}},
		{name: "fork session conflict", arguments: []string{"--fork", "old.jsonl", "--session", "new.jsonl"}},
		{name: "fork all conflicts", arguments: []string{"--fork", "old.jsonl", "--continue", "--resume", "--no-session"}},
		{name: "session id conflict", arguments: []string{"--session-id", "session-1", "--resume"}},
		{name: "invalid session id", arguments: []string{"--session-id", "-bad"}},
		{name: "empty name", arguments: []string{"--name", ""}},
		{name: "whitespace name", arguments: []string{"--name", " \t "}},
		{name: "session id validation precedes help", arguments: []string{"--session-id", "-bad", "--help"}},
		{name: "name validation precedes model list", arguments: []string{"--name", " ", "--list-models"}},
		{name: "fork validation precedes extension discovery", arguments: []string{"--fork", "old.jsonl", "--resume", "--extension", "extension.ts"}},
		{name: "RPC validation precedes extension flag", arguments: []string{"--mode", "rpc", "@prompt.md", "--plan"}},
		{name: "missing install source", arguments: []string{"install"}},
		{name: "missing remove source", arguments: []string{"remove"}},
		{name: "missing uninstall source", arguments: []string{"uninstall"}},
		{name: "install extra argument", arguments: []string{"install", "one", "two"}},
		{name: "remove unknown option", arguments: []string{"remove", "pkg", "--force"}},
		{name: "list unknown option", arguments: []string{"list", "--wat"}},
		{name: "config unknown option", arguments: []string{"config", "--force"}},
		{name: "config extra argument", arguments: []string{"config", "extra"}},
		{name: "update missing extension", arguments: []string{"update", "--extension"}},
		{name: "update duplicate extension", arguments: []string{"update", "--extension", "one", "--extension", "two"}},
		{name: "update all conflict", arguments: []string{"update", "--all", "--models"}},
		{name: "update all source conflict", arguments: []string{"update", "source", "--all"}},
		{name: "update models conflict", arguments: []string{"update", "--models", "--self"}},
		{name: "update models source conflict", arguments: []string{"update", "source", "--models"}},
		{name: "update extension flag conflict", arguments: []string{"update", "--extension", "one", "--self"}},
		{name: "update extension source conflict", arguments: []string{"update", "source", "--extension", "one"}},
		{name: "update positional conflict", arguments: []string{"update", "source", "--self"}},
		{name: "unknown auth command", arguments: []string{"auth", "login"}},
		{name: "missing auth selector", arguments: []string{"auth", "check"}},
		{name: "missing auth print selector", arguments: []string{"auth", "print-api-key"}},
		{name: "auth unknown option", arguments: []string{"auth", "check", "--wat"}},
		{name: "auth missing provider", arguments: []string{"auth", "check", "--provider"}},
		{name: "auth api key rejects json", arguments: []string{"auth", "print-api-key", "--provider", "openai", "--json"}},
		{name: "auth check rejects expiry", arguments: []string{"auth", "check", "--provider", "openai", "--min-expiry", "30m"}},
		{name: "auth invalid expiry", arguments: []string{"auth", "print-bearer-token", "--provider", "openai", "--min-expiry", "30"}},
		{name: "auth rejects message", arguments: []string{"auth", "check", "--provider", "openai", "message"}},
	}

	var snapshot strings.Builder
	for _, test := range tests {
		result, err := RunCLI(context.Background(), CLIInvocation{
			Arguments:        test.arguments,
			StdinIsTerminal:  test.stdinTTY,
			StdoutIsTerminal: test.stdoutTTY,
		})
		exitCode, stderr := 0, result.Stderr
		if err != nil {
			exitCode = 1
			stderr += err.Error() + "\n"
		}
		fmt.Fprintf(&snapshot, "%s\n  args: %q\n  terminals: stdin=%t stdout=%t\n  exit: %d\n  stdout: %q\n  stderr: %q\n",
			test.name, test.arguments, test.stdinTTY, test.stdoutTTY, exitCode, result.Stdout, stderr)
	}
	assertIssue33CLIGolden(t, "pig_routing.golden.txt", snapshot.String())
}

func TestIssue33ExperimentalCLIRoutingSnapshot(t *testing.T) {
	tests := []struct {
		name      string
		arguments []string
	}{
		{name: "pig root"},
		{name: "pig listener", arguments: []string{"--listen", "unix:///tmp/pig.sock"}},
		{name: "pig auth token", arguments: []string{"--auth-token", "token"}},
		{name: "pig auth token file", arguments: []string{"--auth-token-file", "/tmp/pig.token"}},
		{name: "server root", arguments: []string{"server"}},
		{name: "server", arguments: []string{"server", "--listen", "unix:///tmp/pig.sock"}},
		{name: "server auth", arguments: []string{"server", "--auth-token", "token"}},
		{name: "client root", arguments: []string{"client"}},
		{name: "client", arguments: []string{"client", "--connect", "unix:///tmp/pig.sock"}},
		{name: "client auth", arguments: []string{"client", "--auth-token-file", "/tmp/pig.token"}},
		{name: "legacy remainder", arguments: []string{"--model", "openai/gpt", "--listen=unix:///tmp/ignored.sock"}},
		{name: "root help is legacy input", arguments: []string{"--help"}},
		{name: "missing listener", arguments: []string{"--listen"}},
		{name: "duplicate listener", arguments: []string{"--listen", "unix:///tmp/pig.sock", "--listen=unix:///tmp/other.sock"}},
		{name: "duplicate connector", arguments: []string{"client", "--connect", "unix:///tmp/pig.sock", "--connect=unix:///tmp/other.sock"}},
		{name: "duplicate auth token", arguments: []string{"--auth-token", "one", "--auth-token", "two"}},
		{name: "mutually exclusive auth", arguments: []string{"--auth-token", "token", "--auth-token-file", "/tmp/pig.token"}},
		{name: "root connector is client only", arguments: []string{"--connect="}},
		{name: "invalid transport", arguments: []string{"server", "--listen", "ws://localhost:8080"}},
		{name: "invalid address", arguments: []string{"--listen", "relative.sock"}},
		{name: "authority forbidden", arguments: []string{"--listen", "unix://relative.sock"}},
		{name: "client rejects legacy option", arguments: []string{"client", "--listen", "unix:///tmp/pig.sock"}},
		{name: "server rejects legacy option", arguments: []string{"server", "--model", "openai/gpt"}},
	}

	var snapshot strings.Builder
	for _, test := range tests {
		err := runExperimentalCLI(context.Background(), test.arguments)
		exitCode, stderr := 0, ""
		if err != nil {
			exitCode = 1
			stderr = err.Error() + "\n"
		}
		fmt.Fprintf(&snapshot, "%s\n  args: %q\n  exit: %d\n  stdout: %q\n  stderr: %q\n",
			test.name, test.arguments, exitCode, "", stderr)
	}
	assertIssue33CLIGolden(t, "pig_experimental_routing.golden.txt", snapshot.String())
}

func assertIssue33CLIGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *updateIssue33CLIGolden {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatalf("CLI snapshot drifted; regenerate with -update-issue33-cli-golden\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}
