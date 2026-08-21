package ai_test

import (
	"math"
	"reflect"
	"testing"

	"github.com/nankedr/pig/ai"
)

var (
	_ func(ai.Model, ai.API) bool                                 = ai.HasAPI
	_ func(ai.Model, *ai.Usage) ai.UsageCost                      = ai.CalculateCost
	_ func(ai.Model) []ai.ModelThinkingLevel                      = ai.GetSupportedThinkingLevels
	_ func(ai.Model, ai.ModelThinkingLevel) ai.ModelThinkingLevel = ai.ClampThinkingLevel
	_ func(*ai.Model, *ai.Model) bool                             = ai.ModelsAreEqual
)

func TestHasAPIMatchesOnlyTheModelsAPI(t *testing.T) {
	t.Parallel()

	model := ai.Model{API: ai.APIAnthropicMessages}
	if !ai.HasAPI(model, ai.APIAnthropicMessages) {
		t.Fatal("HasAPI(model, matching API) = false, want true")
	}
	if ai.HasAPI(model, ai.APIOpenAIResponses) {
		t.Fatal("HasAPI(model, different API) = true, want false")
	}
}

func TestCalculateCostUsesBaseRatesAndUpdatesUsage(t *testing.T) {
	t.Parallel()

	model := ai.Model{Cost: ai.ModelCost{ModelCostRates: ai.ModelCostRates{
		Input: 1, Output: 2, CacheRead: 0.5, CacheWrite: 1.5,
	}}}
	usage := &ai.Usage{
		Input: 1_000_000, Output: 500_000, CacheRead: 200_000, CacheWrite: 100_000,
		Cost: ai.UsageCost{Input: 99, Output: 99, CacheRead: 99, CacheWrite: 99, Total: 495},
	}
	want := ai.UsageCost{Input: 1, Output: 1, CacheRead: 0.1, CacheWrite: 0.15, Total: 2.25}

	got := ai.CalculateCost(model, usage)

	assertUsageCostClose(t, got, want)
	assertUsageCostClose(t, usage.Cost, want)
}

func TestCalculateCostTreatsNilUsageAsZeroCost(t *testing.T) {
	t.Parallel()

	if got := ai.CalculateCost(ai.Model{}, nil); got != (ai.UsageCost{}) {
		t.Fatalf("CalculateCost(model, nil) = %#v, want zero cost", got)
	}
}

func TestCalculateCostSelectsTheHighestStrictlyMatchedTier(t *testing.T) {
	t.Parallel()

	rates := func(marker float64) ai.ModelCostRates {
		return ai.ModelCostRates{
			Input: marker * 1_000_000, Output: (marker + 1) * 1_000_000,
			CacheRead: (marker + 2) * 1_000_000, CacheWrite: (marker + 3) * 1_000_000,
		}
	}
	model := ai.Model{Cost: ai.ModelCost{
		ModelCostRates: rates(1),
		Tiers: []ai.ModelCostTier{
			{ModelCostRates: rates(31), InputTokensAbove: 300},
			{ModelCostRates: rates(11), InputTokensAbove: 100},
			{ModelCostRates: rates(21), InputTokensAbove: 200},
		},
	}}
	// The tier input is exactly 300. Output must not push it above the 300
	// threshold, so the highest strictly matched threshold is 200.
	usage := &ai.Usage{Input: 100, Output: 1, CacheRead: 100, CacheWrite: 100}
	want := ai.UsageCost{Input: 2_100, Output: 22, CacheRead: 2_300, CacheWrite: 2_400, Total: 6_822}

	assertUsageCostClose(t, ai.CalculateCost(model, usage), want)
}

func TestCalculateCostPricesOneHourCacheWritesAtTwiceInputRate(t *testing.T) {
	t.Parallel()

	model := ai.Model{Cost: ai.ModelCost{ModelCostRates: ai.ModelCostRates{
		Input: 1, CacheWrite: 3,
	}, Tiers: []ai.ModelCostTier{{
		InputTokensAbove: 0,
		ModelCostRates:   ai.ModelCostRates{Input: 2, CacheWrite: 4},
	}}}}
	usage := &ai.Usage{
		CacheWrite:   1_000_000,
		CacheWrite1H: ai.Some[int64](250_000),
	}
	// At the selected tier, 750k short writes cost 4/M and the 250k one-hour
	// subset costs twice the tier's 2/M input rate.
	want := ai.UsageCost{CacheWrite: 4, Total: 4}

	assertUsageCostClose(t, ai.CalculateCost(model, usage), want)
}

