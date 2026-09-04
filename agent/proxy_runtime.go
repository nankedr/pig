package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/nankedr/pig/ai"
)

// ProxyMessageEventStream is the consumer-only result of StreamProxy.
type ProxyMessageEventStream struct {
	stream *ai.AssistantMessageEventStream
}

func (s *ProxyMessageEventStream) Next(ctx context.Context) (ai.AssistantMessageEvent, bool, error) {
	return s.stream.Next(ctx)
}
func (s *ProxyMessageEventStream) Result(ctx context.Context) (ai.AssistantMessage, error) {
	return s.stream.Result(ctx)
}

// AssistantMessageEventStream adapts the proxy to AgentOptions.StreamFunction.
func (s *ProxyMessageEventStream) AssistantMessageEventStream() *ai.AssistantMessageEventStream {
	return s.stream
}

func StreamProxy(ctx context.Context, model ai.Model, input ai.Context, options ProxyStreamOptions) *ProxyMessageEventStream {
	stream := &ProxyMessageEventStream{stream: ai.NewAssistantMessageEventStream()}
	partial := ai.AssistantMessage{Role: ai.MessageRoleAssistant, API: model.API, Provider: model.Provider, Model: model.ID, Content: []ai.AssistantContent{}, StopReason: ai.StopReasonPending, Timestamp: time.Now().UnixMilli()}
	payload, err := json.Marshal(struct {
		Model   ai.Model           `json:"model"`
		Context ai.Context         `json:"context"`
		Options ProxyStreamOptions `json:"options"`
	}{model, input, options})
	ctx, cancel := context.WithCancelCause(ctx)
	stopSignal := func() bool { return false }
	if options.Signal != nil {
		signal := options.Signal
		stopSignal = context.AfterFunc(signal, func() { cancel(context.Cause(signal)) })
		if signal.Err() != nil {
			cancel(context.Cause(signal))
		}
	}
	go func() {
		defer cancel(nil)
		defer stopSignal()
		producer := proxyReducer{message: partial}
		consumer := proxyReducer{message: partial}
		runErr := err
		if runErr == nil {
			runErr = stream.consume(ctx, options, payload, &producer, &consumer)
		}
		if runErr != nil {
			reason := ai.StopReasonError
			if ctx.Err() != nil {
				reason = ai.StopReasonAborted
				runErr = context.Cause(ctx)
			}
			producer.message.StopReason = reason
			message := runErr.Error()
			producer.message.ErrorMessage = ai.Some(message)
			stream.stream.Push(ai.AssistantMessageErrorEvent{Type: ai.AssistantMessageEventTypeError, Reason: reason, Error: producer.snapshot()})
		}
	}()
	return stream
}

func (s *ProxyMessageEventStream) consume(ctx context.Context, options ProxyStreamOptions, payload []byte, producer, consumer *proxyReducer) error {
	if err := context.Cause(ctx); err != nil {
		return err
	}
	request := ai.FetchRequest{URL: strings.TrimRight(options.ProxyURL, "/") + "/api/stream", Method: "POST", Headers: map[string]string{"Authorization": "Bearer " + options.AuthToken, "Content-Type": "application/json"}, Body: payload}
	fetch := options.Fetch
	if fetch == nil {
		fetch = fetchProxy
	}
	response, err := fetch(ctx, request)
	body := response.BodyReader
	if body == nil {
		body = io.NopCloser(bytes.NewReader(response.Body))
	}
	var closeOnce sync.Once
	closeBody := func() { closeOnce.Do(func() { _ = body.Close() }) }
	stop := context.AfterFunc(ctx, closeBody)
	defer closeBody()
	defer stop()
	if err != nil {
		return err
	}
	if err := context.Cause(ctx); err != nil {
		return err
	}
	if response.Status < 200 || response.Status >= 300 {
		message := fmt.Sprintf("Proxy error: %d %s", response.Status, http.StatusText(response.Status))
		var remote struct {
			Error string `json:"error"`
		}
		if json.NewDecoder(io.LimitReader(body, 1<<20)).Decode(&remote) == nil && remote.Error != "" {
			message = "Proxy error: " + remote.Error
		}
		return errors.New(message)
	}
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 4096), 16<<20)
	for scanner.Scan() {
		if err := context.Cause(ctx); err != nil {
			return err
		}
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		event, err := UnmarshalProxyAssistantMessageEvent([]byte(data))
		if err != nil {
			return fmt.Errorf("Proxy protocol error: %w", err)
		}
		if err = producer.apply(event); err != nil {
			return fmt.Errorf("Proxy protocol error: %w", err)
		}
		kind := event.ProxyAssistantMessageEventType()
		if kind == ai.AssistantMessageEventTypeDone || kind == ai.AssistantMessageEventTypeError {
			closeBody()
			s.stream.Push(producer.event(event))
			return nil
		}
		s.stream.PushLazy(func() ai.AssistantMessageEvent { _ = consumer.apply(event); return consumer.event(event) })
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("Proxy stream: %w", err)
	}
	return errors.New("Proxy protocol error: stream ended without a terminal event")
}

