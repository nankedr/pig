package codingagent

import "testing"

func TestListModelsOption(t *testing.T) {
	t.Run("zero value is absent", func(t *testing.T) {
		var option ListModelsOption
		if option.IsSet() {
			t.Fatal("zero ListModelsOption is set, want absent")
		}
		if option.IsAll() {
			t.Fatal("zero ListModelsOption selects all models, want absent")
		}
		if search, ok := option.Search(); ok {
			t.Fatalf("zero ListModelsOption search = %q, want no search", search)
		}
	})

	t.Run("bare flag selects all models", func(t *testing.T) {
		option := ParseArgs([]string{"--list-models"}).ListModels
		if !option.IsSet() {
			t.Fatal("bare --list-models is absent, want set")
		}
		if !option.IsAll() {
			t.Fatal("bare --list-models does not select all models")
		}
		if search, ok := option.Search(); ok {
			t.Fatalf("bare --list-models search = %q, want no search", search)
		}
	})

	t.Run("search preserves an empty string", func(t *testing.T) {
		option := ParseArgs([]string{"--list-models", ""}).ListModels
		if !option.IsSet() {
			t.Fatal("--list-models with an empty search is absent, want set")
		}
		if option.IsAll() {
			t.Fatal("--list-models with an empty search selects all models")
		}
		if search, ok := option.Search(); !ok || search != "" {
			t.Fatalf("Search() = (%q, %t), want (empty string, true)", search, ok)
		}
	})
}

func TestParseArgsClosedCLIUnions(t *testing.T) {
	t.Run("list models forms", func(t *testing.T) {
		tests := []struct {
			name           string
			arguments      []string
			wantSet        bool
			wantAll        bool
			wantSearch     string
			wantSearchSet  bool
			wantUnknownKey string
			wantUnknown    *string
			wantUnknownSet bool
			wantFileArgs   []string
		}{
			{name: "absent"},
			{name: "bare", arguments: []string{"--list-models"}, wantSet: true, wantAll: true},
			{name: "separate string", arguments: []string{"--list-models", "claude"}, wantSet: true, wantSearch: "claude", wantSearchSet: true},
			{name: "separate empty string", arguments: []string{"--list-models", ""}, wantSet: true, wantSearchSet: true},
			{name: "next flag", arguments: []string{"--list-models", "--extension-flag"}, wantSet: true, wantAll: true, wantUnknownKey: "extension-flag", wantUnknownSet: true},
			{name: "next file", arguments: []string{"--list-models", "@prompt.md"}, wantSet: true, wantAll: true, wantFileArgs: []string{"prompt.md"}},
			{name: "equals string is unknown", arguments: []string{"--list-models=claude"}, wantUnknownKey: "list-models", wantUnknown: stringPointer("claude"), wantUnknownSet: true},
			{name: "equals empty string is unknown", arguments: []string{"--list-models="}, wantUnknownKey: "list-models", wantUnknown: stringPointer(""), wantUnknownSet: true},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				got := ParseArgs(test.arguments)
				if got.ListModels.IsSet() != test.wantSet {
					t.Fatalf("ListModels.IsSet() = %t, want %t", got.ListModels.IsSet(), test.wantSet)
				}
				if got.ListModels.IsAll() != test.wantAll {
					t.Fatalf("ListModels.IsAll() = %t, want %t", got.ListModels.IsAll(), test.wantAll)
				}
				search, searchSet := got.ListModels.Search()
				if search != test.wantSearch || searchSet != test.wantSearchSet {
					t.Fatalf("ListModels.Search() = (%q, %t), want (%q, %t)", search, searchSet, test.wantSearch, test.wantSearchSet)
				}
				assertUnknownFlag(t, got.UnknownFlags, test.wantUnknownKey, test.wantUnknown, test.wantUnknownSet)
				assertStrings(t, "FileArgs", got.FileArgs, test.wantFileArgs)
			})
		}
	})

	t.Run("unknown flag forms", func(t *testing.T) {
		tests := []struct {
			name           string
			arguments      []string
			flag           string
			wantValue      *string
			wantFileArgs   []string
			additionalFlag string
			additional     *string
		}{
			{name: "bare", arguments: []string{"--feature"}, flag: "feature"},
			{name: "equals string", arguments: []string{"--feature=value"}, flag: "feature", wantValue: stringPointer("value")},
			{name: "equals empty string", arguments: []string{"--feature="}, flag: "feature", wantValue: stringPointer("")},
			{name: "equals splits once", arguments: []string{"--feature=a=b"}, flag: "feature", wantValue: stringPointer("a=b")},
			{name: "separate string", arguments: []string{"--feature", "value"}, flag: "feature", wantValue: stringPointer("value")},
			{name: "separate empty string", arguments: []string{"--feature", ""}, flag: "feature", wantValue: stringPointer("")},
			{name: "next flag", arguments: []string{"--feature", "--other=value"}, flag: "feature", additionalFlag: "other", additional: stringPointer("value")},
			{name: "next file", arguments: []string{"--feature", "@prompt.md"}, flag: "feature", wantFileArgs: []string{"prompt.md"}},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				got := ParseArgs(test.arguments)
				assertUnknownFlag(t, got.UnknownFlags, test.flag, test.wantValue, true)
				if test.additionalFlag != "" {
					assertUnknownFlag(t, got.UnknownFlags, test.additionalFlag, test.additional, true)
				}
				assertStrings(t, "FileArgs", got.FileArgs, test.wantFileArgs)
			})
		}
	})

	t.Run("unknown flags map is initialized", func(t *testing.T) {
		var flags map[string]*string = ParseArgs(nil).UnknownFlags
		if flags == nil {
			t.Fatal("UnknownFlags is nil, want initialized empty map")
		}
	})
}

