package agent_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
)

func TestNewAgentHarnessOpensOnlyRecordFreeSessions(t *testing.T) {
	ctx := context.Background()
	repo := agent.NewInMemorySessionRepo()
	session, err := repo.Create(ctx, agent.SessionCreateOptions{ID: "empty"})
	if err != nil {
		t.Fatalf("create empty session: %v", err)
	}

	harness, suspended, err := agent.NewAgentHarness(ctx, agent.AgentHarnessOptions{
		Session: session,
		Models:  ai.CreateModels(),
		Model:   ai.Model{ID: "model", Provider: "provider", API: "api"},
	})
	if err != nil {
		t.Fatalf("NewAgentHarness(empty): %v", err)
	}
	if len(suspended) != 0 {
		t.Fatalf("suspended = %#v, want empty", suspended)
	}
	if got := harness.Name(); got != "main" {
		t.Fatalf("Name() = %q, want main", got)
	}
	if harness.Session() != session {
		t.Fatal("Session() does not expose the supplied session view")
	}
	if leaf, ok, err := harness.GetLeafID(ctx); err != nil || ok || leaf != "" {
		t.Fatalf("GetLeafID() = (%q, %t, %v), want empty leaf", leaf, ok, err)
	}
	if err := harness.Close(ctx); err != nil {
		t.Fatalf("Close(): %v", err)
	}

	recorded, err := repo.Create(ctx, agent.SessionCreateOptions{ID: "recorded"})
	if err != nil {
		t.Fatalf("create recorded session: %v", err)
	}
	if _, err := recorded.AppendRecord(ctx, agent.OperationStartedRecord{
		RecordBase: agent.RecordBase{ID: "run", Lane: "main"},
		Intent:     agent.RunOperationIntent{},
	}); err != nil {
		t.Fatalf("append record: %v", err)
	}

	harness, suspended, err = agent.NewAgentHarness(ctx, agent.AgentHarnessOptions{
		Session: recorded,
		Models:  ai.CreateModels(),
		Model:   ai.Model{ID: "model", Provider: "provider", API: "api"},
	})
	if harness != nil || suspended != nil {
		t.Fatalf("NewAgentHarness(recorded) = (%p, %#v, %v), want nil results", harness, suspended, err)
	}
	if !errors.Is(err, agent.ErrNotImplemented) {
		t.Fatalf("NewAgentHarness(recorded) error = %v, want ErrNotImplemented", err)
	}
	var unavailable *agent.HarnessNotImplemented
	if !errors.As(err, &unavailable) || unavailable.Operation != "create.restore" {
		t.Fatalf("NewAgentHarness(recorded) error = %#v, want HarnessNotImplemented create.restore", err)
	}
	if unavailable.Name != "HarnessNotImplemented" || unavailable.Message != unavailable.Error() || unavailable.Cause == nil {
		t.Fatalf("HarnessNotImplemented fields = %#v", unavailable)
	}
	var capability *agent.NotImplementedError
	if !errors.As(err, &capability) || capability.Module != "agent" || capability.Operation != "AgentHarness.create.restore" {
		t.Fatalf("NewAgentHarness(recorded) capability = %#v, want agent/AgentHarness.create.restore", capability)
	}
}