func fetchProxy(ctx context.Context, request ai.FetchRequest) (ai.FetchResponse, error) {
	req, err := http.NewRequestWithContext(ctx, request.Method, request.URL, bytes.NewReader(request.Body))
	if err != nil {
		return ai.FetchResponse{}, err
	}
	for k, v := range request.Headers {
		req.Header.Set(k, v)
	}
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		return ai.FetchResponse{}, err
	}
	return ai.FetchResponse{Status: response.StatusCode, BodyReader: response.Body}, nil
}

type proxyReducer struct {
	message   ai.AssistantMessage
	started   bool
	ended     []bool
	arguments []string
}

func (r *proxyReducer) apply(event ProxyAssistantMessageEvent) error {
	kind := event.ProxyAssistantMessageEventType()
	if kind == ai.AssistantMessageEventTypeStart {
		if r.started {
			return errors.New("duplicate start")
		}
		r.started = true
		return nil
	}
	switch e := event.(type) {
	case *ProxyDoneEvent:
		if !r.started {
			return errors.New("done before start")
		}
		r.message.StopReason = e.Reason
		r.message.Usage = e.Usage
		return nil
	case *ProxyErrorEvent:
		r.message.StopReason = e.Reason
		r.message.Usage = e.Usage
		if e.ErrorMessage != nil {
			r.message.ErrorMessage = ai.Some(*e.ErrorMessage)
		}
		return nil
	}
	if !r.started {
		return errors.New("content before start")
	}
	index := 0
	var block ai.AssistantContent
	switch e := event.(type) {
	case *ProxyTextStartEvent:
		index = e.ContentIndex
		block = ai.TextContent{Type: ai.ContentTypeText}
	case *ProxyThinkingStartEvent:
		index = e.ContentIndex
		block = ai.ThinkingContent{Type: ai.ContentTypeThinking}
	case *ProxyToolCallStartEvent:
		index = e.ContentIndex
		block = ai.ToolCall{Type: ai.ContentTypeToolCall, ID: e.ID, Name: e.ToolName, Arguments: map[string]any{}}
	case *ProxyTextDeltaEvent:
		index = e.ContentIndex
	case *ProxyTextEndEvent:
		index = e.ContentIndex
	case *ProxyThinkingDeltaEvent:
		index = e.ContentIndex
	case *ProxyThinkingEndEvent:
		index = e.ContentIndex
	case *ProxyToolCallDeltaEvent:
		index = e.ContentIndex
	case *ProxyToolCallEndEvent:
		index = e.ContentIndex
	}
	if block != nil {
		if index != len(r.message.Content) {
			return fmt.Errorf("invalid start index %d", index)
		}
		r.message.Content = append(r.message.Content, block)
		r.ended = append(r.ended, false)
		r.arguments = append(r.arguments, "")
		return nil
	}
	if index < 0 || index >= len(r.message.Content) || r.ended[index] {
		return fmt.Errorf("invalid content index %d for %s", index, kind)
	}
	block = r.message.Content[index]
	switch e := event.(type) {
	case *ProxyTextDeltaEvent:
		b, ok := block.(ai.TextContent)
		if !ok {
			return errors.New("text_delta for non-text content")
		}
		b.Text += e.Delta
		block = b
	case *ProxyTextEndEvent:
		b, ok := block.(ai.TextContent)
		if !ok {
			return errors.New("text_end for non-text content")
		}
		if e.ContentSignature != nil {
			b.TextSignature = ai.Some(*e.ContentSignature)
		}
		block = b
		r.ended[index] = true
	case *ProxyThinkingDeltaEvent:
		b, ok := block.(ai.ThinkingContent)
		if !ok {
			return errors.New("thinking_delta for non-thinking content")
		}
		b.Thinking += e.Delta
		block = b
	case *ProxyThinkingEndEvent:
		b, ok := block.(ai.ThinkingContent)
		if !ok {
			return errors.New("thinking_end for non-thinking content")
		}
		if e.ContentSignature != nil {
			b.ThinkingSignature = ai.Some(*e.ContentSignature)
		}
		block = b
		r.ended[index] = true
	case *ProxyToolCallDeltaEvent:
		if _, ok := block.(ai.ToolCall); !ok {
			return errors.New("toolcall_delta for non-tool content")
		}
		r.arguments[index] += e.Delta
	case *ProxyToolCallEndEvent:
		b, ok := block.(ai.ToolCall)
		if !ok {
			return errors.New("toolcall_end for non-tool content")
		}
		if b.ID != e.ToolCall.ID || b.Name != e.ToolCall.Name {
			return errors.New("toolcall_end identity mismatch")
		}
		block = e.ToolCall
		r.ended[index] = true
	}
	r.message.Content[index] = block
	return nil
}

