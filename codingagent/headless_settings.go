package codingagent

import (
	"context"
	"fmt"
	"strings"

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

func resolveHeadlessModel(ctx context.Context, models ai.Models, settings *SettingsManager, options CreateHeadlessSessionOptions) (ai.Model, agent.ThinkingLevel, error) {
	provider, pattern := options.Provider, options.Model
	thinking := options.Thinking
	var model ai.Model
	if pattern != "" {
		if provider != "" {
			if _, ok := models.GetProvider(provider); !ok {
				return model, thinking, &CLIArgumentError{Message: fmt.Sprintf("Unknown provider %q", provider)}
			}
		}
		if prefix, rest, ok := strings.Cut(pattern, "/"); ok {
			if _, known := models.GetProvider(ai.ProviderID(prefix)); known && (provider == "" || strings.EqualFold(string(provider), prefix)) {
				provider = ai.ProviderID(prefix)
				pattern = rest
			}
		}
		candidates := models.GetModels()
		if provider != "" {
			candidates = models.GetModels(provider)
		}
		matches := func(pattern string) []ai.Model {
			result := []ai.Model{}
			for _, m := range candidates {
				if strings.EqualFold(m.ID, pattern) {
					result = append(result, m)
				}
			}
			return result
		}
		found := matches(pattern)
		if len(found) == 0 {
			if index := strings.LastIndexByte(pattern, ':'); index >= 0 {
				level := agent.ThinkingLevel(pattern[index+1:])
				switch level {
				case "off", "minimal", "low", "medium", "high", "xhigh", "max":
					if thinking == "" {
						thinking = level
					}
					pattern = pattern[:index]
					found = matches(pattern)
				}
			}
		}
		if len(found) > 1 {
			available, err := headlessAvailableModels(ctx, models)
			if err != nil {
				return model, thinking, err
			}
			authenticated := []ai.Model{}
			for _, m := range found {
				for _, a := range available {
					if ai.ModelsAreEqual(&m, &a) {
						authenticated = append(authenticated, m)
						break
					}
				}
			}
			if len(authenticated) == 1 {
				found = authenticated
			}
		}
		if len(found) > 1 {
			return model, thinking, &CLIArgumentError{Message: fmt.Sprintf("Model %q is ambiguous; use --provider or provider/model", pattern)}
		}
		if len(found) == 0 {
			return model, thinking, &CLIArgumentError{Message: fmt.Sprintf("Unknown model %q for provider %q", pattern, provider)}
		}
		model = found[0]
	} else {
		canRestore := func(model ai.Model) bool {
			if model.ID == "" {
				return false
			}
			if options.APIKey != nil {
				return true
			}
			available, err := models.GetAvailable(ctx, model.Provider)
			return err == nil && len(available) > 0
		}
		if options.SessionManager != nil {
			saved := options.SessionManager.BuildSessionContext()
			if len(saved.Messages) > 0 && saved.Model != nil {
				model, _ = models.GetModel(ai.ProviderID(saved.Model.Provider), saved.Model.ModelID)
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
			if provider != "" && id != "" {
				model, _ = models.GetModel(ai.ProviderID(provider), id)
				if !canRestore(model) {
					model = ai.Model{}
				}
			}
		}
		if model.ID == "" {
			available, err := headlessAvailableModels(ctx, models)
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
			return model, thinking, &CLIArgumentError{Message: "Headless mode requires --provider <provider>"}
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

var headlessDefaultModels = [][2]string{{"deepseek", "deepseek-v4-pro"}}

func headlessAvailableModels(ctx context.Context, models ai.Models) ([]ai.Model, error) {
	var available []ai.Model
	for _, provider := range models.GetProviders() {
		if len(provider.GetModels()) == 0 {
			continue
		}
		values, err := models.GetAvailable(ctx, provider.ID())
		if err != nil {
			return nil, err
		}
		available = append(available, values...)
	}
	return available, nil
}
