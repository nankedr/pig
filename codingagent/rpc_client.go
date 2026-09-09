package codingagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
)

type rpcResult struct {
	response RPCResponse
	err      error
}
type rpcRequest struct {
	id   string
	data []byte
}
type rpcSubscription struct {
	stream *ai.EventStream[json.RawMessage, struct{}]
	ctx    context.Context
	cancel context.CancelFunc
}
type rpcProcess struct {
	cmd      *exec.Cmd
	input    io.WriteCloser
	done     chan struct{}
	err      error
	pending  map[string]chan rpcResult
	outgoing *ai.EventStream[rpcRequest, struct{}]
}

// RPCClient owns one JSONL subprocess at a time. Request contexts cancel local
// waiters; Abort cancels the remote run. Event callbacks run off the pipe reader.
type RPCClient struct {
	options       RPCClientOptions
	initialized   bool
	mu            sync.Mutex
	process       *rpcProcess
	next          uint64
	stderr        strings.Builder
	subscriptions map[*rpcSubscription]struct{}
}

func NewRPCClient(options ...RPCClientOptions) *RPCClient {
	c := &RPCClient{initialized: true, subscriptions: make(map[*rpcSubscription]struct{})}
	if len(options) > 0 {
		c.options = options[0]
	}
	c.options.Args = append([]string(nil), c.options.Args...)
	c.options.CLIPath = cloneStringPointer(c.options.CLIPath)
	c.options.CWD = cloneStringPointer(c.options.CWD)
	c.options.Provider = cloneStringPointer(c.options.Provider)
	c.options.Model = cloneStringPointer(c.options.Model)
	env := make(map[string]string, len(c.options.Env))
	for k, v := range c.options.Env {
		env[k] = v
	}
	c.options.Env = env
	return c
}