func TestGetSupportedThinkingLevelsRespectsReasoningAndMappingStates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		model ai.Model
		want  []ai.ModelThinkingLevel
	}{
		{
			name: "non-reasoning ignores mappings",
			model: ai.Model{ThinkingLevelMap: ai.ThinkingLevelMap{
				ai.ModelThinkingLevelXHigh: ai.Some("xhigh"),
			}},
			want: []ai.ModelThinkingLevel{ai.ModelThinkingLevelOff},
		},
		{
			name:  "reasoning defaults through high",
			model: ai.Model{Reasoning: true},
			want: []ai.ModelThinkingLevel{
				ai.ModelThinkingLevelOff, ai.ModelThinkingLevelMinimal, ai.ModelThinkingLevelLow,
				ai.ModelThinkingLevelMedium, ai.ModelThinkingLevelHigh,
			},
		},
		{
			name: "null excludes and empty non-null opts in",
			model: ai.Model{Reasoning: true, ThinkingLevelMap: ai.ThinkingLevelMap{
				ai.ModelThinkingLevelOff:   ai.Null[string](),
				ai.ModelThinkingLevelLow:   ai.Null[string](),
				ai.ModelThinkingLevelXHigh: ai.Some(""),
				ai.ModelThinkingLevelMax:   ai.Null[string](),
			}},
			want: []ai.ModelThinkingLevel{
				ai.ModelThinkingLevelMinimal, ai.ModelThinkingLevelMedium,
				ai.ModelThinkingLevelHigh, ai.ModelThinkingLevelXHigh,
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ai.GetSupportedThinkingLevels(tt.model); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("GetSupportedThinkingLevels() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClampThinkingLevelPrefersHigherSupportedLevels(t *testing.T) {
	t.Parallel()

	model := ai.Model{Reasoning: true, ThinkingLevelMap: ai.ThinkingLevelMap{
		ai.ModelThinkingLevelLow:     ai.Null[string](),
		ai.ModelThinkingLevelMedium:  ai.Null[string](),
		ai.ModelThinkingLevelXHigh:   ai.Some("xhigh"),
		ai.ModelThinkingLevelMax:     ai.Null[string](),
		ai.ModelThinkingLevelMinimal: ai.Some("minimal"),
	}}

	tests := []struct {
		name      string
		requested ai.ModelThinkingLevel
		want      ai.ModelThinkingLevel
	}{
		{name: "supported stays unchanged", requested: ai.ModelThinkingLevelHigh, want: ai.ModelThinkingLevelHigh},
		{name: "unsupported prefers upward", requested: ai.ModelThinkingLevelMedium, want: ai.ModelThinkingLevelHigh},
		{name: "fixed-sequence unknown uses first supported", requested: ai.ModelThinkingLevel("turbo"), want: ai.ModelThinkingLevelOff},
	}
	for _, tt := range tests {
		if got := ai.ClampThinkingLevel(model, tt.requested); got != tt.want {
			t.Errorf("%s: ClampThinkingLevel(%q) = %q, want %q", tt.name, tt.requested, got, tt.want)
		}
	}

	allNull := make(ai.ThinkingLevelMap, 7)
	for _, level := range []ai.ModelThinkingLevel{
		ai.ModelThinkingLevelOff, ai.ModelThinkingLevelMinimal, ai.ModelThinkingLevelLow,
		ai.ModelThinkingLevelMedium, ai.ModelThinkingLevelHigh, ai.ModelThinkingLevelXHigh, ai.ModelThinkingLevelMax,
	} {
		allNull[level] = ai.Null[string]()
	}
	if got := ai.ClampThinkingLevel(ai.Model{Reasoning: true, ThinkingLevelMap: allNull}, ai.ModelThinkingLevelHigh); got != ai.ModelThinkingLevelOff {
		t.Fatalf("ClampThinkingLevel(all unsupported) = %q, want off fallback", got)
	}
}

func TestModelsAreEqualUsesOnlyProviderAndID(t *testing.T) {
	t.Parallel()

	left := &ai.Model{
		Provider: ai.ProviderIDAnthropic, ID: "model-1", API: ai.APIAnthropicMessages, Name: "left",
	}
	right := &ai.Model{
		Provider: ai.ProviderIDAnthropic, ID: "model-1", API: ai.APIOpenAIResponses, Name: "right",
	}
	if !ai.ModelsAreEqual(left, right) {
		t.Fatal("ModelsAreEqual(same provider and ID) = false, want true despite other differences")
	}
	if ai.ModelsAreEqual(left, &ai.Model{Provider: ai.ProviderIDOpenAI, ID: left.ID}) {
		t.Fatal("ModelsAreEqual(different provider) = true, want false")
	}
	if ai.ModelsAreEqual(left, &ai.Model{Provider: left.Provider, ID: "model-2"}) {
		t.Fatal("ModelsAreEqual(different ID) = true, want false")
	}
	if ai.ModelsAreEqual(nil, nil) || ai.ModelsAreEqual(left, nil) || ai.ModelsAreEqual(nil, right) {
		t.Fatal("ModelsAreEqual with a nil operand = true, want false")
	}
}

func assertUsageCostClose(t *testing.T, got, want ai.UsageCost) {
	t.Helper()

	const tolerance = 1e-12
	closeEnough := func(actual, expected float64) bool {
		return !math.IsNaN(actual) && math.Abs(actual-expected) <= tolerance
	}
	if !closeEnough(got.Input, want.Input) ||
		!closeEnough(got.Output, want.Output) ||
		!closeEnough(got.CacheRead, want.CacheRead) ||
		!closeEnough(got.CacheWrite, want.CacheWrite) ||
		!closeEnough(got.Total, want.Total) {
		t.Fatalf("usage cost = %#v, want %#v within %g", got, want, tolerance)
	}
}
