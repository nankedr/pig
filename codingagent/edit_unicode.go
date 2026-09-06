package codingagent

import (
	"slices"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// ECMAScript NFKC does not insert x/text's stream-safe grapheme joiners.
func normalizeEditNFKC(text string) string {
	type character struct {
		value rune
		ccc   uint8
	}
	var decomposed []character
	for _, r := range text {
		value, ok := editUnicode16Decomposition[r]
		if !ok {
			value = norm.NFKD.String(string(r))
		}
		for _, part := range value {
			ccc, ok := editUnicode16CCC[part]
			if !ok {
				ccc = norm.NFKD.PropertiesString(string(part)).CCC()
			}
			decomposed = append(decomposed, character{part, ccc})
		}
	}
	start := 0
	for i := 0; i <= len(decomposed); i++ {
		if i == len(decomposed) || decomposed[i].ccc == 0 {
			slices.SortStableFunc(decomposed[start:i], func(a, b character) int { return int(a.ccc) - int(b.ccc) })
			start = i + 1
		}
	}
	composed := decomposed[:0]
	starter, lastCCC := -1, uint8(0)
	for _, char := range decomposed {
		if starter >= 0 && (lastCCC == 0 || lastCCC < char.ccc) {
			pair := [2]rune{composed[starter].value, char.value}
			combined := editUnicode16Composition[pair]
			if combined == 0 && !norm.NFC.PropertiesString(string(char.value)).BoundaryBefore() {
				value := norm.NFC.String(string(pair[:]))
				if utf8.RuneCountInString(value) == 1 {
					combined, _ = utf8.DecodeRuneInString(value)
				}
			}
			if combined != 0 {
				composed[starter].value = combined
				continue
			}
		}
		if char.ccc == 0 {
			starter = len(composed)
		}
		composed = append(composed, char)
		lastCCC = char.ccc
	}
	var result strings.Builder
	for _, char := range composed {
		result.WriteRune(char.value)
	}
	return result.String()
}
