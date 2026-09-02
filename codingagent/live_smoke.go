package codingagent

import "strings"

// LiveSmokeDecision selects whether the DeepSeek live smoke runs, is skipped
// on an ordinary PR, or must fail a protected gate.
type LiveSmokeDecision string

const (
	LiveSmokeRun  LiveSmokeDecision = "run"
	LiveSmokeSkip LiveSmokeDecision = "skip"
	LiveSmokeFail LiveSmokeDecision = "fail"
)

// DecideLiveSmoke gates the live smoke on a restricted DEEPSEEK_API_KEY. An
// ordinary run without a key skips; a required Freeze or release run without a
// key must fail; a present key always runs.
func DecideLiveSmoke(require bool, key string) LiveSmokeDecision {
	if strings.TrimSpace(key) != "" {
		return LiveSmokeRun
	}
	if require {
		return LiveSmokeFail
	}
	return LiveSmokeSkip
}
