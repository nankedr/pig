package tui

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

func completionBefore(lines []string, line, col int) (string, error) {
	if line < 0 || line >= len(lines) || col < 0 || col > len(lines[line]) || !utf8.ValidString(lines[line][:col]) {
		return "", errors.New("invalid autocomplete cursor")
	}
	return lines[line][:col], nil
}
func completionPrefix(text string) string {
	quote := -1
	for i, r := range text {
		if r == '"' {
			if quote < 0 {
				quote = i
			} else {
				quote = -1
			}
		}
	}
	if quote >= 0 {
		start := quote
		if start > 0 && text[start-1] == '@' {
			start--
		}
		if start == 0 || strings.ContainsRune(" \t\"'=", rune(text[start-1])) {
			return text[start:]
		}
	}
	return text[strings.LastIndexAny(text, " \t\"'=")+1:]
}
func (p *CombinedAutocompleteProvider) GetSuggestions(ctx context.Context, lines []string, line, col int, options AutocompleteOptions) (AutocompleteSuggestions, bool, error) {
	before, err := completionBefore(lines, line, col)
	if err != nil {
		return AutocompleteSuggestions{}, false, err
	}
	if err = ctx.Err(); err != nil {
		return AutocompleteSuggestions{}, false, err
	}
	prefix := completionPrefix(before)
	var items []AutocompleteItem
	switch {
	case strings.HasPrefix(prefix, "@"):
		items, err = p.fuzzyFiles(ctx, prefix)
	case !options.Force && strings.HasPrefix(before, "/"):
		prefix = before
		name, args, hasArgs := strings.Cut(before[1:], " ")
		if hasArgs {
			prefix = args
			for _, entry := range p.commands {
				if c, ok := entry.(SlashCommand); ok && c.Name == name {
					if c.GetArgumentCompletions != nil {
						items, _, err = c.GetArgumentCompletions(ctx, args)
					}
					break
				}
			}
		} else {
			type scored struct {
				item  AutocompleteItem
				score float64
			}
			var matches []scored
			for _, entry := range p.commands {
				var item AutocompleteItem
				switch c := entry.(type) {
				case AutocompleteItem:
					item = AutocompleteItem{Value: c.Value, Label: c.Value, Description: c.Description}
				case SlashCommand:
					item = AutocompleteItem{Value: c.Name, Label: c.Name, Description: c.Description}
					if c.ArgumentHint != nil && *c.ArgumentHint != "" {
						desc := *c.ArgumentHint
						if c.Description != nil && *c.Description != "" {
							desc += " — " + *c.Description
						}
						item.Description = &desc
					}
				}
				if score, ok := completionScore(name, item.Value); ok {
					matches = append(matches, scored{item, score})
				}
			}
			slices.SortStableFunc(matches, func(a, b scored) int {
				if a.score < b.score {
					return -1
				}
				if a.score > b.score {
					return 1
				}
				return 0
			})
			for _, m := range matches {
				items = append(items, m.item)
			}
		}
	case options.Force || strings.HasPrefix(prefix, "\"") || strings.Contains(prefix, "/") || strings.HasPrefix(prefix, ".") || prefix == "" && strings.HasSuffix(before, " "):
		items = p.files(prefix)
	}
	if err == nil {
		err = ctx.Err()
	}
	return AutocompleteSuggestions{Items: items, Prefix: prefix}, len(items) > 0 && err == nil, err
}
func (p *CombinedAutocompleteProvider) ApplyCompletion(lines []string, line, col int, item AutocompleteItem, prefix string) (AutocompleteResult, error) {
	before, err := completionBefore(lines, line, col)
	if err != nil {
		return AutocompleteResult{}, err
	}
	if !strings.HasSuffix(before, prefix) {
		return AutocompleteResult{}, errors.New("autocomplete prefix no longer matches cursor")
	}
	before = strings.TrimSuffix(before, prefix)
	after := lines[line][col:]
	if (strings.HasPrefix(prefix, "\"") || strings.HasPrefix(prefix, "@\"")) && strings.HasSuffix(item.Value, "\"") && strings.HasPrefix(after, "\"") {
		after = after[1:]
	}
	value := item.Value
	offset := len(value)
	if strings.HasPrefix(prefix, "/") && strings.TrimSpace(before) == "" && !strings.Contains(prefix[1:], "/") {
		value = "/" + value + " "
		offset = len(value)
	} else if strings.HasSuffix(item.Label, "/") {
		if strings.HasSuffix(value, "\"") {
			offset--
		}
	} else if strings.HasPrefix(prefix, "@") {
		value += " "
		offset++
	}
	result := AutocompleteResult{Lines: slices.Clone(lines), CursorLine: line, CursorCol: len(before) + offset}
	result.Lines[line] = before + value + after
	return result, nil
}
func (*CombinedAutocompleteProvider) ShouldTriggerFileCompletion(lines []string, line, col int) (bool, error) {
	before, err := completionBefore(lines, line, col)
	before = strings.TrimSpace(before)
	return !(strings.HasPrefix(before, "/") && !strings.Contains(before, " ")), err
}
func completionValue(path string, at, quoted bool) string {
	if quoted || strings.Contains(path, " ") {
		path = "\"" + path + "\""
	}
	if at {
		path = "@" + path
	}
	return path
}
func completionPath(prefix string) (string, bool, bool) {
	at := strings.HasPrefix(prefix, "@")
	if at {
		prefix = prefix[1:]
	}
	quoted := strings.HasPrefix(prefix, "\"")
	return strings.TrimPrefix(prefix, "\""), at, quoted
}
func (p *CombinedAutocompleteProvider) resolvePath(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil {
			return filepath.Join(home, strings.TrimPrefix(path, "~"))
		}
	}
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(p.basePath, path)
}
func (p *CombinedAutocompleteProvider) files(prefix string) []AutocompleteItem {
	raw, at, quoted := completionPath(prefix)
	dir, match := raw, ""
	if raw != "" && raw != "~" && !strings.HasSuffix(raw, "/") {
		dir = filepath.Dir(raw)
		match = filepath.Base(raw)
	}
	entries, err := os.ReadDir(p.resolvePath(dir))
	if err != nil {
		return nil
	}
	var items []AutocompleteItem
	for _, entry := range entries {
		if !strings.HasPrefix(strings.ToLower(entry.Name()), strings.ToLower(match)) {
			continue
		}
		directory := entry.IsDir()
		if entry.Type()&os.ModeSymlink != 0 {
			if info, err := os.Stat(filepath.Join(p.resolvePath(dir), entry.Name())); err == nil {
				directory = info.IsDir()
			}
		}
		display := entry.Name()
		switch {
		case strings.HasSuffix(raw, "/"):
			display = raw + display
		case strings.Contains(raw, "/"):
			display = filepath.ToSlash(filepath.Join(filepath.Dir(raw), display))
			if strings.HasPrefix(raw, "./") {
				display = "./" + display
			}
		case strings.HasPrefix(raw, "~"):
			display = "~/" + display
		}
		label := entry.Name()
		if directory {
			display += "/"
			label += "/"
		}
		items = append(items, AutocompleteItem{Value: completionValue(display, at, quoted), Label: label})
	}
	order := collate.New(language.English)
	slices.SortStableFunc(items, func(a, b AutocompleteItem) int {
		ad, bd := strings.HasSuffix(a.Value, "/"), strings.HasSuffix(b.Value, "/")
		if ad != bd {
			if ad {
				return -1
			}
			return 1
		}
		return order.CompareString(a.Label, b.Label)
	})
	return items
}

