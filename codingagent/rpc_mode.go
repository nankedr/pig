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

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
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
	var binding sync.Mutex
	var listenerID uint64
	var unsubscribe func()
	writer := &jsonLineWriter{output: stdout}
	outgoing := ai.NewEventStream(func(any) bool { return false }, func(any) struct{} { return struct{}{} })
	written := make(chan struct{})
	go func() {
		defer close(written)
		for {
			value, ok, _ := outgoing.Next(context.Background())
			if !ok {
				return
			}
			writer.Write(value)
			if err := writer.Err(); err != nil {
				cancel(err)
				return
			}
		}
	}()
	output := func(value any) { outgoing.Push(value) }
	bind := func(next *AgentSession) error {
		binding.Lock()
		defer binding.Unlock()
		stop, id, err := next.subscribe(func(event AgentSessionEvent) {
			binding.Lock()
			defer binding.Unlock()
			if session != next {
				return
			}
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
		if unsubscribe != nil {
			unsubscribe()
		}
		session, listenerID, unsubscribe = next, id, stop
		return nil
	}
	if err := bind(session); err != nil {
		outgoing.End(struct{}{})
		<-written
		return err
	}
	runtime.SetRebindSession(bind)
	defer func() { unsubscribe(); runtime.SetRebindSession(nil) }()
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
		binding.Lock()
		current := session
		binding.Unlock()
		_ = current.Abort()
		_ = stdout.SetWriteDeadline(time.Now().Add(time.Second))
		commands.Wait()
		outgoing.End(struct{}{})
		<-written
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
			binding.Lock()
			current, id := session, listenerID
			binding.Unlock()
			commandOutput := func(value any) {
				if event, ok := value.(map[string]any); ok && event["type"] != "response" {
					binding.Lock()
					defer binding.Unlock()
					if session != current {
						return
					}
				}
				output(value)
			}
			switch fields["type"] {
			case "steer", "follow_up", "set_steering_mode", "set_follow_up_mode", "set_thinking_level", "cycle_thinking_level", "set_auto_retry", "abort_retry", "abort_bash", "get_state", "get_messages", "get_last_assistant_text", "get_session_stats", "get_available_models", "get_available_thinking_levels", "set_auto_compaction", "set_session_name", "get_entries", "get_tree", "get_fork_messages":
				rpcCommand(ctx, current, fields, commandOutput, id, runtime)
			default:
				commands.Add(1)
				go func() { defer commands.Done(); rpcCommand(ctx, current, fields, commandOutput, id, runtime) }()
			}
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

func rpcCommand(ctx context.Context, s *AgentSession, fields map[string]any, output func(any), listenerID uint64, runtime *AgentSessionRuntime) {
	response := map[string]any{"type": "response", "success": true}
	if value, ok := fields["id"]; ok {
		response["id"] = value
	}
	if value, ok := fields["type"]; ok {
		response["command"] = value
	}
	fail := func(err error) { response["success"] = false; response["error"] = err.Error(); output(response) }
	command, _ := fields["type"].(string)
	var err error
	switch command {
	case "prompt", "steer", "follow_up":
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
		if command == "steer" || command == "follow_up" {
			if command == "steer" {
				err = s.Steer(text)
			} else {
				err = s.FollowUp(text)
			}
			if err != nil {
				fail(err)
				return
			}
			output(response)
			return
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
	case "set_steering_mode", "set_follow_up_mode":
		mode, _ := fields["mode"].(string)
		if command == "set_steering_mode" {
			err = s.SetSteeringMode(agent.QueueMode(mode))
		} else {
			err = s.SetFollowUpMode(agent.QueueMode(mode))
		}
	case "set_model":
		provider, modelID := "undefined", "undefined"
		if value, ok := fields["provider"]; ok {
			provider = rpcJSString(value)
		}
		if value, ok := fields["modelId"]; ok {
			modelID = rpcJSString(value)
		}
		models, listErr := s.ModelRuntime().GetAvailableSnapshot()
		if listErr != nil {
			fail(listErr)
			return
		}
		err = fmt.Errorf("Model not found: %s/%s", provider, modelID)
		for _, model := range models {
			if fields["provider"] == string(model.Provider) && fields["modelId"] == model.ID {
				err = s.SetModel(model)
				response["data"] = model
				break
			}
		}
	case "get_available_models":
		models, listErr := s.ModelRuntime().GetAvailableSnapshot()
		err = listErr
		if models == nil {
			models = []ai.Model{}
		}
		response["data"] = map[string]any{"models": models}
	case "cycle_model":
		result, cycleErr := s.CycleModel(ctx)
		err = cycleErr
		response["data"] = nil
		if result != nil {
			response["data"] = map[string]any{"model": result.Model, "thinkingLevel": result.ThinkingLevel, "isScoped": result.IsScoped}
		}
	case "set_thinking_level":
		level, _ := fields["level"].(string)
		err = s.SetThinkingLevel(agent.ThinkingLevel(level))
	case "cycle_thinking_level":
		level, cycleErr := s.CycleThinkingLevel()
		err = cycleErr
		response["data"] = nil
		if level != "" {
			response["data"] = map[string]any{"level": level}
		}
	case "get_available_thinking_levels":
		levels, levelsErr := s.GetAvailableThinkingLevels()
		err = levelsErr
		response["data"] = map[string]any{"levels": levels}
	case "set_auto_retry":
		err = s.SetAutoRetryEnabled(rpcTruthy(fields["enabled"]))
	case "abort_retry":
		err = s.AbortRetry()
	case "bash":
		text, ok := fields["command"].(string)
		if !ok {
			fail(errors.New("Bash command must be a string"))
			return
		}
		var id *string
		if raw, ok := fields["id"].(json.RawMessage); ok {
			if json.Unmarshal(raw, &id) != nil {
				id = nil
			}
		}
		result, bashErr := s.executeBash(ctx, text, ExecuteBashOptions{ID: id, ExcludeFromContext: rpcTruthy(fields["excludeFromContext"])}, func(delta string) {
			event := map[string]any{"type": "bash_execution_update", "delta": delta}
			if id, ok := fields["id"]; ok {
				event["id"] = id
			}
			output(event)
		}, &listenerID)
		err = bashErr
		data := map[string]any{"output": result.Output, "cancelled": result.Cancelled, "truncated": result.Truncated}
		if result.ExitCode != nil {
			data["exitCode"] = *result.ExitCode
		}
		if result.FullOutputPath != nil {
			data["fullOutputPath"] = *result.FullOutputPath
		}
		response["data"] = data
	case "abort_bash":
		err = s.AbortBash()
	case "get_session_stats":
		stats, statsErr := s.GetSessionStats()
		err = statsErr
		data := map[string]any{"sessionId": stats.SessionID, "userMessages": stats.UserMessages, "assistantMessages": stats.AssistantMessages, "toolCalls": stats.ToolCalls, "toolResults": stats.ToolResults, "totalMessages": stats.TotalMessages, "cost": stats.Cost, "contextUsage": stats.ContextUsage, "tokens": map[string]any{"input": stats.Tokens.Input, "output": stats.Tokens.Output, "cacheRead": stats.Tokens.CacheRead, "cacheWrite": stats.Tokens.CacheWrite, "total": stats.Tokens.TotalTokens}}
		if stats.SessionFile != nil {
			data["sessionFile"] = *stats.SessionFile
		}
		if stats.ContextUsage != nil {
			data["contextUsage"] = map[string]any{"tokens": stats.ContextUsage.Tokens, "contextWindow": stats.ContextUsage.ContextWindow, "percent": stats.ContextUsage.Percent}
		}
		response["data"] = data
	case "compact":
		instructions, _ := fields["customInstructions"].(string)
		response["data"], err = s.Compact(ctx, instructions)
	case "set_auto_compaction":
		err = s.SetAutoCompactionEnabled(rpcTruthy(fields["enabled"]))
	case "new_session", "switch_session", "fork", "clone":
		var result SessionReplacementResult
		switch command {
		case "new_session":
			var options NewRuntimeSessionOptions
			if parent, ok := fields["parentSession"].(string); ok && parent != "" {
				options.ParentSession = &parent
			}
			result, err = runtime.NewSession(ctx, options)
		case "switch_session":
			path, _ := fields["sessionPath"].(string)
			if path == "" {
				err = errors.New("Session path must be a non-empty string")
			} else {
				result, err = runtime.SwitchSession(ctx, path)
			}
		case "fork":
			id, _ := fields["entryId"].(string)
			result, err = runtime.Fork(ctx, id)
		case "clone":
			result, err = runtime.fork(ctx, "", true)

		}
		data := map[string]any{"cancelled": result.Cancelled}
		if command == "fork" && result.SelectedText != nil {
			data["text"] = *result.SelectedText
		}
		response["data"] = data
	case "get_fork_messages":
		messages, listErr := s.GetUserMessagesForForking()
		err = listErr
		data := make([]map[string]string, len(messages))
		for i, message := range messages {
			data[i] = map[string]string{"entryId": message.EntryID, "text": message.Text}
		}
		response["data"] = map[string]any{"messages": data}
	case "get_entries":
		var since *string
		if value, exists := fields["since"]; exists {
			text, ok := value.(string)
			if !ok {
				fail(fmt.Errorf("Entry not found: %s", rpcJSString(value)))
				return
			}
			since = &text
		}
		entries, leaf, listErr := s.SessionManager().getEntriesSince(since)
		if listErr != nil {
			fail(listErr)
			return
		}
		data := make([]json.RawMessage, len(entries))
		for i, entry := range entries {
			data[i], err = marshalSessionEntry(entry)
			if err != nil {
				fail(err)
				return
			}
		}
		response["data"] = map[string]any{"entries": data, "leafId": leaf}
	case "get_tree":
		entries, leaf, listErr := s.SessionManager().getEntriesSince(nil)
		err = listErr
		var data []rpcTreeNode
		if err == nil {
			data, err = encodeRPCTree(buildSessionTree(entries))
		}
		response["data"] = map[string]any{"tree": data, "leafId": leaf}
	case "set_session_name":
		name, ok := fields["name"].(string)
		if !ok {
			err = errors.New("Session name must be a string")
		} else if name = strings.TrimSpace(name); name == "" {
			err = errors.New("Session name cannot be empty")
		} else {
			err = s.SetSessionName(name)
		}
	case "export_html":
		path, ok := fields["outputPath"].(string)
		if value := fields["outputPath"]; value != nil && !ok {
			fail(fmt.Errorf("outputPath must be a string"))
			return
		}
		var exported string
		exported, err = s.ExportToHTML(ctx, path)
		if err == nil {
			response["data"] = map[string]any{"path": exported}
		}
	case "extension_ui_response":
		fail(notImplemented("rpc.extension_ui_response"))
		return
	default:
		if strings.Contains("|get_commands|", "|"+command+"|") && command != "" {
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
	if err != nil {
		delete(response, "data")
		fail(err)
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