func (c *RPCClient) Start(ctx context.Context) (err error) {
	if c == nil || !c.initialized {
		return notImplemented("RPCClient.Start")
	}
	defer func() {
		if err != nil {
			c.mu.Lock()
			if c.process == nil {
				c.endRPCSubscriptionsLocked()
			}
			c.mu.Unlock()
		}
	}()
	if ctx == nil {
		return errors.New("RPC Start context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	c.mu.Lock()
	if c.process != nil {
		c.mu.Unlock()
		return errors.New("Client already started")
	}
	path := "pig"
	if c.options.CLIPath != nil {
		path = *c.options.CLIPath
	}
	args := []string{"--mode", "rpc"}
	if c.options.Provider != nil {
		args = append(args, "--provider", *c.options.Provider)
	}
	if c.options.Model != nil {
		args = append(args, "--model", *c.options.Model)
	}
	args = append(args, c.options.Args...)
	cmd := exec.Command(path, args...)
	if c.options.CWD != nil {
		cmd.Dir = *c.options.CWD
	}
	cmd.Env = os.Environ()
	for k, v := range c.options.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	input, err := cmd.StdinPipe()
	if err != nil {
		c.mu.Unlock()
		return err
	}
	output, outputWriter, err := os.Pipe()
	if err != nil {
		input.Close()
		c.mu.Unlock()
		return err
	}
	stderr, stderrWriter, err := os.Pipe()
	if err != nil {
		input.Close()
		output.Close()
		outputWriter.Close()
		c.mu.Unlock()
		return err
	}
	cmd.Stdout, cmd.Stderr = outputWriter, stderrWriter
	configureRPCProcess(cmd)
	if err = cmd.Start(); err != nil {
		input.Close()
		output.Close()
		stderr.Close()
		outputWriter.Close()
		stderrWriter.Close()
		c.endRPCSubscriptionsLocked()
		c.mu.Unlock()
		return fmt.Errorf("Agent process error: %w", err)
	}
	outputWriter.Close()
	stderrWriter.Close()
	p := &rpcProcess{cmd: cmd, input: input, done: make(chan struct{}), pending: make(map[string]chan rpcResult), outgoing: ai.NewEventStream(func(rpcRequest) bool { return false }, func(rpcRequest) struct{} { return struct{}{} })}
	c.stderr.Reset()
	c.process = p
	c.mu.Unlock()
	var readers sync.WaitGroup
	readers.Add(2)
	go func() {
		defer readers.Done()
		defer stderr.Close()
		buf := make([]byte, 4096)
		for {
			n, err := stderr.Read(buf)
			if n > 0 {
				c.mu.Lock()
				c.stderr.Write(buf[:n])
				c.mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	go func() {
		defer readers.Done()
		defer output.Close()
		err := readRPCLines(output, func(line string) bool { c.handleRPCLine(p, []byte(line)); return true })
		if err == nil {
			err = io.EOF
		}
		c.failRPC(p, fmt.Errorf("Agent process stdout: %w", err))
		killRPCProcess(cmd.Process)
	}()
	written := make(chan struct{})
	go func() {
		defer close(written)
		for {
			request, ok, _ := p.outgoing.Next(context.Background())
			if !ok {
				return
			}
			c.mu.Lock()
			_, pending := p.pending[request.id]
			failed := p.err != nil
			c.mu.Unlock()
			if failed {
				return
			}
			if !pending {
				continue
			}
			n, err := input.Write(request.data)
			if err == nil && n != len(request.data) {
				err = io.ErrShortWrite
			}
			if err != nil {
				c.failRPC(p, fmt.Errorf("Agent process stdin: %w", err))
				killRPCProcess(cmd.Process)
				return
			}
		}
	}()
	go func() {
		err := cmd.Wait()
		killRPCProcess(cmd.Process)
		_ = input.Close()
		readers.Wait()
		c.mu.Lock()
		details := c.stderr.String()
		c.mu.Unlock()
		c.failRPC(p, fmt.Errorf("Agent process exited: %v. Stderr: %s", err, details))
		_ = input.Close()
		p.outgoing.End(struct{}{})
		<-written
		close(p.done)
	}()
	if _, err := c.GetState(ctx); err != nil {
		_ = c.Stop(context.Background())
		details, _ := c.GetStderr()
		return fmt.Errorf("%w. Stderr: %s", err, details)
	}
	return nil
}

func (c *RPCClient) failRPC(p *rpcProcess, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if p.err != nil {
		return
	}
	p.err = err
	for id, waiter := range p.pending {
		waiter <- rpcResult{err: err}
		delete(p.pending, id)
	}
	c.endRPCSubscriptionsLocked()
	p.outgoing.End(struct{}{})
}

func (c *RPCClient) Stop(ctx context.Context) error {
	if c == nil || !c.initialized {
		return notImplemented("RPCClient.Stop")
	}
	if ctx == nil {
		return errors.New("RPC Stop context must not be nil")
	}
	c.mu.Lock()
	p := c.process
	if p == nil {
		c.endRPCSubscriptionsLocked()
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()
	c.failRPC(p, errors.New("RPC client stopped"))
	_ = p.input.Close()
	_ = p.cmd.Process.Signal(syscall.SIGTERM)
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	var err error
	select {
	case <-p.done:
	case <-ctx.Done():
		err = context.Cause(ctx)
		killRPCProcess(p.cmd.Process)
		<-p.done
	case <-timer.C:
		killRPCProcess(p.cmd.Process)
		<-p.done
	}
	c.mu.Lock()
	if c.process == p {
		c.process = nil
	}
	c.mu.Unlock()
	return err
}

func (c *RPCClient) sendRPC(ctx context.Context, command map[string]any) (RPCResponse, error) {
	if ctx == nil {
		return RPCResponse{}, errors.New("RPC request context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return RPCResponse{}, err
	}
	c.mu.Lock()
	p := c.process
	if p == nil {
		c.mu.Unlock()
		return RPCResponse{}, errors.New("Client not started")
	}
	if p.err != nil {
		err := p.err
		c.mu.Unlock()
		return RPCResponse{}, err
	}
	c.next++
	id := "req_" + strconv.FormatUint(c.next, 10)
	command["id"] = id
	data, err := json.Marshal(command)
	if err != nil {
		c.mu.Unlock()
		return RPCResponse{}, err
	}
	waiter := make(chan rpcResult, 1)
	p.pending[id] = waiter
	p.outgoing.Push(rpcRequest{id: id, data: append(data, '\n')})
	c.mu.Unlock()
	defer func() { c.mu.Lock(); delete(p.pending, id); c.mu.Unlock() }()
	select {
	case result := <-waiter:
		if result.err != nil {
			return RPCResponse{}, result.err
		}
		if !result.response.Success {
			if result.response.Error != nil {
				return result.response, errors.New(*result.response.Error)
			}
			return result.response, errors.New("RPC command failed")
		}
		return result.response, nil
	case <-ctx.Done():
		return RPCResponse{}, context.Cause(ctx)
	}
}

func (c *RPCClient) handleRPCLine(p *rpcProcess, line []byte) {
	var envelope struct {
		Type string
		ID   *string
	}
	if json.Unmarshal(line, &envelope) != nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.process != p || p.err != nil {
		return
	}
	if envelope.Type == "response" {
		if envelope.ID != nil {
			if waiter := p.pending[*envelope.ID]; waiter != nil {
				var response RPCResponse
				if err := json.Unmarshal(line, &response); err != nil {
					waiter <- rpcResult{err: err}
				} else {
					waiter <- rpcResult{response: response}
				}
				delete(p.pending, *envelope.ID)
			}
		}
		return
	}
	if _, err := decodeRPCEvent(line); err != nil {
		return
	}
	for sub := range c.subscriptions {
		sub.stream.Push(append(json.RawMessage(nil), line...))
	}
}

func (c *RPCClient) subscribeRPC() (*rpcSubscription, func(), error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.process != nil && c.process.err != nil {
		return nil, nil, c.process.err
	}
	ctx, cancel := context.WithCancel(context.Background())
	sub := &rpcSubscription{stream: ai.NewEventStream(func(json.RawMessage) bool { return false }, func(json.RawMessage) struct{} { return struct{}{} }), ctx: ctx, cancel: cancel}
	c.subscriptions[sub] = struct{}{}
	return sub, func() {
		c.mu.Lock()
		delete(c.subscriptions, sub)
		sub.cancel()
		sub.stream.End(struct{}{})
		c.mu.Unlock()
	}, nil
}
func (c *RPCClient) OnEvent(listener RPCEventListener) (func(), error) {
	if c == nil || !c.initialized {
		return nil, notImplemented("RPCClient.OnEvent")
	}
	if listener == nil {
		return func() {}, nil
	}
	sub, unsubscribe, err := c.subscribeRPC()
	if err != nil {
		return nil, err
	}
	go func() {
		defer unsubscribe()
		for {
			raw, ok, err := sub.stream.Next(sub.ctx)
			if err != nil || !ok || sub.ctx.Err() != nil {
				return
			}
			event, err := decodeRPCEvent(raw)
			if err == nil {
				listener(event)
			}
		}
	}()
	return unsubscribe, nil
}
func (c *RPCClient) collectRPC(ctx context.Context, sub *rpcSubscription) ([]JSONAgentSessionEvent, error) {
	events := []JSONAgentSessionEvent{}
	for {
		raw, ok, err := sub.stream.Next(ctx)
		if err != nil {
			return events, err
		}
		if !ok {
			c.mu.Lock()
			err = errors.New("RPC client disconnected")
			if c.process != nil && c.process.err != nil {
				err = c.process.err
			}
			c.mu.Unlock()
			return events, err
		}
		event, err := decodeRPCEvent(raw)
		if err != nil {
			return events, err
		}
		events = append(events, event)
		if event.AgentSessionEventType() == AgentSessionEventTypeAgentSettled {
			return events, nil
		}
	}
}
func (c *RPCClient) CollectEvents(ctx context.Context) ([]JSONAgentSessionEvent, error) {
	if c == nil || !c.initialized {
		return nil, notImplemented("RPCClient.CollectEvents")
	}
	if ctx == nil {
		return nil, errors.New("RPC collect context must not be nil")
	}
	sub, unsubscribe, err := c.subscribeRPC()
	if err != nil {
		return nil, err
	}
	defer unsubscribe()
	return c.collectRPC(ctx, sub)
}
func (c *RPCClient) PromptAndWait(ctx context.Context, text string, images ...[]ai.ImageContent) ([]JSONAgentSessionEvent, error) {
	if c == nil || !c.initialized {
		return nil, notImplemented("RPCClient.PromptAndWait")
	}
	sub, unsubscribe, err := c.subscribeRPC()
	if err != nil {
		return nil, err
	}
	defer unsubscribe()
	if err = c.Prompt(ctx, text, images...); err != nil {
		return nil, err
	}
	return c.collectRPC(ctx, sub)
}
func (c *RPCClient) WaitForIdle(ctx context.Context) error {
	if c == nil || !c.initialized {
		return notImplemented("RPCClient.WaitForIdle")
	}
	sub, unsubscribe, err := c.subscribeRPC()
	if err != nil {
		return err
	}
	defer unsubscribe()
	state, err := c.GetState(ctx)
	if err != nil {
		return err
	}
	if !state.IsStreaming && !state.IsCompacting {
		return nil
	}
	_, err = c.collectRPC(ctx, sub)
	return err
}
func (c *RPCClient) Prompt(ctx context.Context, text string, images ...[]ai.ImageContent) error {
	if c == nil || !c.initialized {
		return notImplemented("RPCClient.Prompt")
	}
	command := map[string]any{"type": "prompt", "message": text}
	if len(images) > 0 {
		command["images"] = images[0]
	}
	_, err := c.sendRPC(ctx, command)
	return err
}
func (c *RPCClient) Abort(ctx context.Context) error {
	if c == nil || !c.initialized {
		return notImplemented("RPCClient.Abort")
	}
	_, err := c.sendRPC(ctx, map[string]any{"type": "abort"})
	return err
}
func (c *RPCClient) GetState(ctx context.Context) (RPCSessionState, error) {
	if c == nil || !c.initialized {
		return RPCSessionState{}, notImplemented("RPCClient.GetState")
	}
	response, err := c.sendRPC(ctx, map[string]any{"type": "get_state"})
	if err != nil {
		return RPCSessionState{}, err
	}
	var state RPCSessionState
	err = json.Unmarshal(response.Data, &state)
	return state, err
}
func (c *RPCClient) GetMessages(ctx context.Context) ([]agent.AgentMessage, error) {
	if c == nil || !c.initialized {
		return nil, notImplemented("RPCClient.GetMessages")
	}
	response, err := c.sendRPC(ctx, map[string]any{"type": "get_messages"})
	if err != nil {
		return nil, err
	}
	var data struct{ Messages []json.RawMessage }
	if err = json.Unmarshal(response.Data, &data); err != nil {
		return nil, err
	}
	messages := make([]agent.AgentMessage, len(data.Messages))
	for i, raw := range data.Messages {
		messages[i], err = unmarshalSessionAgentMessage(raw)
		if err != nil {
			return nil, err
		}
	}
	return messages, nil
}
func (c *RPCClient) GetLastAssistantText(ctx context.Context) (*string, error) {
	if c == nil || !c.initialized {
		return nil, notImplemented("RPCClient.GetLastAssistantText")
	}
	response, err := c.sendRPC(ctx, map[string]any{"type": "get_last_assistant_text"})
	if err != nil {
		return nil, err
	}
	var data struct{ Text *string }
	err = json.Unmarshal(response.Data, &data)
	return data.Text, err
}
func (c *RPCClient) GetStderr() (string, error) {
	if c == nil || !c.initialized {
		return "", notImplemented("RPCClient.GetStderr")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stderr.String(), nil
}

func (c *RPCClient) endRPCSubscriptionsLocked() {
	for sub := range c.subscriptions {
		sub.cancel()
		sub.stream.End(struct{}{})
		delete(c.subscriptions, sub)
	}
}
