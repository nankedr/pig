package tui

import (
	"cmp"
	"slices"
	"strings"
	"unicode"
)

func FuzzyFilter[T any](items []T, query string, getText func(T) string) ([]T, error) {
	tokens := strings.FieldsFunc(query, func(r rune) bool { return r == '/' || unicode.IsSpace(r) })
	if len(tokens) == 0 {
		return slices.Clone(items), nil
	}
	type scored struct {
		item  T
		score float64
	}
	matches := []scored{}
	for _, item := range items {
		score := 0.0
		ok := true
		text := getText(item)
		for _, token := range tokens {
			n, matched := completionScore(token, text)
			if !matched {
				ok = false
				break
			}
			score += n
		}
		if ok {
			matches = append(matches, scored{item, score})
		}
	}
	slices.SortStableFunc(matches, func(a, b scored) int { return cmp.Compare(a.score, b.score) })
	result := make([]T, 0, len(matches))
	for _, m := range matches {
		result = append(result, m.item)
	}
	return result, nil
}
