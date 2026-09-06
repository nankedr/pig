package codingagent

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/nankedr/pig/agent"
	"github.com/nankedr/pig/ai"
)

func ResolveCLIModel(options ResolveCliModelOptions) (ResolveCLIModelResult, error) {
	if options.CLIModel == "" {
		return ResolveCLIModelResult{}, nil
	}
	models, err := options.ModelRuntime.GetModels()
	if err != nil {
		return ResolveCLIModelResult{}, err
	}
	fail := func(message string) (ResolveCLIModelResult, error) {
		return ResolveCLIModelResult{Error: &message}, nil
	}
	if len(models) == 0 {
		return fail("No models available. Check your installation or add models to models.json.")
	}
	providerMap := map[string]string{}
	for _, m := range models {
		providerMap[strings.ToLower(string(m.Provider))] = string(m.Provider)
	}
	provider := providerMap[strings.ToLower(options.CLIProvider)]
	if options.CLIProvider != "" && provider == "" {
		return fail(fmt.Sprintf("Unknown provider %q. Use --list-models to see available providers/models.", options.CLIProvider))
	}
	pattern := options.CLIModel
	inferred := false
	if provider == "" {
		if prefix, rest, ok := strings.Cut(pattern, "/"); ok {
			if canonical := providerMap[strings.ToLower(prefix)]; canonical != "" {
				provider = canonical
				pattern = rest
				inferred = true
			}
		}
	}
	configured := func(m ai.Model) bool { ok, _ := options.ModelRuntime.HasConfiguredAuth(string(m.Provider)); return ok }
	if provider == "" {
		exact := []ai.Model{}
		authenticated := []ai.Model{}
		for _, m := range models {
			if strings.EqualFold(m.ID, pattern) || strings.EqualFold(string(m.Provider)+"/"+m.ID, pattern) {
				exact = append(exact, m)
				if configured(m) {
					authenticated = append(authenticated, m)
				}
			}
		}
		if len(exact) == 1 {
			return ResolveCLIModelResult{Model: &exact[0]}, nil
		}
		if len(exact) > 1 {
			if len(authenticated) == 1 {
				return ResolveCLIModelResult{Model: &authenticated[0]}, nil
			}
			names := []string{}
			for _, m := range exact {
				names = append(names, string(m.Provider)+"/"+m.ID)
			}
			sort.Strings(names)
			hint := "No matching provider is authenticated."
			if len(authenticated) > 1 {
				hint = "More than one matching provider is authenticated."
			}
			return fail(fmt.Sprintf("Model %q is ambiguous across providers: %s. %s Use --provider or provider/model.", pattern, strings.Join(names, ", "), hint))
		}
	}
	if options.CLIProvider != "" && strings.HasPrefix(strings.ToLower(pattern), strings.ToLower(provider)+"/") {
		pattern = pattern[len(provider)+1:]
	}
	candidates := models
	if provider != "" {
		candidates = nil
		for _, m := range models {
			if string(m.Provider) == provider {
				candidates = append(candidates, m)
			}
		}
	}
	result := parseModelPattern(pattern, candidates, false)
	if result.Model != nil {
		if inferred && !configured(*result.Model) {
			raw := []ai.Model{}
			for _, m := range models {
				if strings.EqualFold(m.ID, options.CLIModel) && configured(m) {
					raw = append(raw, m)
				}
			}
			if len(raw) == 1 {
				return ResolveCLIModelResult{Model: &raw[0]}, nil
			}
		}
		return result, nil
	}
	if inferred {
		fallback := parseModelPattern(options.CLIModel, models, false)
		if fallback.Model != nil {
			return fallback, nil
		}
	}
	if provider != "" && len(candidates) > 0 {
		var level *agent.ThinkingLevel
		if options.CLIThinking == "" {
			if prefix, suffix, ok := thinkingSuffix(pattern); ok {
				pattern = prefix
				level = &suffix
			}
		}
		model := candidates[0]
		for _, candidate := range headlessDefaultModels {
			if candidate[0] == provider {
				for _, m := range candidates {
					if m.ID == candidate[1] {
						model = m
						break
					}
				}
				break
			}
		}
		model = cloneRuntimeModels([]ai.Model{model})[0]
		model.ID = pattern
		model.Name = pattern
		requested := options.CLIThinking
		if requested == "" && level != nil {
			requested = *level
		}
		if requested != "" && requested != "off" {
			model.Reasoning = true
		}
		warning := fmt.Sprintf("Model %q not found for provider %q. Using custom model id.", pattern, provider)
		return ResolveCLIModelResult{Model: &model, ThinkingLevel: level, Warning: &warning}, nil
	}
	return fail(fmt.Sprintf("Model %q not found. Use --list-models to see available models.", options.CLIModel))
}

