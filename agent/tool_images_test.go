package agent_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
)

func TestToolImagesParallelUpdatesFinalsAndCancellation(t *testing.T) {
	for _, cancelRun := range []bool{false, true} {
		t.Run(map[bool]string{false: "settle", true: "cancel"}[cancelRun], func(t *testing.T) {
			image, err := ai.LoadImageFile("../parity/services/user-image.png")
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			started := make(chan struct{}, 2)
			release := make(chan struct{})
			var executions atomic.Int32
			tool, err := agent.EraseAgentTool(agent.AgentTool[map[string]any, any]{Tool: ai.Tool{Name: "lookup", Parameters: json.RawMessage(`{"type":"object"}`)}, DecodeValidated: func(v ai.JSONValue) map[string]any { return v.(map[string]any) }, Execute: func(ctx context.Context, id string, _ map[string]any, update agent.AgentToolUpdateCallback[any]) (agent.AgentToolResult[any], error) {
				executions.Add(1)
				content := []ai.ToolResultContent{ai.TextContent{Type: ai.ContentTypeText, Text: id}, image, ai.TextContent{Type: ai.ContentTypeText, Text: "tail"}}
				update(agent.AgentToolResult[any]{Content: content})
				started <- struct{}{}
				select {
				case <-release:
				case <-ctx.Done():
					return agent.AgentToolResult[any]{}, ctx.Err()
				}
				return agent.AgentToolResult[any]{Content: content}, nil
			}})
			if err != nil {
				t.Fatal(err)
			}
			model := ai.DeepSeekVisionModel()
			model.API = ai.APIOpenAIResponses
			model.Compat = ai.Absent[json.RawMessage]()
			key := "offline"
			var requests atomic.Int32
			stream := func(ctx context.Context, m ai.Model, input ai.Context, o ai.SimpleStreamOptions) *ai.AssistantMessageEventStream {
				o.APIKey = &key
				cache := ai.CacheRetentionNone
				o.CacheRetention = &cache
				o.Fetch = func(_ context.Context, r ai.FetchRequest) (ai.FetchResponse, error) {
					n := requests.Add(1)
					sse := ""
					if n == 1 {
						for i, id := range []string{"a", "b"} {
							item := responsesToolItem(id, "{}")
							sse += responsesFrame(map[string]any{"type": "response.output_item.added", "output_index": i, "item": item}) + responsesFrame(map[string]any{"type": "response.output_item.done", "output_index": i, "item": item})
						}
					} else {
						var body struct{ Input []map[string]any }
						json.Unmarshal(r.Body, &body)
						ids := []string{}
						for _, item := range body.Input {
							if item["type"] == "function_call_output" {
								ids = append(ids, item["call_id"].(string))
								output := item["output"].([]any)
								if output[0].(map[string]any)["text"] != item["call_id"].(string)+"|item-"+item["call_id"].(string)+"\ntail" || output[1].(map[string]any)["image_url"] != "data:image/png;base64,"+image.Data {
									t.Error("final image/text linkage lost")
								}
							}
						}
						if !reflect.DeepEqual(ids, []string{"a", "b"}) {
							t.Errorf("parallel source order=%v", ids)
						}
					}
					sse += responsesFrame(map[string]any{"type": "response.completed", "response": map[string]any{"status": "completed"}})
					return ai.FetchResponse{Status: 200, Headers: map[string]string{"Content-Type": "text/event-stream"}, Body: []byte(sse)}, nil
				}
				return ai.StreamSimpleOpenAIResponses(ctx, m, input, o)
			}
			a, err := agent.NewAgent(agent.AgentOptions{InitialState: &agent.AgentInitialState{Model: model, Tools: []agent.ErasedAgentTool{tool}}, StreamFunction: stream, ToolExecution: agent.ToolExecutionParallel})
			if err != nil {
				t.Fatal(err)
			}
			updates, ends := map[string]bool{}, map[string]bool{}
			a.Subscribe(func(_ context.Context, e agent.AgentEvent) error {
				switch e := e.(type) {
				case agent.ToolExecutionUpdateEvent:
					if len(e.PartialResult.Content) != 3 || e.PartialResult.Content[1].(ai.ImageContent) != image {
						t.Error("update image lost")
					}
					updates[e.ToolCallID] = true
				case agent.ToolExecutionEndEvent:
					if !updates[e.ToolCallID] || ends[e.ToolCallID] {
						t.Error("update/end barrier")
					}
					ends[e.ToolCallID] = true
					if !cancelRun && (e.IsError || len(e.Result.Content) != 3 || e.Result.Content[1].(ai.ImageContent) != image) {
						t.Error("terminal image lost")
					}
				}
				return nil
			})
			done := make(chan error, 1)
			go func() { done <- a.PromptText(ctx, "read screenshot") }()
			for range 2 {
				select {
				case <-started:
				case err := <-done:
					t.Fatalf("agent ended before tools: %v", err)
				case <-ctx.Done():
					t.Fatal("tools did not execute in parallel")
				}
			}
			if cancelRun {
				cancel()
			} else {
				close(release)
			}
			err = <-done
			if cancelRun && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancel=%v", err)
			}
			if !cancelRun && err != nil {
				t.Fatal(err)
			}
			if executions.Load() != 2 || len(updates) != 2 || len(ends) != 2 {
				t.Fatal("unsettled tool lifecycle")
			}
			transcript, _ := json.Marshal(a.State().Messages)
			if !cancelRun && strings.Count(string(transcript), image.Data) != 2 {
				t.Fatal("history did not retain two final images")
			}
		})
	}
}
