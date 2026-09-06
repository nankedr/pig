package codingagent

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf16"
)

func normalizeEditFuzzy(text string) string {
	lines := strings.Split(normalizeEditNFKC(text), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRightFunc(line, func(r rune) bool {
			return r == '\t' || r == '\v' || r == '\f' || r == '\r' || r == ' ' || r == '\u00a0' || r == '\u1680' || r >= '\u2000' && r <= '\u200a' || r == '\u2028' || r == '\u2029' || r == '\u202f' || r == '\u205f' || r == '\u3000' || r == '\ufeff'
		})
	}
	return strings.Map(func(r rune) rune {
		switch {
		case r >= '\u2018' && r <= '\u201b':
			return '\''
		case r >= '\u201c' && r <= '\u201f':
			return '"'
		case r >= '\u2010' && r <= '\u2015' || r == '\u2212':
			return '-'
		case r == '\u00a0' || r >= '\u2002' && r <= '\u200a' || r == '\u202f' || r == '\u205f' || r == '\u3000':
			return ' '
		}
		return r
	}, strings.Join(lines, "\n"))
}

type matchedFileEdit struct {
	index, start, length int
	text                 string
}

func applyFileEdits(content string, edits []Edit, path string) (string, error) {
	normalized := make([]Edit, len(edits))
	fuzzy := false
	for i, edit := range edits {
		normalized[i] = Edit{OldText: normalizeEditLF(edit.OldText), NewText: normalizeEditLF(edit.NewText)}
		if normalized[i].OldText == "" {
			if len(edits) == 1 {
				return "", fmt.Errorf("oldText must not be empty in %s.", path)
			}
			return "", fmt.Errorf("edits[%d].oldText must not be empty in %s.", i, path)
		}
	}
	for _, edit := range normalized {
		if !strings.Contains(content, edit.OldText) && strings.Contains(normalizeEditFuzzy(content), normalizeEditFuzzy(edit.OldText)) {
			fuzzy = true
		}
	}
	base := content
	if fuzzy {
		base = normalizeEditFuzzy(content)
	}
	matches := make([]matchedFileEdit, 0, len(edits))
	for i, edit := range normalized {
		start, length := strings.Index(base, edit.OldText), len(edit.OldText)
		if start < 0 {
			old := normalizeEditFuzzy(edit.OldText)
			start, length = strings.Index(normalizeEditFuzzy(base), old), len(old)
		}
		if start < 0 {
			if len(edits) == 1 {
				return "", fmt.Errorf("Could not find the exact text in %s. The old text must match exactly including all whitespace and newlines.", path)
			}
			return "", fmt.Errorf("Could not find edits[%d] in %s. The oldText must match exactly including all whitespace and newlines.", i, path)
		}
		fuzzyBase, old := normalizeEditFuzzy(base), normalizeEditFuzzy(edit.OldText)
		occurrences := strings.Count(fuzzyBase, old)
		if old == "" {
			occurrences = len(utf16.Encode([]rune(fuzzyBase))) - 1
		}
		if occurrences > 1 {
			if len(edits) == 1 {
				return "", fmt.Errorf("Found %d occurrences of the text in %s. The text must be unique. Please provide more context to make it unique.", occurrences, path)
			}
			return "", fmt.Errorf("Found %d occurrences of edits[%d] in %s. Each oldText must be unique. Please provide more context to make it unique.", occurrences, i, path)
		}
		matches = append(matches, matchedFileEdit{index: i, start: start, length: length, text: edit.NewText})
	}
	slices.SortStableFunc(matches, func(a, b matchedFileEdit) int { return a.start - b.start })
	for i := 1; i < len(matches); i++ {
		if matches[i-1].start+matches[i-1].length > matches[i].start {
			return "", fmt.Errorf("edits[%d] and edits[%d] overlap in %s. Merge them into one edit or target disjoint regions.", matches[i-1].index, matches[i].index, path)
		}
	}
	result := base
	if fuzzy {
		var err error
		result, err = preserveUnchangedEditLines(content, base, matches)
		if err != nil {
			return "", err
		}
	} else {
		result = replaceFileEdits(base, matches, 0)
	}
	if result == content {
		if len(edits) == 1 {
			return "", fmt.Errorf("No changes made to %s. The replacement produced identical content. This might indicate an issue with special characters or the text not existing as expected.", path)
		}
		return "", fmt.Errorf("No changes made to %s. The replacements produced identical content.", path)
	}
	return result, nil
}

func replaceFileEdits(content string, matches []matchedFileEdit, offset int) string {
	for i := len(matches) - 1; i >= 0; i-- {
		edit := matches[i]
		start := edit.start - offset
		content = content[:start] + edit.text + content[start+edit.length:]
	}
	return content
}

func preserveUnchangedEditLines(original, base string, matches []matchedFileEdit) (string, error) {
	originalLines, baseLines := editLines(original), editLines(base)
	if len(originalLines) != len(baseLines) {
		return "", fmt.Errorf("Cannot preserve unchanged lines because the base content has a different line count.")
	}
	offsets := make([]int, len(baseLines)+1)
	for i, line := range baseLines {
		offsets[i+1] = offsets[i] + len(line)
	}
	type group struct {
		start, end int
		edits      []matchedFileEdit
	}
	var groups []group
	for _, edit := range matches {
		start := 0
		for start < len(baseLines) && offsets[start+1] <= edit.start {
			start++
		}
		end := start
		for end < len(baseLines) && offsets[end+1] < edit.start+edit.length {
			end++
		}
		if start >= len(baseLines) || end >= len(baseLines) {
			return "", fmt.Errorf("Replacement range is outside the base content.")
		}
		if len(groups) > 0 && start < groups[len(groups)-1].end {
			current := &groups[len(groups)-1]
			current.end = max(current.end, end+1)
			current.edits = append(current.edits, edit)
		} else {
			groups = append(groups, group{start: start, end: end + 1, edits: []matchedFileEdit{edit}})
		}
	}
	var result strings.Builder
	line := 0
	for _, group := range groups {
		for _, text := range originalLines[line:group.start] {
			result.WriteString(text)
		}
		result.WriteString(replaceFileEdits(base[offsets[group.start]:offsets[group.end]], group.edits, offsets[group.start]))
		line = group.end
	}
	for _, text := range originalLines[line:] {
		result.WriteString(text)
	}
	return result.String(), nil
}
