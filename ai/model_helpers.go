package ai

var extendedThinkingLevels = [...]ModelThinkingLevel{
	ModelThinkingLevelOff,
	ModelThinkingLevelMinimal,
	ModelThinkingLevelLow,
	ModelThinkingLevelMedium,
	ModelThinkingLevelHigh,
	ModelThinkingLevelXHigh,
	ModelThinkingLevelMax,
}

// HasAPI reports whether model uses api.
func HasAPI(model Model, api API) bool {
	return model.API == api
}

// CalculateCost calculates model usage cost in dollars and stores it on usage.
// A nil usage is treated as an empty usage report.
func CalculateCost(model Model, usage *Usage) UsageCost {
	if usage == nil {
		return UsageCost{}
	}

	rates := model.Cost.ModelCostRates
	inputTokens := usage.Input + usage.CacheRead + usage.CacheWrite
	matchedThreshold := int64(-1)
	for _, tier := range model.Cost.Tiers {
		if inputTokens > tier.InputTokensAbove && tier.InputTokensAbove > matchedThreshold {
			rates = tier.ModelCostRates
			matchedThreshold = tier.InputTokensAbove
		}
	}
	longWrite, _ := usage.CacheWrite1H.Value()
	shortWrite := usage.CacheWrite - longWrite
	usage.Cost = UsageCost{
		Input:      (rates.Input / 1_000_000) * float64(usage.Input),
		Output:     (rates.Output / 1_000_000) * float64(usage.Output),
		CacheRead:  (rates.CacheRead / 1_000_000) * float64(usage.CacheRead),
		CacheWrite: (rates.CacheWrite*float64(shortWrite) + rates.Input*2*float64(longWrite)) / 1_000_000,
	}
	usage.Cost.Total = usage.Cost.Input + usage.Cost.Output + usage.Cost.CacheRead + usage.Cost.CacheWrite
	return usage.Cost
}

// GetSupportedThinkingLevels returns model thinking levels in increasing order.
func GetSupportedThinkingLevels(model Model) []ModelThinkingLevel {
	if !model.Reasoning {
		return []ModelThinkingLevel{ModelThinkingLevelOff}
	}

	levels := make([]ModelThinkingLevel, 0, len(extendedThinkingLevels))
	for _, level := range extendedThinkingLevels {
		mapped, present := model.ThinkingLevelMap[level]
		if mapped.IsNull() {
			continue
		}
		if level == ModelThinkingLevelXHigh || level == ModelThinkingLevelMax {
			if _, ok := mapped.Value(); !present || !ok {
				continue
			}
		}
		levels = append(levels, level)
	}
	return levels
}

// ClampThinkingLevel returns level when supported, otherwise preferring the
// next higher supported level before falling back to lower levels.
func ClampThinkingLevel(model Model, level ModelThinkingLevel) ModelThinkingLevel {
	available := GetSupportedThinkingLevels(model)
	for _, candidate := range available {
		if candidate == level {
			return level
		}
	}

	requestedIndex := -1
	for index, candidate := range extendedThinkingLevels {
		if candidate == level {
			requestedIndex = index
			break
		}
	}
	if requestedIndex < 0 {
		return firstThinkingLevelOrOff(available)
	}

	for index := requestedIndex; index < len(extendedThinkingLevels); index++ {
		candidate := extendedThinkingLevels[index]
		if containsThinkingLevel(available, candidate) {
			return candidate
		}
	}
	for index := requestedIndex - 1; index >= 0; index-- {
		candidate := extendedThinkingLevels[index]
		if containsThinkingLevel(available, candidate) {
			return candidate
		}
	}
	return firstThinkingLevelOrOff(available)
}

func containsThinkingLevel(levels []ModelThinkingLevel, target ModelThinkingLevel) bool {
	for _, level := range levels {
		if level == target {
			return true
		}
	}
	return false
}

func firstThinkingLevelOrOff(levels []ModelThinkingLevel) ModelThinkingLevel {
	if len(levels) == 0 {
		return ModelThinkingLevelOff
	}
	return levels[0]
}

// ModelsAreEqual compares model identity. Nil models are never equal.
func ModelsAreEqual(left, right *Model) bool {
	if left == nil || right == nil {
		return false
	}
	return left.Provider == right.Provider && left.ID == right.ID
}
