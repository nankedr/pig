package parity

import (
	"errors"
	"fmt"
	"strings"
)

type NormalizationKind string

const NormalizationBrandToken NormalizationKind = "brand-token"

var (
	ErrInvalidNormalization    = errors.New("parity: invalid normalization")
	ErrNormalizationMatchCount = errors.New("parity: normalization match count differs")
)

type Normalization struct {
	Target       string            `json:"target"`
	Kind         NormalizationKind `json:"kind"`
	Oracle       string            `json:"oracle"`
	Pig          string            `json:"pig"`
	ExactMatches int               `json:"exact_matches"`
}

type NormalizationApplication struct {
	Target  string `json:"target"`
	Matches int    `json:"matches"`
}

func applyNormalizations(rules []Normalization, oracle, pig Observation) (Observation, Observation, []NormalizationApplication, error) {
	normalizedOracle, normalizedPig := oracle, pig
	applications := make([]NormalizationApplication, 0, len(rules))
	for _, rule := range rules {
		if normalizedOracle.Stdout == nil {
			return Observation{}, Observation{}, nil, fmt.Errorf("%w: %s is unobserved", ErrNormalizationMatchCount, rule.Target)
		}
		replaced, matches := replaceASCIIWord(*normalizedOracle.Stdout, rule.Oracle, rule.Pig)
		if matches != rule.ExactMatches {
			return Observation{}, Observation{}, nil, fmt.Errorf("%w: %s matched %d, want %d", ErrNormalizationMatchCount, rule.Target, matches, rule.ExactMatches)
		}
		normalizedOracle.Stdout = &replaced
		applications = append(applications, NormalizationApplication{Target: rule.Target, Matches: matches})
	}
	return normalizedOracle, normalizedPig, applications, nil
}

func validateNormalizations(rules []Normalization) error {
	seen := make(map[string]bool, len(rules))
	for _, rule := range rules {
		if rule.Target != "/stdout" || rule.Kind != NormalizationBrandToken || rule.Oracle != "pi" || rule.Pig != "pig" || rule.ExactMatches < 1 || seen[rule.Target] {
			return fmt.Errorf("%w: %+v", ErrInvalidNormalization, rule)
		}
		seen[rule.Target] = true
	}
	return nil
}

func replaceASCIIWord(value, old, replacement string) (string, int) {
	var result strings.Builder
	start, matches := 0, 0
	for {
		index := strings.Index(value[start:], old)
		if index < 0 {
			result.WriteString(value[start:])
			break
		}
		index += start
		end := index + len(old)
		if (index == 0 || !isASCIIWordByte(value[index-1])) && (end == len(value) || !isASCIIWordByte(value[end])) {
			result.WriteString(value[start:index])
			result.WriteString(replacement)
			start = end
			matches++
			continue
		}
		result.WriteString(value[start:end])
		start = end
	}
	return result.String(), matches
}

func isASCIIWordByte(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= 'A' && value <= 'Z' || value >= '0' && value <= '9' || value == '_' || value == '-'
}