var datedModelID = regexp.MustCompile(`-\d{8}$`)

func matchModel(pattern string, models []ai.Model) *ai.Model {
	normalized := strings.ToLower(strings.TrimSpace(pattern))
	if normalized == "" {
		return nil
	}
	for _, m := range models {
		if strings.ToLower(string(m.Provider)+"/"+m.ID) == normalized {
			return &m
		}
	}
	exact := []ai.Model{}
	for _, m := range models {
		if strings.ToLower(m.ID) == normalized {
			exact = append(exact, m)
		}
	}
	if len(exact) == 1 {
		return &exact[0]
	}
	matches := []ai.Model{}
	for _, m := range models {
		if strings.Contains(strings.ToLower(m.ID), strings.ToLower(pattern)) || strings.Contains(strings.ToLower(m.Name), strings.ToLower(pattern)) {
			matches = append(matches, m)
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		a, b := datedModelID.MatchString(matches[i].ID), datedModelID.MatchString(matches[j].ID)
		if a != b {
			return !a
		}
		return matches[i].ID > matches[j].ID
	})
	if len(matches) > 0 {
		return &matches[0]
	}
	return nil
}

func thinkingSuffix(pattern string) (string, agent.ThinkingLevel, bool) {
	index := strings.LastIndexByte(pattern, ':')
	if index < 0 {
		return pattern, "", false
	}
	level := agent.ThinkingLevel(pattern[index+1:])
	switch level {
	case "off", "minimal", "low", "medium", "high", "xhigh", "max":
		return pattern[:index], level, true
	}
	return pattern, "", false
}

func parseModelPattern(pattern string, models []ai.Model, allowInvalid bool) ResolveCLIModelResult {
	if m := matchModel(pattern, models); m != nil {
		return ResolveCLIModelResult{Model: m}
	}
	index := strings.LastIndexByte(pattern, ':')
	if index < 0 {
		return ResolveCLIModelResult{}
	}
	prefix, level, valid := thinkingSuffix(pattern)
	if valid {
		result := parseModelPattern(prefix, models, allowInvalid)
		if result.Model != nil && result.Warning == nil {
			result.ThinkingLevel = &level
		}
		return result
	}
	if !allowInvalid {
		return ResolveCLIModelResult{}
	}
	result := parseModelPattern(pattern[:index], models, true)
	if result.Model != nil {
		warning := fmt.Sprintf("Invalid thinking level %q in pattern %q. Using default instead.", pattern[index+1:], pattern)
		result.ThinkingLevel = nil
		result.Warning = &warning
	}
	return result
}

func ResolveModelScopeWithDiagnostics(ctx context.Context, patterns []string, runtime *ModelRuntime) (ResolveModelScopeResult, error) {
	models, err := runtime.GetAvailable(ctx)
	if err != nil {
		return ResolveModelScopeResult{}, err
	}
	result := ResolveModelScopeResult{ScopedModels: []ScopedModel{}, Diagnostics: []ModelScopeDiagnostic{}}
	seen := map[string]bool{}
	add := func(model ai.Model, level *agent.ThinkingLevel) {
		id := string(model.Provider) + "/" + model.ID
		if !seen[id] {
			seen[id] = true
			thinking := agent.ThinkingLevel("")
			if level != nil {
				thinking = *level
			}
			result.ScopedModels = append(result.ScopedModels, ScopedModel{Model: model, ThinkingLevel: thinking})
		}
	}
	for _, pattern := range patterns {
		matched := false
		if strings.ContainsAny(pattern, "*?[") {
			glob := pattern
			var level *agent.ThinkingLevel
			if prefix, suffix, ok := thinkingSuffix(pattern); ok {
				glob = prefix
				level = &suffix
			}
			for _, m := range models {
				full, _ := path.Match(strings.ToLower(glob), strings.ToLower(string(m.Provider)+"/"+m.ID))
				bare, _ := path.Match(strings.ToLower(glob), strings.ToLower(m.ID))
				if full || bare {
					matched = true
					add(m, level)
				}
			}
		} else {
			parsed := parseModelPattern(pattern, models, true)
			if parsed.Warning != nil {
				result.Diagnostics = append(result.Diagnostics, ModelScopeDiagnostic{Type: "warning", Code: "invalid-thinking-level", Message: *parsed.Warning, Pattern: pattern})
			}
			if parsed.Model != nil {
				matched = true
				add(*parsed.Model, parsed.ThinkingLevel)
			}
		}
		if !matched {
			result.Diagnostics = append(result.Diagnostics, ModelScopeDiagnostic{Type: "warning", Code: "no-match", Message: fmt.Sprintf("No models match pattern %q", pattern), Pattern: pattern})
		}
	}
	return result, nil
}
