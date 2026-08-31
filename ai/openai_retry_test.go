package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestFetchOpenAICompletionsDoesNotRetryRequestConstructionError(t *testing.T) {
	retries := 1
	calls := 0
	request := FetchRequest{URL: "://invalid", Method: http.MethodPost}
	_, _, err := fetchOpenAICompletions(context.Background(), func(ctx context.Context, request FetchRequest) (FetchResponse, error) {
		calls++
		return defaultOpenAIFetch(ctx, request)
	}, request, &retries, nil)
	if err == nil || calls != 1 {
		t.Fatalf("fetchOpenAICompletions() error = %v, calls = %d, want request error after one call", err, calls)
	}
}

func TestOpenAIRetryDelayMatchesParityCases(t *testing.T) {
	fixture := loadOpenAIRetryFixture(t)
	now := time.Date(2026, time.August, 31, 0, 0, 0, 0, time.UTC)
	status := http.StatusTooManyRequests
	for _, test := range []struct {
		name       string
		caseID     string
		delayIndex int
		headers    map[string]string
		retryIndex int
		random     float64
	}{
		{name: "retry-after-ms precedence", caseID: "retry_after_ms_precedence", headers: map[string]string{"retry-after-ms": "125", "retry-after": "9"}},
		{name: "retry-after seconds", caseID: "retry_after_seconds", headers: map[string]string{"retry-after": "1.5"}},
		{name: "retry-after date", caseID: "retry_after_date", headers: map[string]string{"retry-after": now.Add(2 * time.Second).Format(http.TimeFormat)}},
		{name: "invalid milliseconds fallback", caseID: "invalid_ms_fallback", headers: map[string]string{"retry-after-ms": "NaN"}},
		{name: "exponential", caseID: "exponential_jitter", delayIndex: 1, retryIndex: 1, random: .4},
		{name: "jitter preserves fractional milliseconds", caseID: "fractional_jitter", random: .333},
		{name: "eight second cap before jitter", caseID: "eight_second_cap", delayIndex: 5, retryIndex: 10, random: .8},
	} {
		t.Run(test.name, func(t *testing.T) {
			want := durationFromFixture(t, fixture.Actual.Delay[test.caseID].DelaysMS[test.delayIndex])
			got, err := openAIRetryDelay(&openAIProviderError{status: &status, headers: test.headers, message: "rate limited"}, test.retryIndex, nil, now, test.random)
			if err != nil || got != want {
				t.Fatalf("openAIRetryDelay() = (%v, %v), want fixture %v", got, err, want)
			}
		})
	}
}

func TestOpenAIRetryDelayCapsServerHints(t *testing.T) {
	fixture := loadOpenAIRetryFixture(t)
	status := http.StatusTooManyRequests
	now := time.Unix(0, 0)
	configured := int64(1_000)
	disabled := int64(0)
	for _, test := range []struct {
		name       string
		caseID     string
		header     string
		configured *int64
		wantDelay  bool
		wantError  bool
	}{
		{name: "default cap", caseID: "default", header: "61", wantError: true},
		{name: "configured equal", caseID: "configured_equal", header: "1", configured: &configured, wantDelay: true},
		{name: "configured exceeded", caseID: "configured_exceeded", header: "1.001", configured: &configured, wantError: true},
		{name: "disabled", caseID: "disabled", header: "277403", configured: &disabled, wantDelay: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			observation := fixture.Actual.Cap[test.caseID]
			got, err := openAIRetryDelay(&openAIProviderError{status: &status, headers: map[string]string{"retry-after": test.header}, message: "rate limited"}, 0, test.configured, now, 0)
			if test.wantError {
				if err == nil || !strings.Contains(err.Error(), observation.Outcome.Message) {
					t.Fatalf("openAIRetryDelay() error = %v, want fixture %q", err, observation.Outcome.Message)
				}
				return
			}
			want := durationFromFixture(t, observation.DelaysMS[0])
			if !test.wantDelay || err != nil || got != want {
				t.Fatalf("openAIRetryDelay() = (%v, %v), want fixture %v", got, err, want)
			}
		})
	}
}

type openAIRetryFixtureObservation struct {
	DelaysMS []float64 `json:"delays_ms"`
	Outcome  struct {
		Message string `json:"message"`
	} `json:"outcome"`
}

type openAIRetryFixture struct {
	BaselineCommit string `json:"baseline_commit"`
	Deterministic  bool   `json:"deterministic"`
	Actual         struct {
		Delay map[string]openAIRetryFixtureObservation `json:"delay"`
		Cap   map[string]openAIRetryFixtureObservation `json:"cap"`
	} `json:"actual"`
}

func loadOpenAIRetryFixture(t *testing.T) openAIRetryFixture {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	data, err := os.ReadFile(filepath.Join(filepath.Dir(file), "..", "parity", "oracle", "fixtures", "openai-completions-retry.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture openAIRetryFixture
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	if !fixture.Deterministic || fixture.BaselineCommit != "936aff00918de1187f085f123c2812d8f2d67745" {
		t.Fatalf("invalid retry fixture provenance: %#v", fixture)
	}
	return fixture
}

func durationFromFixture(t *testing.T, milliseconds float64) time.Duration {
	t.Helper()
	return time.Duration(milliseconds * float64(time.Millisecond))
}
