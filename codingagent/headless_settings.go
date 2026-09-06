package codingagent

import (
	"context"
	"fmt"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
)

func checkHeadlessSettings(settings *SettingsManager) error {
	// Reading configuration is not authorization to activate future resource runtimes.
	packages, err := settings.GetPackages()
	if err != nil {
		return err
	}
	if len(packages) > 0 {
		return notImplemented("headless.settings.resources")
	}
	for _, get := range []func() ([]string, error){settings.GetExtensionPaths, settings.GetSkillPaths, settings.GetPromptTemplatePaths, settings.GetThemePaths} {
		paths, err := get()
		if err != nil {
			return err
		}
		if len(paths) > 0 {
			return notImplemented("headless.settings.resources")
		}
	}
	proxy, err := settingsValue[string](*settings, "httpProxy", "", "")
	if err != nil {
		return err
	}
	if proxy != "" {
		return notImplemented("headless.settings.httpProxy")
	}

	return nil
}

func resolveHeadlessModel(ctx context.Context, runtime *ModelRuntime, settings *SettingsManager, options CreateHeadlessSessionOptions) (ai.Model, agent.ThinkingLevel, error) {
	thinking := options.Thinking
	var model ai.Model
	if options.Model != "" {
		resolved, err := ResolveCLIModel(ResolveCliModelOptions{CLIProvider: string(options.Provider), CLIModel: options.Model, CLIThinking: thinking, ModelRuntime: runtime})
		if err != nil {
			return model, thinking, err
		}
		if resolved.Error != nil {
			return model, thinking, &CLIArgumentError{Message: *resolved.Error}
		}
		model = *resolved.Model
		if thinking == "" && resolved.ThinkingLevel != nil {
			thinking = *resolved.ThinkingLevel
		}
	} else {
		canRestore := func(m ai.Model) bool {
			configured, _ := runtime.HasConfiguredAuth(string(m.Provider))
			return m.ID != "" && configured
		}
		if options.SessionManager != nil {
			saved := options.SessionManager.BuildSessionContext()
			if len(saved.Messages) > 0 && saved.Model != nil {
				model, _, _ = runtime.GetModel(saved.Model.Provider, saved.Model.ModelID)
				if !canRestore(model) {
					model = ai.Model{}
				}
			}
		}
		if model.ID == "" {
			provider, err := settings.GetDefaultProvider()
			if err != nil {
				return model, thinking, err
			}
			id, err := settings.GetDefaultModel()
			if err != nil {
				return model, thinking, err
			}
			model, _, _ = runtime.GetModel(provider, id)
			if !canRestore(model) {
				model = ai.Model{}
			}
		}
		if model.ID == "" {
			available, err := runtime.GetAvailableSnapshot()
			if err != nil {
				return model, thinking, err
			}
			for _, candidate := range headlessDefaultModels {
				for _, m := range available {
					if string(m.Provider) == candidate[0] && m.ID == candidate[1] {
						model = m
						break
					}
				}
				if model.ID != "" {
					break
				}
			}
			if model.ID == "" && len(available) > 0 {
				model = available[0]
			}
		}
		if model.ID == "" {
			diagnostic, _ := runtime.GetError()
			if diagnostic != "" {
				return model, thinking, fmt.Errorf("%s", diagnostic)
			}
			return model, thinking, &CLIArgumentError{Message: "No models available with configured authentication. Use --provider and --model with --api-key, or configure credentials."}
		}
	}
	if thinking == "" && options.SessionManager != nil {
		saved := options.SessionManager.BuildSessionContext()
		if len(saved.Messages) > 0 {
			for _, entry := range options.SessionManager.GetBranch() {
				if entry.Type == "thinking_level_change" {
					thinking = agent.ThinkingLevel(saved.ThinkingLevel)
					break
				}
			}
		}
	}
	if thinking == "" {
		var err error
		thinking, err = settings.GetDefaultThinkingLevel()
		if err != nil {
			return model, thinking, err
		}
	}
	if thinking == "" {
		thinking = "medium"
	}
	thinking = ai.ClampThinkingLevel(model, thinking)
	return model, thinking, nil
}

