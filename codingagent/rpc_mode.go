package codingagent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"
)

// RunRPCMode owns the runtime and process stdin until EOF, cancellation or an I/O failure.
func RunRPCMode(ctx context.Context, runtime *AgentSessionRuntime) (result error) {
	if ctx == nil || runtime == nil || runtime.Session() == nil {
		return fmt.Errorf("RPC mode requires a context and AgentSession runtime")
	}
	defer runtime.Dispose(context.Background())
	input, err := openRPCPipe(os.Stdin)
	if err != nil {
		return err
	}
	defer input.Close()
	stdout, err := openRPCPipe(os.Stdout)
	if err != nil {
		return err
	}
	defer stdout.Close()
	stopIO := context.AfterFunc(ctx, func() { _ = input.Close(); _ = stdout.Close() })
	defer stopIO()
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	session := runtime.Session()
	writer := &jsonLineWriter{output: stdout}
	output := func(value any) {
		writer.Write(value)
		if err := writer.Err(); err != nil {
			cancel(err)
		}
	}
	unsubscribe, err := session.Subscribe(func(event AgentSessionEvent) {
		value, err := projectJSONAgentSessionEvent(event)
		if err != nil {
			cancel(err)
			return
		}
		output(value)
	})
	if err != nil {
		return err
	}
	defer unsubscribe()
	lines := make(chan string)
	readDone := make(chan struct{})
	var readErr error
	go func() {
		readErr = readRPCLines(input, func(line string) bool {
			select {
			case lines <- line:
				return true
			case <-ctx.Done():
				return false
			}
		})
		close(readDone)
		close(lines)
	}()
	var commands sync.WaitGroup
	defer func() {
		cancel(nil)
		_ = input.Close()
		_ = session.Abort()
		_ = stdout.SetWriteDeadline(time.Now().Add(time.Second))
		commands.Wait()
		if result == nil {
			result = writer.Err()
		}
		<-readDone
	}()
	for {
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case line, ok := <-lines:
			if !ok {
				return readErr
			}
			var parsed any
			if err := json.Unmarshal([]byte(line), &parsed); err != nil {
				response := map[string]any{"type": "response", "command": "parse", "success": false, "error": "Failed to parse command: " + err.Error()}
				commands.Add(1)
				go func() { defer commands.Done(); output(response) }()
				continue
			}
			if parsed == nil {
				return fmt.Errorf("Cannot read properties of null (reading 'id')")
			}
			fields, _ := parsed.(map[string]any)
			if fields != nil {
				var raw map[string]json.RawMessage
				_ = json.Unmarshal([]byte(line), &raw)
				if id, ok := raw["id"]; ok {
					fields["id"] = id
				}
			}
			commands.Add(1)
			go func() { defer commands.Done(); rpcCommand(ctx, session, fields, output) }()
		}
	}
}

