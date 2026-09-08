package codingagent

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
)

func (s *AgentSession) configurationReady() error {
	if s.agent == nil {
		return notImplemented("AgentSession.Configuration")
	}
	if s.disposed {
		return fmt.Errorf("AgentSession is disposed")
	}
	if s.active || s.compactionCancel != nil || s.configurationNotifying || s.agent.State().IsStreaming {
		return fmt.Errorf("AgentSession is busy")
	}
	return nil
}

func validSessionThinking(level agent.ThinkingLevel) bool {
	return slices.Contains([]agent.ThinkingLevel{"off", "minimal", "low", "medium", "high", "xhigh", "max"}, level)
}

func (s *AgentSession) GetAvailableThinkingLevels() ([]agent.ThinkingLevel, error) {
	if s.agent == nil {
		return nil, notImplemented("AgentSession.GetAvailableThinkingLevels")
	}
	return ai.GetSupportedThinkingLevels(s.Model()), nil
}
func (s *AgentSession) SupportsThinking() (bool, error) {
	if s.agent == nil {
		return false, notImplemented("AgentSession.SupportsThinking")
	}
	return s.Model().Reasoning, nil
}
func (s *AgentSession) SetThinkingLevel(level agent.ThinkingLevel) error {
	if s.agent == nil {
		return notImplemented("AgentSession.SetThinkingLevel")
	}
	s.mu.Lock()
	event, err := s.setThinkingLocked(level)
	s.finishConfigurationLocked(event)
	return err
}
func (s *AgentSession) setThinkingLocked(level agent.ThinkingLevel) (*AgentSessionThinkingLevelChangedEvent, error) {
	if err := s.configurationReady(); err != nil {
		return nil, err
	}
	if !validSessionThinking(level) {
		return nil, fmt.Errorf("invalid thinking level: %q", level)
	}
	state := s.agent.State()
	effective := ai.ClampThinkingLevel(state.Model, level)
	if effective == state.ThinkingLevel {
		return nil, nil
	}
	if err := s.persistConfiguration(nil, &effective); err != nil {
		return nil, err
	}
	_ = s.agent.SetThinkingLevel(effective)
	return &AgentSessionThinkingLevelChangedEvent{Type: AgentSessionEventTypeThinkingLevelChanged, Level: effective}, nil
}
func (s *AgentSession) CycleThinkingLevel() (agent.ThinkingLevel, error) {
	if s.agent == nil {
		return "", notImplemented("AgentSession.CycleThinkingLevel")
	}
	s.mu.Lock()
	if err := s.configurationReady(); err != nil {
		s.mu.Unlock()
		return "", err
	}
	state := s.agent.State()
	if !state.Model.Reasoning {
		s.mu.Unlock()
		return "", nil
	}
	levels := ai.GetSupportedThinkingLevels(state.Model)
	if len(levels) == 0 {
		s.mu.Unlock()
		return "", fmt.Errorf("model has no supported thinking levels")
	}
	next := levels[(slices.Index(levels, state.ThinkingLevel)+1)%len(levels)]
	event, err := s.setThinkingLocked(next)
	s.finishConfigurationLocked(event)
	if err != nil {
		return "", err
	}
	return next, nil
}