func TestHarnessTaggedErrorsExposeJSONTags(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		json    func() map[string]any
		message string
	}{
		{"LaneBusy", &agent.LaneBusy{Lane: "main", OperationID: "run", OperationKind: agent.OperationKindRun, Message: "busy"}, (&agent.LaneBusy{Message: "busy"}).ToJSON, "busy"},
		{"MissingIdentities", &agent.MissingIdentities{Message: "missing"}, (&agent.MissingIdentities{Message: "missing"}).ToJSON, "missing"},
		{"NoActiveRun", &agent.NoActiveRun{Message: "none"}, (&agent.NoActiveRun{Message: "none"}).ToJSON, "none"},
		{"NoActiveOperation", &agent.NoActiveOperation{Message: "none"}, (&agent.NoActiveOperation{Message: "none"}).ToJSON, "none"},
		{"NothingToResume", &agent.NothingToResume{Message: "none"}, (&agent.NothingToResume{Message: "none"}).ToJSON, "none"},
		{"InvalidMessage", &agent.InvalidMessage{Message: "invalid"}, (&agent.InvalidMessage{Message: "invalid"}).ToJSON, "invalid"},
		{"UnknownSkill", &agent.UnknownSkill{Message: "unknown"}, (&agent.UnknownSkill{Message: "unknown"}).ToJSON, "unknown"},
		{"UnknownTemplate", &agent.UnknownTemplate{Message: "unknown"}, (&agent.UnknownTemplate{Message: "unknown"}).ToJSON, "unknown"},
		{"UnknownTarget", &agent.UnknownTarget{Message: "unknown"}, (&agent.UnknownTarget{Message: "unknown"}).ToJSON, "unknown"},
		{"UnknownQueueItem", &agent.UnknownQueueItem{Message: "unknown"}, (&agent.UnknownQueueItem{Message: "unknown"}).ToJSON, "unknown"},
		{"LaneExists", &agent.LaneExists{Message: "exists"}, (&agent.LaneExists{Message: "exists"}).ToJSON, "exists"},
		{"InvalidLane", &agent.InvalidLane{Message: "invalid"}, (&agent.InvalidLane{Message: "invalid"}).ToJSON, "invalid"},
		{"NothingToCompact", &agent.NothingToCompact{Message: "none"}, (&agent.NothingToCompact{Message: "none"}).ToJSON, "none"},
		{"Closed", &agent.Closed{Message: "closed"}, (&agent.Closed{Message: "closed"}).ToJSON, "closed"},
	}
	for _, test := range tests {
		if test.err.Error() != test.message {
			t.Errorf("%s Error() = %q, want %q", test.name, test.err, test.message)
		}
		payload := test.json()
		if payload["_tag"] != test.name || payload["message"] != test.message {
			t.Errorf("%s ToJSON() = %#v", test.name, payload)
		}
	}

	busy := (&agent.LaneBusy{Lane: "main", OperationID: "run", OperationKind: agent.OperationKindRun, Message: "busy"}).ToJSON()
	if busy["lane"] != "main" || busy["operationId"] != "run" || busy["operationKind"] != agent.OperationKindRun {
		t.Fatalf("LaneBusy.ToJSON() = %#v", busy)
	}
}

func TestHarnessUnionMembersRemainCompileUsable(t *testing.T) {
	var rejected agent.RunRejected = &agent.LaneBusy{Message: "busy"}
	if rejected.ToJSON()["_tag"] != "LaneBusy" {
		t.Fatalf("RunRejected.ToJSON() = %#v", rejected.ToJSON())
	}
	resume := agent.ResumeOutcome{Kind: "completed", Operation: agent.OperationKindRun, RunID: "run"}
	if resume.Kind != "completed" {
		t.Fatalf("ResumeOutcome.Kind = %q", resume.Kind)
	}
	suspended := agent.SuspendedOperation{Missing: agent.SuspendedMissingIdentities{Tools: []string{"read"}, Models: []string{"model"}}}
	if !reflect.DeepEqual(suspended.Missing.Tools, []string{"read"}) || !reflect.DeepEqual(suspended.Missing.Models, []string{"model"}) {
		t.Fatalf("SuspendedOperation.Missing = %#v", suspended.Missing)
	}
}