var headlessDefaultModels = [][2]string{{"amazon-bedrock", "us.anthropic.claude-opus-4-6-v1"}, {"ant-ling", "Ring-2.6-1T"}, {"anthropic", "claude-opus-4-8"}, {"openai", "gpt-5.5"}, {"azure-openai-responses", "gpt-5.4"}, {"openai-codex", "gpt-5.5"}, {"radius", "auto"}, {"nvidia", "nvidia/nemotron-3-super-120b-a12b"}, {"deepseek", "deepseek-v4-pro"}, {"google", "gemini-3.1-pro-preview"}, {"google-vertex", "gemini-3.1-pro-preview"}, {"github-copilot", "gpt-5.4"}, {"openrouter", "moonshotai/kimi-k2.6"}, {"vercel-ai-gateway", "zai/glm-5.1"}, {"xai", "grok-4.5"}, {"groq", "openai/gpt-oss-120b"}, {"cerebras", "zai-glm-4.7"}, {"zai", "glm-5.1"}, {"zai-coding-cn", "glm-5.1"}, {"mistral", "devstral-medium-latest"}, {"minimax", "MiniMax-M2.7"}, {"minimax-cn", "MiniMax-M2.7"}, {"moonshotai", "kimi-k2.6"}, {"moonshotai-cn", "kimi-k2.6"}, {"huggingface", "moonshotai/Kimi-K2.6"}, {"fireworks", "accounts/fireworks/models/kimi-k2p6"}, {"together", "moonshotai/Kimi-K2.6"}, {"baseten", "zai-org/GLM-5.2"}, {"opencode", "kimi-k2.6"}, {"opencode-go", "kimi-k2.6"}, {"kimi-coding", "kimi-for-coding"}, {"cloudflare-workers-ai", "@cf/moonshotai/kimi-k2.6"}, {"cloudflare-ai-gateway", "workers-ai/@cf/moonshotai/kimi-k2.6"}, {"qwen-token-plan", "qwen3.7-max"}, {"qwen-token-plan-cn", "qwen3.7-max"}, {"qwen-token-plan-individual", "qwen3.8-max"}, {"xiaomi", "mimo-v2.5-pro"}, {"xiaomi-token-plan-cn", "mimo-v2.5-pro"}, {"xiaomi-token-plan-ams", "mimo-v2.5-pro"}, {"xiaomi-token-plan-sgp", "mimo-v2.5-pro"}}

func prepareHeadlessProjectSettings(ctx context.Context, cwd, agentDir string, settings *SettingsManager, override ProjectTrustDecision) error {
	trusted := false
	if override != nil {
		trusted = *override
	} else {
		resources, err := HasTrustRequiringProjectResources(ctx, cwd)
		if err != nil {
			return err
		}
		if !resources {
			trusted = true
		} else {
			if agentDir == "" {
				var err error
				agentDir, err = GetAgentDir()
				if err != nil {
					return err
				}
			}
			decision, err := NewProjectTrustStore(agentDir).Get(ctx, cwd)
			if err != nil {
				return err
			}
			if decision != nil {
				trusted = *decision
			} else {
				global, err := settings.GetGlobalSettings()
				if err != nil {
					return err
				}
				trusted = global.DefaultProjectTrust != nil && *global.DefaultProjectTrust == DefaultProjectTrustAlways
			}
		}
	}
	return settings.SetProjectTrusted(trusted)
}

func restoredModelFallback(manager *SessionManager, model ai.Model, explicit bool) *string {
	if explicit || manager == nil {
		return nil
	}
	saved := manager.BuildSessionContext()
	if len(saved.Messages) == 0 || saved.Model == nil || (saved.Model.Provider == string(model.Provider) && saved.Model.ModelID == model.ID) {
		return nil
	}
	message := fmt.Sprintf("Could not restore model %s/%s. Using %s/%s", saved.Model.Provider, saved.Model.ModelID, model.Provider, model.ID)
	return &message
}
