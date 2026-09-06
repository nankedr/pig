package codingagent

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/nankedr/pig/ai"
)

type credentialCommandResult struct {
	done  chan struct{}
	value string
	err   error
}

var credentialCommands = struct {
	sync.Mutex
	entries map[string]*credentialCommandResult
}{entries: map[string]*credentialCommandResult{}}

func resolveCredentialValue(ctx context.Context, value string, env ai.ProviderEnv) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if strings.HasPrefix(value, "!") {
		return cachedCredentialCommand(ctx, value)
	}
	var out strings.Builder
	for i := 0; i < len(value); {
		if value[i] != '$' || i+1 == len(value) {
			out.WriteByte(value[i])
			i++
			continue
		}
		next := value[i+1]
		if next == '$' || next == '!' {
			out.WriteByte(next)
			i += 2
			continue
		}
		start, end := i+1, i+1
		if next == '{' {
			close := strings.IndexByte(value[i+2:], '}')
			if close < 0 {
				out.WriteByte('$')
				i++
				continue
			}
			start = i + 2
			end = start + close
			if !credentialEnvName(value[start:end]) {
				out.WriteString(value[i : end+1])
				i = end + 1
				continue
			}
			i = end + 1
		} else {
			for end < len(value) && ((value[end] >= 'A' && value[end] <= 'Z') || (value[end] >= 'a' && value[end] <= 'z') || value[end] == '_' || (end > start && value[end] >= '0' && value[end] <= '9')) {
				end++
			}
			if end == start {
				out.WriteByte('$')
				i++
				continue
			}
			i = end
		}
		resolved := env[value[start:end]]
		if resolved == "" {
			resolved = os.Getenv(value[start:end])
		}
		if resolved == "" {
			return "", errors.New("stored API key environment reference could not be resolved")
		}
		out.WriteString(resolved)
	}
	return out.String(), nil
}

func credentialEnvName(value string) bool {
	if value == "" {
		return false
	}
	for i, c := range []byte(value) {
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || c == '_' || (i > 0 && c >= '0' && c <= '9') {
			continue
		}
		return false
	}
	return true
}

func cachedCredentialCommand(ctx context.Context, value string) (string, error) {
	credentialCommands.Lock()
	entry, ok := credentialCommands.entries[value]
	if !ok {
		entry = &credentialCommandResult{done: make(chan struct{})}
		credentialCommands.entries[value] = entry
	}
	credentialCommands.Unlock()
	if ok {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-entry.done:
			return entry.value, entry.err
		}
	}
	commandCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := credentialCommand(commandCtx, value[1:])
	cmd.Stderr = io.Discard
	output, err := cmd.Output()
	if err == nil {
		entry.value = strings.TrimSpace(string(output))
	}
	if err != nil || entry.value == "" {
		entry.err = errors.New("stored API key command could not be resolved")
	}
	if ctx.Err() != nil {
		entry.value = ""
		entry.err = ctx.Err()
		credentialCommands.Lock()
		delete(credentialCommands.entries, value)
		credentialCommands.Unlock()
	}
	close(entry.done)
	return entry.value, entry.err
}