func TestAgentHarnessConfigurationUsesDefensiveCopies(t *testing.T) {
	ctx := context.Background()
	harness := newHarnessForTest(t, ctx)

	if got, err := harness.GetThinkingLevel(ctx); err != nil || got != agent.ThinkingLevel("off") {
		t.Fatalf("default GetThinkingLevel() = (%q, %v), want off", got, err)
	}
	if got, err := harness.GetRetryPolicy(ctx); err != nil || got != (agent.RetryPolicy{BaseDelayMS: 1000}) {
		t.Fatalf("default GetRetryPolicy() = (%#v, %v)", got, err)
	}
	if got, err := harness.GetCompactionSettings(ctx); err != nil || got != agent.DefaultCompactionSettings {
		t.Fatalf("default GetCompactionSettings() = (%#v, %v), want %#v", got, err, agent.DefaultCompactionSettings)
	}
	if got, err := harness.GetSteeringMode(ctx); err != nil || got != agent.QueueOneAtATime {
		t.Fatalf("default GetSteeringMode() = (%q, %v), want one-at-a-time", got, err)
	}
	if got, err := harness.GetFollowUpMode(ctx); err != nil || got != agent.QueueOneAtATime {
		t.Fatalf("default GetFollowUpMode() = (%q, %v), want one-at-a-time", got, err)
	}

	model := ai.Model{ID: "updated", Provider: "provider", API: "api"}
	if err := harness.SetModel(ctx, model); err != nil {
		t.Fatalf("SetModel(): %v", err)
	}
	if got, err := harness.GetModel(ctx); err != nil || !reflect.DeepEqual(got, model) {
		t.Fatalf("GetModel() = (%#v, %v), want %#v", got, err, model)
	}
	if err := harness.SetThinkingLevel(ctx, agent.ThinkingLevel("high")); err != nil {
		t.Fatalf("SetThinkingLevel(): %v", err)
	}
	if got, err := harness.GetThinkingLevel(ctx); err != nil || got != agent.ThinkingLevel("high") {
		t.Fatalf("GetThinkingLevel() = (%q, %v), want high", got, err)
	}

	active := []string{"one"}
	if err := harness.SetActiveTools(ctx, active); err != nil {
		t.Fatalf("SetActiveTools(): %v", err)
	}
	active[0] = "caller-mutated"
	gotActive, err := harness.GetActiveTools(ctx)
	if err != nil {
		t.Fatalf("GetActiveTools(): %v", err)
	}
	gotActive[0] = "read-mutated"
	if got, _ := harness.GetActiveTools(ctx); !reflect.DeepEqual(got, []string{"one"}) {
		t.Fatalf("GetActiveTools() after mutations = %#v, want [one]", got)
	}

	tools := []agent.HarnessTool{{Tool: ai.Tool{Name: "tool", Parameters: []byte(`{"type":"object"}`)}}}
	if err := harness.SetTools(ctx, tools); err != nil {
		t.Fatalf("SetTools(): %v", err)
	}
	tools[0].Name = "caller-mutated"
	tools[0].Parameters[0] = '['
	gotTools, err := harness.GetTools(ctx)
	if err != nil {
		t.Fatalf("GetTools(): %v", err)
	}
	gotTools[0].Name = "read-mutated"
	gotTools[0].Parameters[0] = '['
	if got, _ := harness.GetTools(ctx); got[0].Name != "tool" || string(got[0].Parameters) != `{"type":"object"}` {
		t.Fatalf("GetTools() after mutations = %#v", got)
	}
	if got, _ := harness.GetActiveTools(ctx); !reflect.DeepEqual(got, []string{"tool"}) {
		t.Fatalf("GetActiveTools() after SetTools = %#v, want [tool]", got)
	}

	resources := agent.Resources{
		Skills:          []agent.Skill{{Name: "skill", Description: "description", Content: "body", FilePath: "/tmp/SKILL.md"}},
		PromptTemplates: []agent.PromptTemplate{{Name: "template", Content: "body"}},
	}
	if err := harness.SetResources(ctx, resources); err != nil {
		t.Fatalf("SetResources(): %v", err)
	}
	resources.Skills[0].Name = "caller-mutated"
	gotResources, err := harness.GetResources(ctx)
	if err != nil {
		t.Fatalf("GetResources(): %v", err)
	}
	gotResources.PromptTemplates[0].Name = "read-mutated"
	if got, _ := harness.GetResources(ctx); got.Skills[0].Name != "skill" || got.PromptTemplates[0].Name != "template" {
		t.Fatalf("GetResources() after mutations = %#v", got)
	}

	maxTokens := int64(10)
	header := "value"
	streamOptions := agent.StreamOptions{
		StreamOptions: ai.StreamOptions{
			ProviderRequestOptions: ai.ProviderRequestOptions{Headers: ai.ProviderHeaders{"x-test": &header}},
			MaxTokens:              &maxTokens,
		},
	}
	if err := harness.SetStreamOptions(ctx, streamOptions); err != nil {
		t.Fatalf("SetStreamOptions(): %v", err)
	}
	*streamOptions.MaxTokens = 20
	*streamOptions.Headers["x-test"] = "caller-mutated"
	gotStream, err := harness.GetStreamOptions(ctx)
	if err != nil {
		t.Fatalf("GetStreamOptions(): %v", err)
	}
	*gotStream.MaxTokens = 30
	*gotStream.Headers["x-test"] = "read-mutated"
	if got, _ := harness.GetStreamOptions(ctx); got.MaxTokens == nil || *got.MaxTokens != 10 || got.Headers["x-test"] == nil || *got.Headers["x-test"] != "value" {
		t.Fatalf("GetStreamOptions() after mutations = %#v", got)
	}

	retry := agent.RetryPolicy{Enabled: true, MaxRetries: 2, BaseDelayMS: 10}
	if err := harness.SetRetryPolicy(ctx, retry); err != nil {
		t.Fatalf("SetRetryPolicy(): %v", err)
	}
	if got, err := harness.GetRetryPolicy(ctx); err != nil || got != retry {
		t.Fatalf("GetRetryPolicy() = (%#v, %v), want %#v", got, err, retry)
	}
	compaction := agent.CompactionSettings{Enabled: false, ReserveTokens: 1, KeepRecentTokens: 2}
	if err := harness.SetCompactionSettings(ctx, compaction); err != nil {
		t.Fatalf("SetCompactionSettings(): %v", err)
	}
	if got, err := harness.GetCompactionSettings(ctx); err != nil || got != compaction {
		t.Fatalf("GetCompactionSettings() = (%#v, %v), want %#v", got, err, compaction)
	}
	if err := harness.SetSteeringMode(ctx, agent.QueueAll); err != nil {
		t.Fatalf("SetSteeringMode(): %v", err)
	}
	if got, err := harness.GetSteeringMode(ctx); err != nil || got != agent.QueueAll {
		t.Fatalf("GetSteeringMode() = (%q, %v), want all", got, err)
	}
	if err := harness.SetFollowUpMode(ctx, agent.QueueAll); err != nil {
		t.Fatalf("SetFollowUpMode(): %v", err)
	}
	if got, err := harness.GetFollowUpMode(ctx); err != nil || got != agent.QueueAll {
		t.Fatalf("GetFollowUpMode() = (%q, %v), want all", got, err)
	}
}