func TestParseArgsMatchesPinnedValueEdgeCases(t *testing.T) {
	t.Run("ordinary required-value flags consume option-shaped tokens", func(t *testing.T) {
		for _, flag := range []string{
			"--provider", "--model", "--api-key", "--system-prompt",
			"--append-system-prompt", "--name", "-n", "--session",
			"--session-id", "--fork", "--session-dir", "--export",
			"--extension", "-e", "--skill", "--prompt-template", "--theme",
			"--models", "--tools", "-t", "--exclude-tools", "-xt",
		} {
			t.Run(flag, func(t *testing.T) {
				got := ParseArgs([]string{flag, "--help"})
				if got.Help {
					t.Fatal("Help = true, want following option-shaped token consumed as the value")
				}
				if len(got.Diagnostics) != 0 {
					t.Fatalf("Diagnostics = %#v, want none", got.Diagnostics)
				}
				value, ok := parsedOrdinaryRequiredValue(got, flag)
				if !ok || value != "--help" {
					t.Fatalf("parsed value = (%q, %t), want (%q, true)", value, ok, "--help")
				}
			})
		}
	})

	t.Run("EOF is missing for ordinary required-value flags", func(t *testing.T) {
		tests := []struct {
			flag     string
			wantFlag string
		}{
			{flag: "--mode"},
			{flag: "--provider"},
			{flag: "--model"},
			{flag: "--api-key"},
			{flag: "--system-prompt"},
			{flag: "--append-system-prompt"},
			{flag: "--name"},
			{flag: "-n", wantFlag: "--name"},
			{flag: "--session"},
			{flag: "--session-id"},
			{flag: "--fork"},
			{flag: "--session-dir"},
			{flag: "--export"},
			{flag: "--extension"},
			{flag: "-e"},
			{flag: "--skill"},
			{flag: "--prompt-template"},
			{flag: "--theme"},
			{flag: "--models"},
			{flag: "--tools"},
			{flag: "-t"},
			{flag: "--exclude-tools"},
			{flag: "-xt"},
			{flag: "--thinking"},
		}

		for _, test := range tests {
			t.Run(test.flag, func(t *testing.T) {
				got := ParseArgs([]string{test.flag})
				wantFlag := test.wantFlag
				if wantFlag == "" {
					wantFlag = test.flag
				}
				wantDiagnostic := ArgDiagnostic{Type: "error", Message: wantFlag + " requires a value"}
				if len(got.UnknownFlags) != 0 {
					t.Fatalf("UnknownFlags = %#v, want no extension flags", got.UnknownFlags)
				}
				if len(got.Diagnostics) != 1 || got.Diagnostics[0] != wantDiagnostic {
					t.Fatalf("Diagnostics = %#v, want %#v", got.Diagnostics, []ArgDiagnostic{wantDiagnostic})
				}
			})
		}
	})

	t.Run("tui mode is the option-shaped exception", func(t *testing.T) {
		tests := []struct {
			name         string
			arguments    []string
			wantHelp     bool
			wantMessage  string
			wantMessages []string
		}{
			{
				name:        "EOF",
				arguments:   []string{"--tui-mode"},
				wantMessage: "--tui-mode requires regular or fullscreen",
			},
			{
				name:        "following option is not consumed",
				arguments:   []string{"--tui-mode", "--help"},
				wantHelp:    true,
				wantMessage: "--tui-mode requires regular or fullscreen",
			},
			{
				name:         "invalid non-option is consumed",
				arguments:    []string{"--tui-mode", "bogus", "message"},
				wantMessage:  `Invalid TUI mode "bogus". Valid values: regular, fullscreen`,
				wantMessages: []string{"message"},
			},
			{
				name:        "empty value is invalid",
				arguments:   []string{"--tui-mode", ""},
				wantMessage: `Invalid TUI mode "". Valid values: regular, fullscreen`,
			},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				got := ParseArgs(test.arguments)
				wantDiagnostic := ArgDiagnostic{Type: "error", Message: test.wantMessage}
				if got.Help != test.wantHelp {
					t.Fatalf("Help = %t, want %t", got.Help, test.wantHelp)
				}
				if len(got.Diagnostics) != 1 || got.Diagnostics[0] != wantDiagnostic {
					t.Fatalf("Diagnostics = %#v, want %#v", got.Diagnostics, []ArgDiagnostic{wantDiagnostic})
				}
				assertStrings(t, "Messages", got.Messages, test.wantMessages)
			})
		}
	})

	t.Run("invalid mode values are errors", func(t *testing.T) {
		tests := []struct {
			name        string
			flag        string
			value       string
			wantMessage string
		}{
			{name: "mode", flag: "--mode", value: "bogus", wantMessage: `Invalid mode "bogus". Valid values: text, json, rpc`},
			{name: "mode consumes option-shaped value", flag: "--mode", value: "--help", wantMessage: `Invalid mode "--help". Valid values: text, json, rpc`},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				got := ParseArgs([]string{test.flag, test.value})
				wantDiagnostic := ArgDiagnostic{Type: "error", Message: test.wantMessage}
				if got.Help {
					t.Fatal("Help = true, want enum option to consume its value")
				}
				if len(got.Diagnostics) != 1 || got.Diagnostics[0] != wantDiagnostic {
					t.Fatalf("Diagnostics = %#v, want %#v", got.Diagnostics, []ArgDiagnostic{wantDiagnostic})
				}
			})
		}
	})

	t.Run("invalid thinking values are warnings", func(t *testing.T) {
		tests := []struct {
			name        string
			value       string
			wantMessage string
		}{
			{name: "ordinary value", value: "bogus", wantMessage: `Invalid thinking level "bogus". Valid values: off, minimal, low, medium, high, xhigh, max`},
			{name: "option-shaped value", value: "--help", wantMessage: `Invalid thinking level "--help". Valid values: off, minimal, low, medium, high, xhigh, max`},
		}

		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				got := ParseArgs([]string{"--thinking", test.value})
				wantDiagnostic := ArgDiagnostic{Type: "warning", Message: test.wantMessage}
				if got.Help {
					t.Fatal("Help = true, want thinking option to consume its value")
				}
				if len(got.Diagnostics) != 1 || got.Diagnostics[0] != wantDiagnostic {
					t.Fatalf("Diagnostics = %#v, want %#v", got.Diagnostics, []ArgDiagnostic{wantDiagnostic})
				}
			})
		}
	})

	t.Run("unknown short option uses the pinned diagnostic", func(t *testing.T) {
		got := ParseArgs([]string{"-z"})
		wantDiagnostic := ArgDiagnostic{Type: "error", Message: "Unknown option: -z"}
		if len(got.Diagnostics) != 1 || got.Diagnostics[0] != wantDiagnostic {
			t.Fatalf("Diagnostics = %#v, want %#v", got.Diagnostics, []ArgDiagnostic{wantDiagnostic})
		}
	})

	t.Run("models preserve empty patterns while tools filter them", func(t *testing.T) {
		got := ParseArgs([]string{"--models", "a,, b, ", "--tools", "read,, ", "--exclude-tools", "bash,, "})
		assertStrings(t, "Models", got.Models, []string{"a", "", "b", ""})
		assertStrings(t, "Tools", got.Tools, []string{"read"})
		assertStrings(t, "ExcludeTools", got.ExcludeTools, []string{"bash"})
	})
}