func (r *proxyReducer) snapshot() ai.AssistantMessage {
	message := ai.CloneAssistantMessage(r.message)
	for i, block := range message.Content {
		if tool, ok := block.(ai.ToolCall); ok && !r.ended[i] {
			tool.Arguments = ai.ParseStreamingJSONObject(r.arguments[i])
			message.Content[i] = tool
		}
	}
	return message
}

func (r *proxyReducer) event(event ProxyAssistantMessageEvent) ai.AssistantMessageEvent {
	partial := r.snapshot()
	kind := event.ProxyAssistantMessageEventType()
	switch e := event.(type) {
	case *ProxyStartEvent:
		return ai.AssistantMessageStartEvent{Type: kind, Partial: partial}
	case *ProxyTextStartEvent:
		return ai.AssistantMessageTextStartEvent{Type: kind, ContentIndex: e.ContentIndex, Partial: partial}
	case *ProxyTextDeltaEvent:
		return ai.AssistantMessageTextDeltaEvent{Type: kind, ContentIndex: e.ContentIndex, Delta: e.Delta, Partial: partial}
	case *ProxyTextEndEvent:
		return ai.AssistantMessageTextEndEvent{Type: kind, ContentIndex: e.ContentIndex, Content: partial.Content[e.ContentIndex].(ai.TextContent).Text, Partial: partial}
	case *ProxyThinkingStartEvent:
		return ai.AssistantMessageThinkingStartEvent{Type: kind, ContentIndex: e.ContentIndex, Partial: partial}
	case *ProxyThinkingDeltaEvent:
		return ai.AssistantMessageThinkingDeltaEvent{Type: kind, ContentIndex: e.ContentIndex, Delta: e.Delta, Partial: partial}
	case *ProxyThinkingEndEvent:
		return ai.AssistantMessageThinkingEndEvent{Type: kind, ContentIndex: e.ContentIndex, Content: partial.Content[e.ContentIndex].(ai.ThinkingContent).Thinking, Partial: partial}
	case *ProxyToolCallStartEvent:
		return ai.AssistantMessageToolCallStartEvent{Type: kind, ContentIndex: e.ContentIndex, Partial: partial}
	case *ProxyToolCallDeltaEvent:
		return ai.AssistantMessageToolCallDeltaEvent{Type: kind, ContentIndex: e.ContentIndex, Delta: e.Delta, Partial: partial}
	case *ProxyToolCallEndEvent:
		return ai.AssistantMessageToolCallEndEvent{Type: kind, ContentIndex: e.ContentIndex, ToolCall: partial.Content[e.ContentIndex].(ai.ToolCall), Partial: partial}
	case *ProxyDoneEvent:
		return ai.AssistantMessageDoneEvent{Type: kind, Reason: e.Reason, Message: partial}
	case *ProxyErrorEvent:
		return ai.AssistantMessageErrorEvent{Type: kind, Reason: e.Reason, Error: partial}
	}
	panic("validated proxy event missing projection")
}
