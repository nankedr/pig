package codingagent

import (
	"context"
	"path"
	"strings"
	"unicode/utf8"
)

const (
	experimentalCommandPig    = "pig"
	experimentalCommandServer = "server"
	experimentalCommandClient = "client"
)

type experimentalCLIOption struct {
	name  string
	value string
}

// runExperimentalCLI validates the dormant experimental command surface. It
// deliberately stops at the capability boundary: valid invocations return a
// structured stub without reading credentials, opening transports, or starting
// a runtime.
func runExperimentalCLI(ctx context.Context, arguments []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	command := experimentalCommandPig
	if len(arguments) > 0 && (arguments[0] == experimentalCommandServer || arguments[0] == experimentalCommandClient) {
		command = arguments[0]
		arguments = arguments[1:]
	}

	options, remainder, parseErrors := parseExperimentalCLIOptions(command, arguments)
	errors := append([]string(nil), parseErrors...)

	var authToken, authTokenFile bool
	for _, option := range options {
		switch option.name {
		case "--auth-token":
			authToken = true
		case "--auth-token-file":
			authTokenFile = true
		}
	}
	if authToken && authTokenFile {
		errors = append(errors, "--auth-token and --auth-token-file are mutually exclusive")
	}

	legacy := ParseArgs(remainder)
	for _, diagnostic := range legacy.Diagnostics {
		if diagnostic.Type == "error" {
			errors = append(errors, diagnostic.Message)
		}
	}
	if command == experimentalCommandPig {
		if _, exists := legacy.UnknownFlags["connect"]; exists {
			errors = append(errors, "--connect is only valid for client mode")
		}
	} else if len(remainder) > 0 {
		errors = append(errors, "The experimental "+command+" command does not support existing CLI options yet")
	}

	if len(errors) > 0 {
		return &CLIArgumentError{Message: errors[0]}
	}
	return notImplemented("experimental." + command)
}

func parseExperimentalCLIOptions(command string, arguments []string) ([]experimentalCLIOption, []string, []string) {
	registered := map[string]bool{
		"--auth-token":      true,
		"--auth-token-file": true,
	}
	switch command {
	case experimentalCommandPig, experimentalCommandServer:
		registered["--listen"] = true
	case experimentalCommandClient:
		registered["--connect"] = true
	}

	options := make([]experimentalCLIOption, 0, len(registered))
	seen := make(map[string]bool, len(registered))
	errors := make([]string, 0)
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		if argument == "--" {
			return options, arguments[index:], errors
		}

		name, value, hasEquals := strings.Cut(argument, "=")
		if !registered[name] {
			return options, arguments[index:], errors
		}
		if !hasEquals {
			if index+1 < len(arguments) && !strings.HasPrefix(arguments[index+1], "-") {
				index++
				value = arguments[index]
			}
		}
		if value == "" {
			errors = append(errors, name+" requires a value")
			continue
		}
		if seen[name] {
			errors = append(errors, name+" may only be specified once")
			continue
		}

		if name == "--listen" || name == "--connect" {
			if err := validateExperimentalTransport(value, name); err != "" {
				errors = append(errors, err)
				continue
			}
		}
		seen[name] = true
		options = append(options, experimentalCLIOption{name: name, value: value})
	}
	return options, nil, errors
}

func validateExperimentalTransport(value, option string) string {
	invalid := `Invalid ` + option + ` address "` + value + `"`
	scheme, ok := experimentalURLScheme(value)
	if !ok {
		return invalid
	}
	if !strings.EqualFold(scheme, "unix") {
		return `Unsupported ` + option + ` transport "` + strings.ToLower(scheme) + `:"`
	}

	// An authority is everything between the leading // and the next path,
	// query, or fragment delimiter. Empty authority is required for unix:///path.
	remainder := value[len(scheme)+1:]
	if strings.HasPrefix(remainder, "//") {
		authority := remainder[2:]
		if end := strings.IndexAny(authority, "/?#"); end >= 0 {
			authority = authority[:end]
		}
		if authority != "" {
			return "Unix transport address must not include an authority"
		}
	}

	if !strings.HasPrefix(value, "unix:///") || strings.HasPrefix(value, "unix:////") ||
		strings.ContainsAny(value, "?#") {
		return invalid
	}

	rawPath := value[len("unix://"):]
	if !canonicalExperimentalURLPath(rawPath) {
		return invalid
	}
	decodedPath, ok := decodeExperimentalURLPath(rawPath)
	if !ok || strings.ContainsRune(decodedPath, '\x00') {
		return invalid
	}
	if !path.IsAbs(decodedPath) {
		return "Unix transport address requires an absolute path"
	}
	return ""
}

func experimentalURLScheme(value string) (string, bool) {
	colon := strings.IndexByte(value, ':')
	if colon <= 0 {
		return "", false
	}
	for index := 0; index < colon; index++ {
		character := value[index]
		if index == 0 {
			if !asciiLetter(character) {
				return "", false
			}
			continue
		}
		if !asciiLetter(character) && (character < '0' || character > '9') &&
			character != '+' && character != '-' && character != '.' {
			return "", false
		}
	}
	return value[:colon], true
}

func asciiLetter(character byte) bool {
	return character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z'
}

func canonicalExperimentalURLPath(rawPath string) bool {
	for index := 0; index < len(rawPath); index++ {
		character := rawPath[index]
		// These bytes are percent-encoded by the URL serializer. Rejecting them
		// preserves the pinned url.href === input canonicality check.
		if character <= 0x20 || character >= 0x7f || strings.ContainsRune("\"<>^`{}", rune(character)) {
			return false
		}
	}
	for _, segment := range strings.Split(rawPath, "/") {
		switch strings.ToLower(segment) {
		case ".", "%2e", "..", ".%2e", "%2e.", "%2e%2e":
			return false
		}
	}
	return true
}

func decodeExperimentalURLPath(rawPath string) (string, bool) {
	decoded := make([]byte, 0, len(rawPath))
	for index := 0; index < len(rawPath); index++ {
		if rawPath[index] != '%' {
			decoded = append(decoded, rawPath[index])
			continue
		}
		if index+2 >= len(rawPath) {
			return "", false
		}
		high, highOK := experimentalHexDigit(rawPath[index+1])
		low, lowOK := experimentalHexDigit(rawPath[index+2])
		if !highOK || !lowOK {
			return "", false
		}
		decoded = append(decoded, high<<4|low)
		index += 2
	}
	if !utf8.Valid(decoded) {
		return "", false
	}
	return string(decoded), true
}

func experimentalHexDigit(character byte) (byte, bool) {
	switch {
	case character >= '0' && character <= '9':
		return character - '0', true
	case character >= 'a' && character <= 'f':
		return character - 'a' + 10, true
	case character >= 'A' && character <= 'F':
		return character - 'A' + 10, true
	default:
		return 0, false
	}
}