func parsedOrdinaryRequiredValue(args Args, flag string) (string, bool) {
	switch flag {
	case "--provider":
		return pointedString(args.Provider)
	case "--model":
		return pointedString(args.Model)
	case "--api-key":
		return pointedString(args.APIKey)
	case "--system-prompt":
		return pointedString(args.SystemPrompt)
	case "--name", "-n":
		return pointedString(args.Name)
	case "--session":
		return pointedString(args.Session)
	case "--session-id":
		return pointedString(args.SessionID)
	case "--fork":
		return pointedString(args.Fork)
	case "--session-dir":
		return pointedString(args.SessionDir)
	case "--export":
		return pointedString(args.Export)
	case "--append-system-prompt":
		return singleString(args.AppendSystemPrompt)
	case "--extension", "-e":
		return singleString(args.Extensions)
	case "--skill":
		return singleString(args.Skills)
	case "--prompt-template":
		return singleString(args.PromptTemplates)
	case "--theme":
		return singleString(args.Themes)
	case "--models":
		return singleString(args.Models)
	case "--tools", "-t":
		return singleString(args.Tools)
	case "--exclude-tools", "-xt":
		return singleString(args.ExcludeTools)
	default:
		return "", false
	}
}

func pointedString(value *string) (string, bool) {
	if value == nil {
		return "", false
	}
	return *value, true
}

func singleString(values []string) (string, bool) {
	if len(values) != 1 {
		return "", false
	}
	return values[0], true
}

func assertUnknownFlag(t *testing.T, flags map[string]*string, key string, want *string, wantSet bool) {
	t.Helper()
	if key == "" {
		if len(flags) != 0 {
			t.Fatalf("UnknownFlags = %#v, want empty map", flags)
		}
		return
	}

	got, ok := flags[key]
	if ok != wantSet {
		t.Fatalf("UnknownFlags[%q] presence = %t, want %t", key, ok, wantSet)
	}
	if !ok {
		return
	}
	if want == nil {
		if got != nil {
			t.Fatalf("UnknownFlags[%q] = %q, want bare true (nil)", key, *got)
		}
		return
	}
	if got == nil || *got != *want {
		if got == nil {
			t.Fatalf("UnknownFlags[%q] = nil, want %q", key, *want)
		}
		t.Fatalf("UnknownFlags[%q] = %q, want %q", key, *got, *want)
	}
}

func assertStrings(t *testing.T, name string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s = %#v, want %#v", name, got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("%s = %#v, want %#v", name, got, want)
		}
	}
}

func stringPointer(value string) *string {
	return &value
}