func TestAgentHarnessUnavailableOperationsAreExactAndDoNotWriteSession(t *testing.T) {
	ctx := context.Background()
	harness := newHarnessForTest(t, ctx)
	callbackCalled := false
	operations := []struct {
		name string
		call func() error
	}{
		{"prompt", func() error {
			_, err := harness.Prompt(ctx, ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("hello")})
			return err
		}},
		{"prompt", func() error { _, err := harness.PromptText(ctx, "hello"); return err }},
		{"skill", func() error { _, err := harness.Skill(ctx, "skill"); return err }},
		{"promptFromTemplate", func() error { _, err := harness.PromptFromTemplate(ctx, "template"); return err }},
		{"compact", func() error { _, err := harness.Compact(ctx); return err }},
		{"navigateTree", func() error { _, err := harness.NavigateTree(ctx, nil); return err }},
		{"resume", func() error { _, err := harness.Resume(ctx); return err }},
		{"abort", func() error { _, err := harness.Abort(ctx); return err }},
		{"steer", func() error { _, err := harness.Steer(ctx, ai.UserMessage{}); return err }},
		{"steer", func() error { _, err := harness.SteerText(ctx, "hello"); return err }},
		{"followUp", func() error { _, err := harness.FollowUp(ctx, ai.UserMessage{}); return err }},
		{"followUp", func() error { _, err := harness.FollowUpText(ctx, "hello"); return err }},
		{"nextRun", func() error { _, err := harness.NextRun(ctx, ai.UserMessage{}); return err }},
		{"nextRun", func() error { _, err := harness.NextRunText(ctx, "hello"); return err }},
		{"cancelQueued", func() error { _, err := harness.CancelQueued(ctx, "entry"); return err }},
		{"recordUsage", func() error { _, err := harness.RecordUsage(ctx, ai.Usage{}); return err }},
		{"waitForIdle", func() error { return harness.WaitForIdle(ctx) }},
		{"runWhenIdle", func() error {
			return harness.RunWhenIdle(ctx, func(context.Context) error { callbackCalled = true; return nil })
		}},
		{"peekAction", func() error { _, _, err := harness.PeekAction(ctx); return err }},
		{"executeAction", func() error { _, _, err := harness.ExecuteAction(ctx); return err }},
		{"runToCompletion", func() error { return harness.RunToCompletion(ctx) }},
		{"watch", func() error { _, err := harness.Watch(ctx); return err }},
		{"lane", func() error { _, _, err := harness.Lane(ctx, "other"); return err }},
		{"createLane", func() error { _, err := harness.CreateLane(ctx, "other", nil); return err }},
		{"lanes", func() error { _, err := harness.Lanes(ctx); return err }},
		{"watchSession", func() error { _, err := harness.WatchSession(ctx); return err }},
		{"hooks.on", func() error {
			_, err := harness.Hooks().On(ctx, agent.HookBeforeRun, func(context.Context, any) (any, error) { return nil, nil })
			return err
		}},
		{"events.on", func() error {
			_, err := harness.Events().On(ctx, "event", func(context.Context, any) error { return nil })
			return err
		}},
	}

	for _, operation := range operations {
		err := operation.call()
		assertHarnessUnavailable(t, err, operation.name)
	}
	if callbackCalled {
		t.Fatal("RunWhenIdle invoked its callback")
	}
	if log, err := harness.Session().(*agent.Session).GetLog(ctx, agent.LogOptions{}); err != nil || len(log) != 0 {
		t.Fatalf("Session log after unavailable calls = (%#v, %v), want empty", log, err)
	}

	if err := harness.Close(ctx); err != nil {
		t.Fatalf("Close(): %v", err)
	}
	for _, operation := range operations {
		err := operation.call()
		var closed *agent.HarnessClosed
		if !errors.As(err, &closed) || errors.Is(err, agent.ErrNotImplemented) {
			t.Errorf("%s after Close error = %#v, want HarnessClosed only", operation.name, err)
		}
	}
	if callbackCalled {
		t.Fatal("RunWhenIdle invoked its callback after Close")
	}
	if log, err := harness.Session().(*agent.Session).GetLog(ctx, agent.LogOptions{}); err != nil || len(log) != 0 {
		t.Fatalf("Session log after closed calls = (%#v, %v), want empty", log, err)
	}
}