func readRPCLines(input io.Reader, line func(string) bool) error {
	reader := bufio.NewReader(input)
	for {
		data, err := reader.ReadString('\n')
		if len(data) > 0 {
			data = strings.TrimSuffix(data, "\n")
			data = strings.TrimSuffix(data, "\r")
			if !line(data) {
				return nil
			}
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func rpcCommand(ctx context.Context, s *AgentSession, fields map[string]any, output func(any)) {
	response := map[string]any{"type": "response", "success": true}
	if value, ok := fields["id"]; ok {
		response["id"] = value
	}
	if value, ok := fields["type"]; ok {
		response["command"] = value
	}
	fail := func(err error) { response["success"] = false; response["error"] = err.Error(); output(response) }
	command, _ := fields["type"].(string)
	switch command {
	case "prompt":
		text, ok := fields["message"].(string)
		if !ok {
			message := "text.startsWith is not a function"
			if value, exists := fields["message"]; !exists {
				message = "Cannot read properties of undefined (reading 'startsWith')"
			} else if value == nil {
				message = "Cannot read properties of null (reading 'startsWith')"
			}
			fail(errors.New(message))
			return
		}
		if images := fields["images"]; images != nil {
			if values, ok := images.([]any); !ok || len(values) > 0 {
				fail(notImplemented("AgentSession.Prompt.Images"))
				return
			}
		}
		behavior := ""
		if rpcTruthy(fields["streamingBehavior"]) {
			behavior = "steer"
			if fields["streamingBehavior"] == "followUp" {
				behavior = "followUp"
			}
		}
		if s.IsStreaming() && behavior == "" {
			fail(errors.New("Agent is already processing. Specify streamingBehavior ('steer' or 'followUp') to queue the message."))
			return
		}
		if !s.IsStreaming() && s.ModelRuntime() != nil {
			auth, err := s.ModelRuntime().CheckAuth(ctx, string(s.Model().Provider))
			if err != nil {
				fail(err)
				return
			}
			if !auth.IsSet() {
				fail(fmt.Errorf("No API key found for %s", s.Model().Provider))
				return
			}
		}
		accepted := false
		err := s.promptWithPreflight(ctx, text, func() { accepted = true; output(response) }, PromptOptions{StreamingBehavior: behavior})
		if err != nil && !accepted {
			fail(err)
		}
		return
	case "abort":
		if err := s.Abort(); err != nil {
			fail(err)
			return
		}
		if err := s.WaitForIdle(ctx); err != nil {
			fail(err)
			return
		}
	case "get_state":
		state := s.State()
		compacting, _ := s.IsCompacting()
		count, _ := s.PendingMessageCount()
		data := map[string]any{"model": state.Model, "thinkingLevel": state.ThinkingLevel, "isStreaming": s.IsStreaming(), "isCompacting": compacting, "steeringMode": s.SteeringMode(), "followUpMode": s.FollowUpMode(), "sessionId": s.SessionID(), "autoCompactionEnabled": s.AutoCompactionEnabled(), "messageCount": len(state.Messages), "pendingMessageCount": count}
		if file := s.SessionFile(); file != nil {
			data["sessionFile"] = *file
		}
		if name := s.SessionName(); name != nil {
			data["sessionName"] = *name
		}
		response["data"] = data
	case "get_messages":
		messages := s.Messages()
		if messages == nil {
			response["data"] = map[string]any{"messages": []any{}}
		} else {
			response["data"] = map[string]any{"messages": messages}
		}
	case "get_last_assistant_text":
		text, err := s.GetLastAssistantText()
		if err != nil {
			fail(err)
			return
		}
		data := map[string]any{}
		if text != nil {
			data["text"] = *text
		}
		response["data"] = data
	case "extension_ui_response":
		fail(notImplemented("rpc.extension_ui_response"))
		return
	default:
		if strings.Contains("|steer|follow_up|new_session|set_model|cycle_model|get_available_models|set_thinking_level|cycle_thinking_level|get_available_thinking_levels|set_steering_mode|set_follow_up_mode|compact|set_auto_compaction|set_auto_retry|abort_retry|bash|abort_bash|get_session_stats|export_html|switch_session|fork|clone|get_fork_messages|get_entries|get_tree|set_session_name|get_commands|", "|"+command+"|") && command != "" {
			fail(notImplemented("rpc." + command))
			return
		}
		value, ok := fields["type"]
		name := "undefined"
		if ok {
			name = rpcJSString(value)
		}
		fail(fmt.Errorf("Unknown command: %s", name))
		return
	}
	output(response)
}

func rpcJSString(value any) string {
	switch value := value.(type) {
	case nil:
		return "null"
	case string:
		return value
	case map[string]any:
		return "[object Object]"
	case []any:
		parts := make([]string, len(value))
		for i, v := range value {
			if v != nil {
				parts[i] = rpcJSString(v)
			}
		}
		return strings.Join(parts, ",")
	default:
		return fmt.Sprint(value)
	}
}

func rpcTruthy(value any) bool {
	switch v := value.(type) {
	case nil:
		return false
	case bool:
		return v
	case string:
		return v != ""
	case float64:
		return v != 0
	default:
		return true
	}
}
