package codingagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"github.com/nankedr/pig/agent"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
)

func (s *AgentSession) ExecuteBash(ctx context.Context, command string, options ...ExecuteBashOptions) (BashResult, error) {
	if ctx == nil {
		return BashResult{}, fmt.Errorf("Bash context must not be nil")
	}
	var option ExecuteBashOptions
	if len(options) > 0 {
		option = options[0]
	}
	option.ID = cloneStringPointer(option.ID)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	s.mu.Lock()
	if s.disposed || s.agent == nil || s.sessionManager == nil {
		s.mu.Unlock()
		return BashResult{}, fmt.Errorf("Bash requires a live AgentSession and SessionManager")
	}
	if s.bashCancels == nil {
		s.bashCancels = make(map[*context.CancelFunc]struct{})
	}
	s.bashCancels[&cancel] = struct{}{}
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.bashCancels, &cancel); s.mu.Unlock() }()
	prefix, err := s.settingsManager.GetShellCommandPrefix()
	if err != nil {
		return BashResult{}, err
	}
	shell, err := s.settingsManager.GetShellPath()
	if err != nil {
		return BashResult{}, err
	}
	resolved := command
	if prefix != "" {
		resolved = prefix + "\n" + command
	}
	ops := option.Operations
	if ops == nil {
		ops = CreateLocalBashOperations(BashToolOptions{ShellPath: shell})
	}
	result, err := executeSessionBash(ctx, resolved, s.sessionManager.GetCWD(), ops, func(delta string) {
		if option.OnChunk != nil {
			option.OnChunk(delta)
		}
		s.emit(AgentSessionBashExecutionUpdateEvent{Type: AgentSessionEventTypeBashExecutionUpdate, ID: option.ID, Delta: delta})
	})
	if err != nil {
		return result, err
	}
	return result, s.recordBashResult(command, result, option.ExcludeFromContext, true)
}

func (s *AgentSession) AbortBash() error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for cancel := range s.bashCancels {
		(*cancel)()
	}
	return nil
}
func (s *AgentSession) IsBashRunning() (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.bashCancels) > 0, nil
}
func (s *AgentSession) HasPendingBashMessages() (bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.pendingBashMessages) > 0, nil
}
func (s *AgentSession) RecordBashResult(command string, result BashResult, options ...RecordBashResultOptions) error {
	exclude := len(options) > 0 && options[0].ExcludeFromContext
	return s.recordBashResult(command, result, exclude, false)
}
func (s *AgentSession) recordBashResult(command string, result BashResult, exclude, executing bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if (s.disposed && !executing) || s.agent == nil || s.sessionManager == nil {
		return fmt.Errorf("Bash recording requires a live AgentSession and SessionManager")
	}
	fields := map[string]any{"role": "bashExecution", "command": command, "output": result.Output, "cancelled": result.Cancelled, "truncated": result.Truncated, "timestamp": time.Now().UnixMilli()}
	if result.ExitCode != nil {
		fields["exitCode"] = *result.ExitCode
	}
	if result.FullOutputPath != nil {
		fields["fullOutputPath"] = *result.FullOutputPath
	}
	if exclude {
		fields["excludeFromContext"] = true
	}
	data, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	message, err := agent.NewRawAgentMessage(data)
	if err != nil {
		return err
	}
	if s.active {
		s.pendingBashMessages = append(s.pendingBashMessages, message)
		return nil
	}
	return s.appendBashMessageLocked(message)
}
func (s *AgentSession) appendBashMessageLocked(message agent.AgentMessage) error {
	if err := s.agent.AppendMessage(message); err != nil {
		return err
	}
	_, err := s.sessionManager.AppendMessage(message)
	return err
}
func (s *AgentSession) flushPendingBashMessagesLocked() error {
	var err error
	for _, message := range s.pendingBashMessages {
		err = errors.Join(err, s.appendBashMessageLocked(message))
	}
	s.pendingBashMessages = nil
	return err
}

var sessionBashANSI = regexp.MustCompile("(?:\\x1b\\][\\s\\S]*?(?:\\x07|\\x1b\\\\|\\x{009c}))|[\\x1b\\x{009b}][[\\]()#;?]*(?:\\d{1,4}(?:[;:]\\d{0,4})*)?[\\dA-PR-TZcf-nq-uy=><~]")

func executeSessionBash(ctx context.Context, command, cwd string, ops BashOperations, onChunk func(string)) (BashResult, error) {
	var mu sync.Mutex
	decoder := unicode.UTF8.NewDecoder()
	var pending []byte
	decodedAny := false
	var chunks []string
	totalBytes, bufferUnits := 0, 0
	var file *os.File
	var outputErr error
	var path *string
	ensureFile := func() {
		if file != nil || outputErr != nil {
			return
		}
		file, outputErr = os.CreateTemp("", "pig-bash-*.log")
		if outputErr != nil {
			return
		}
		name := file.Name()
		path = &name
		for _, chunk := range chunks {
			if _, err := file.WriteString(chunk); err != nil {
				outputErr = err
				return
			}
		}
	}
	accepting := true
	execution, execErr := ops.Exec(ctx, command, cwd, BashExecOptions{OnData: func(data []byte) {
		mu.Lock()
		defer mu.Unlock()
		if !accepting {
			return
		}
		totalBytes += len(data)
		data = append(pending, data...)
		decoded := make([]byte, len(data)*3)
		n, used, err := decoder.Transform(decoded, data, false)
		if err != nil && err != transform.ErrShortSrc {
			outputErr = err
		}
		pending = append([]byte(nil), data[used:]...)
		decodedText := string(decoded[:n])
		if !decodedAny && decodedText != "" {
			decodedAny = true
			decodedText = strings.TrimPrefix(decodedText, "\ufeff")
		}
		text := strings.Map(func(r rune) rune {
			if r == '\r' || (r < 32 && r != '\t' && r != '\n') || (r >= 0xfff9 && r <= 0xfffb) {
				return -1
			}
			return r
		}, sessionBashANSI.ReplaceAllString(decodedText, ""))
		if totalBytes > DefaultMaxBytes {
			ensureFile()
		}
		if file != nil {
			if _, err := file.WriteString(text); err != nil {
				outputErr = err
			}
		}
		chunks = append(chunks, text)
		bufferUnits += len(utf16.Encode([]rune(text)))
		for bufferUnits > DefaultMaxBytes*2 && len(chunks) > 1 {
			bufferUnits -= len(utf16.Encode([]rune(chunks[0])))
			chunks[0] = ""
			chunks = chunks[1:]
		}
		onChunk(text)
	}})
	mu.Lock()
	defer mu.Unlock()
	accepting = false
	cancelled := ctx.Err() != nil
	full := strings.Join(chunks, "")
	trunc := TruncateTail(full)
	if trunc.Truncated {
		ensureFile()
	}
	if file != nil {
		outputErr = errors.Join(outputErr, file.Close())
	}
	if outputErr != nil {
		return BashResult{}, outputErr
	}
	if execErr != nil && !cancelled {
		return BashResult{}, execErr
	}
	if cancelled {
		execution.ExitCode = nil
	}
	if trunc.Truncated {
		full = trunc.Content
	}
	return BashResult{Output: full, ExitCode: execution.ExitCode, Cancelled: cancelled, Truncated: trunc.Truncated, FullOutputPath: path}, nil
}