func assertHarnessUnavailable(t *testing.T, err error, operation string) {
	t.Helper()
	if !errors.Is(err, agent.ErrNotImplemented) {
		t.Fatalf("%s error = %#v, want ErrNotImplemented", operation, err)
	}
	var unavailable *agent.HarnessNotImplemented
	if !errors.As(err, &unavailable) || unavailable.Operation != operation {
		t.Fatalf("%s error = %#v, want HarnessNotImplemented %q", operation, err, operation)
	}
	var capability *agent.NotImplementedError
	if !errors.As(err, &capability) || capability.Module != "agent" || capability.Operation != "AgentHarness."+operation {
		t.Fatalf("%s capability = %#v", operation, capability)
	}
}

func newHarnessForTest(t *testing.T, ctx context.Context) *agent.AgentHarness {
	t.Helper()
	repo := agent.NewInMemorySessionRepo()
	session, err := repo.Create(ctx, agent.SessionCreateOptions{ID: "harness"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	harness, suspended, err := agent.NewAgentHarness(ctx, agent.AgentHarnessOptions{
		Session: session,
		Models:  ai.CreateModels(),
		Model:   ai.Model{ID: "model", Provider: "provider", API: "api"},
	})
	if err != nil || len(suspended) != 0 {
		t.Fatalf("NewAgentHarness() = (%p, %#v, %v)", harness, suspended, err)
	}
	return harness
}