var completionLettersDigits = regexp.MustCompile(`^([a-z]+)([0-9]+)$`)
var completionDigitsLetters = regexp.MustCompile(`^([0-9]+)([a-z]+)$`)

func completionScore(query, text string) (float64, bool) {
	match := func(query string) (float64, bool) {
		q, t := utf16.Encode([]rune(strings.ToLower(query))), utf16.Encode([]rune(strings.ToLower(text)))
		if len(q) == 0 {
			return 0, true
		}
		next, last, consecutive, score := 0, -1, 0, 0.0
		for i, c := range t {
			if next == len(q) {
				break
			}
			if c != q[next] {
				continue
			}
			if last == i-1 {
				consecutive++
				score -= float64(consecutive * 5)
			} else {
				consecutive = 0
				if last >= 0 {
					score += float64((i - last - 1) * 2)
				}
			}
			if i == 0 || unicode.IsSpace(rune(t[i-1])) || strings.ContainsRune("-_./:", rune(t[i-1])) {
				score -= 10
			}
			score += float64(i) * 0.1
			last = i
			next++
		}
		if next < len(q) {
			return 0, false
		}
		if strings.EqualFold(query, text) {
			score -= 100
		}
		return score, true
	}
	total := 0.0
	for _, token := range strings.FieldsFunc(query, func(r rune) bool { return unicode.IsSpace(r) || r == '/' }) {
		score, ok := match(token)
		if !ok {
			swapped := ""
			if parts := completionLettersDigits.FindStringSubmatch(strings.ToLower(token)); parts != nil {
				swapped = parts[2] + parts[1]
			} else if parts := completionDigitsLetters.FindStringSubmatch(strings.ToLower(token)); parts != nil {
				swapped = parts[2] + parts[1]
			}
			if swapped != "" {
				score, ok = match(swapped)
				score += 5
			}
		}
		if !ok {
			return 0, false
		}
		total += score
	}
	return total, true
}
func (p *CombinedAutocompleteProvider) fuzzyFiles(ctx context.Context, prefix string) ([]AutocompleteItem, error) {
	if p.fdPath == nil || *p.fdPath == "" {
		return nil, nil
	}
	raw, _, quoted := completionPath(prefix)
	base, query, display := p.basePath, raw, ""
	if slash := strings.LastIndex(raw, "/"); slash >= 0 {
		dir := raw[:slash+1]
		if info, err := os.Stat(p.resolvePath(dir)); err == nil && info.IsDir() {
			base = p.resolvePath(dir)
			query = raw[slash+1:]
			display = dir
		}
	}
	args := []string{"--base-directory", base, "--max-results", "100", "--type", "f", "--type", "d", "--follow", "--hidden", "--exclude", ".git", "--exclude", ".git/*", "--exclude", ".git/**"}
	pattern := query
	if strings.Contains(query, "/") {
		args = append(args, "--full-path")
		parts := strings.FieldsFunc(strings.Trim(query, "/"), func(r rune) bool { return r == '/' })
		for i := range parts {
			parts[i] = regexp.QuoteMeta(parts[i])
		}
		if len(parts) > 0 {
			pattern = strings.Join(parts, `[\\/]`)
			if strings.HasSuffix(query, "/") {
				pattern += `[\\/]`
			}
		}
	}
	if query != "" {
		args = append(args, pattern)
	}
	cmd := exec.CommandContext(ctx, *p.fdPath, args...)
	cmd.WaitDelay = 100 * time.Millisecond
	output, err := cmd.Output()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, nil
	}
	type scored struct {
		item  AutocompleteItem
		score int
	}
	var entries []scored
	for _, path := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		path = strings.ReplaceAll(path, `\`, "/")
		directory := strings.HasSuffix(path, "/")
		path = strings.TrimSuffix(path, "/")
		if path == "" || path == ".git" || strings.HasPrefix(path, ".git/") || strings.Contains(path, "/.git/") {
			continue
		}
		name := filepath.Base(path)
		score := 1
		if query != "" {
			file, q := strings.ToLower(name), strings.ToLower(query)
			switch {
			case file == q:
				score = 100
			case strings.HasPrefix(file, q):
				score = 80
			case strings.Contains(file, q):
				score = 50
			case strings.Contains(strings.ToLower(path), q):
				score = 30
			default:
				continue
			}
			if directory {
				score += 10
			}
		}
		description := display + path
		value, label := description, name
		if directory {
			value += "/"
			label += "/"
		}
		entries = append(entries, scored{AutocompleteItem{Value: completionValue(value, true, quoted), Label: label, Description: &description}, score})
	}
	slices.SortStableFunc(entries, func(a, b scored) int { return b.score - a.score })
	items := make([]AutocompleteItem, 0, min(20, len(entries)))
	for _, entry := range entries[:min(20, len(entries))] {
		items = append(items, entry.item)
	}
	return items, nil
}