func (s *AgentSession) SetModel(model ai.Model) error {
	if s.agent == nil {
		return notImplemented("AgentSession.SetModel")
	}
	s.mu.Lock()
	event, err := s.setModelLocked(context.Background(), model, "")
	s.finishConfigurationLocked(event)
	return err
}
func (s *AgentSession) setModelLocked(ctx context.Context, model ai.Model, level agent.ThinkingLevel) (*AgentSessionThinkingLevelChangedEvent, error) {
	if err := s.configurationReady(); err != nil {
		return nil, err
	}
	if s.modelRuntime == nil {
		return nil, fmt.Errorf("AgentSession has no ModelRuntime")
	}
	if err := validateSessionModel(model); err != nil {
		return nil, err
	}
	if _, ok, err := s.modelRuntime.GetModel(string(model.Provider), model.ID); err != nil {
		return nil, err
	} else if !ok {
		return nil, fmt.Errorf("unknown model: %s/%s", model.Provider, model.ID)
	}
	if s.runtimeStream {
		if err := checkRuntimeAdapter(model); err != nil {
			return nil, err
		}
	}
	auth, err := s.modelRuntime.GetModelAuth(ctx, model)
	if err != nil {
		return nil, err
	}
	if _, ok := auth.Value(); !ok {
		return nil, fmt.Errorf("No API key for %s/%s", model.Provider, model.ID)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	state := s.agent.State()
	if level == "" {
		level = state.ThinkingLevel
		if !state.Model.Reasoning {
			level, err = s.settingsManager.GetDefaultThinkingLevel()
			if err != nil {
				return nil, err
			}
			if level == "" {
				level = "medium"
			}
		}
	}
	effective := ai.ClampThinkingLevel(model, level)
	var changed *agent.ThinkingLevel
	if effective != state.ThinkingLevel {
		changed = &effective
	}
	if err := s.persistConfiguration(&model, changed); err != nil {
		return nil, err
	}
	s.agent.SetModel(model)
	_ = s.agent.SetThinkingLevel(effective)
	if changed != nil {
		return &AgentSessionThinkingLevelChangedEvent{Type: AgentSessionEventTypeThinkingLevelChanged, Level: effective}, nil
	}
	return nil, nil
}
func validateSessionModel(model ai.Model) error {
	if model.ID == "" || model.Provider == "" || model.API == "" || model.ContextWindow <= 0 || model.MaxTokens <= 0 {
		return fmt.Errorf("invalid model: %s/%s", model.Provider, model.ID)
	}
	return nil
}
func (s *AgentSession) SetScopedModels(models []ScopedModel) error {
	if s.agent == nil {
		return notImplemented("AgentSession.SetScopedModels")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.configurationReady(); err != nil {
		return err
	}
	for _, entry := range models {
		if err := validateSessionModel(entry.Model); err != nil {
			return err
		}
		if entry.ThinkingLevel != "" && !validSessionThinking(entry.ThinkingLevel) {
			return fmt.Errorf("invalid thinking level: %q", entry.ThinkingLevel)
		}
	}
	s.scopedModels = cloneScopedModels(models)
	return nil
}
func (s *AgentSession) CycleModel(ctx context.Context, directions ...ModelCycleDirection) (*ModelCycleResult, error) {
	if s.agent == nil {
		return nil, notImplemented("AgentSession.CycleModel")
	}
	if ctx == nil {
		return nil, fmt.Errorf("cycle model context must not be nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	direction := ModelCycleForward
	if len(directions) > 1 {
		return nil, fmt.Errorf("expected at most one cycle direction")
	}
	if len(directions) == 1 {
		direction = directions[0]
	}
	if direction != ModelCycleForward && direction != ModelCycleBackward {
		return nil, fmt.Errorf("invalid cycle direction: %q", direction)
	}
	s.mu.Lock()
	result, event, err := s.cycleModelLocked(ctx, direction)
	s.finishConfigurationLocked(event)
	return result, err
}
func (s *AgentSession) cycleModelLocked(ctx context.Context, direction ModelCycleDirection) (*ModelCycleResult, *AgentSessionThinkingLevelChangedEvent, error) {
	if err := s.configurationReady(); err != nil {
		return nil, nil, err
	}
	if s.modelRuntime == nil {
		return nil, nil, fmt.Errorf("AgentSession has no ModelRuntime")
	}
	available, err := s.modelRuntime.GetAvailableSnapshot()
	if err != nil {
		return nil, nil, err
	}
	candidates := []ScopedModel{}
	scoped := len(s.scopedModels) > 0
	if scoped {
		for _, entry := range s.scopedModels {
			for _, m := range available {
				if ai.ModelsAreEqual(&entry.Model, &m) {
					candidates = append(candidates, entry)
					break
				}
			}
		}
	} else {
		for _, m := range available {
			candidates = append(candidates, ScopedModel{Model: m})
		}
	}
	if len(candidates) <= 1 {
		return nil, nil, nil
	}
	index := 0
	for i, c := range candidates {
		if sameSessionModel(c.Model, s.agent.State().Model) {
			index = i
			break
		}
	}
	if direction == ModelCycleBackward {
		index = (index - 1 + len(candidates)) % len(candidates)
	} else {
		index = (index + 1) % len(candidates)
	}
	next := candidates[index]
	event, err := s.setModelLocked(ctx, next.Model, next.ThinkingLevel)
	if err != nil {
		return nil, nil, err
	}
	state := s.agent.State()
	return &ModelCycleResult{Model: state.Model, ThinkingLevel: state.ThinkingLevel, IsScoped: scoped}, event, nil
}

func cloneSessionTools(tools []agent.ErasedAgentTool) []agent.ErasedAgentTool {
	result := append([]agent.ErasedAgentTool{}, tools...)
	for i := range result {
		data, _ := json.Marshal(result[i].Tool)
		result[i].Tool = ai.Tool{}
		_ = json.Unmarshal(data, &result[i].Tool)
	}
	return result
}
func (s *AgentSession) SetActiveToolsByName(names []string) error {
	if s.agent == nil {
		return notImplemented("AgentSession.SetActiveToolsByName")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.configurationReady(); err != nil {
		return err
	}
	selected := []agent.ErasedAgentTool{}
	for _, name := range names {
		index := slices.IndexFunc(s.allTools, func(t agent.ErasedAgentTool) bool { return t.Name == name })
		if index < 0 {
			return fmt.Errorf("unknown, excluded or unavailable tool: %s", name)
		}
		if slices.ContainsFunc(selected, func(t agent.ErasedAgentTool) bool { return t.Name == name }) {
			return fmt.Errorf("duplicate tool: %s", name)
		}
		selected = append(selected, s.allTools[index])
	}
	options := s.promptOptions
	options.SelectedTools = append([]string{}, names...)
	options.PromptGuidelines = sessionToolGuidelines(names)
	prompt := buildSystemPrompt(options)
	if err := s.agent.SetTools(selected); err != nil {
		return err
	}
	s.agent.SetSystemPrompt(prompt)
	s.activeToolNames = append([]string{}, names...)
	return nil
}

func sameSessionModel(a, b ai.Model) bool { return ai.ModelsAreEqual(&a, &b) }

func (s *AgentSession) finishConfigurationLocked(event *AgentSessionThinkingLevelChangedEvent) {
	if event == nil {
		s.mu.Unlock()
		return
	}
	s.configurationNotifying = true
	s.mu.Unlock()
	defer func() { s.mu.Lock(); s.configurationNotifying = false; s.mu.Unlock() }()
	s.emit(*event)
}
