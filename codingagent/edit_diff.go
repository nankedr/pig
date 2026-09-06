package codingagent

import (
	"fmt"
	"slices"
	"strings"
)

type editDiffPart struct {
	kind  byte
	lines []string
}
type editDiffComponent struct {
	kind     byte
	count    int
	previous *editDiffComponent
}
type editDiffPath struct {
	oldPos int
	last   *editDiffComponent
}

func editLines(text string) []string {
	lines := strings.SplitAfter(text, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// The line diff follows jsdiff 8.0.4's Myers paths and tie breaking.
func editLineDiff(oldText, newText string) []editDiffPart {
	oldLines, newLines := editLines(oldText), editLines(newText)
	common := func(path *editDiffPath, diagonal int) int {
		newPos, count := path.oldPos-diagonal, 0
		for path.oldPos+1 < len(oldLines) && newPos+1 < len(newLines) && oldLines[path.oldPos+1] == newLines[newPos+1] {
			path.oldPos++
			newPos++
			count++
		}
		if count > 0 {
			path.last = &editDiffComponent{count: count, previous: path.last}
		}
		return newPos
	}
	build := func(path *editDiffPath) []editDiffPart {
		var components []*editDiffComponent
		for last := path.last; last != nil; last = last.previous {
			components = append(components, last)
		}
		slices.Reverse(components)
		parts := make([]editDiffPart, 0, len(components))
		oldPos, newPos := 0, 0
		for _, component := range components {
			var lines []string
			if component.kind == '-' {
				lines = oldLines[oldPos : oldPos+component.count]
				oldPos += component.count
			} else {
				lines = newLines[newPos : newPos+component.count]
				newPos += component.count
				if component.kind == 0 {
					oldPos += component.count
				}
			}
			parts = append(parts, editDiffPart{component.kind, lines})
		}
		return parts
	}
	start := &editDiffPath{oldPos: -1}
	newPos := common(start, 0)
	if start.oldPos+1 >= len(oldLines) && newPos+1 >= len(newLines) {
		return build(start)
	}
	best := map[int]*editDiffPath{0: start}
	minDiagonal, maxDiagonal := -len(oldLines)-len(newLines), len(oldLines)+len(newLines)
	for distance := 1; distance <= len(oldLines)+len(newLines); distance++ {
		for diagonal := max(minDiagonal, -distance); diagonal <= min(maxDiagonal, distance); diagonal += 2 {
			remove, add := best[diagonal-1], best[diagonal+1]
			delete(best, diagonal-1)
			canAdd := add != nil && add.oldPos-diagonal >= 0 && add.oldPos-diagonal < len(newLines)
			canRemove := remove != nil && remove.oldPos+1 < len(oldLines)
			if !canAdd && !canRemove {
				delete(best, diagonal)
				continue
			}
			source, kind, increment := remove, byte('-'), 1
			if !canRemove || canAdd && remove.oldPos < add.oldPos {
				source, kind, increment = add, '+', 0
			}
			component := &editDiffComponent{kind: kind, count: 1, previous: source.last}
			if source.last != nil && source.last.kind == kind {
				component.count += source.last.count
				component.previous = source.last.previous
			}
			path := &editDiffPath{oldPos: source.oldPos + increment, last: component}
			newPos := common(path, diagonal)
			if path.oldPos+1 >= len(oldLines) && newPos+1 >= len(newLines) {
				return build(path)
			}
			best[diagonal] = path
			if path.oldPos+1 >= len(oldLines) {
				maxDiagonal = min(maxDiagonal, diagonal-1)
			}
			if newPos+1 >= len(newLines) {
				minDiagonal = max(minDiagonal, diagonal+1)
			}
		}
	}
	return nil
}

func editDiffDetails(path, oldText, newText string) EditToolDetails {
	parts := editLineDiff(oldText, newText)
	details := EditToolDetails{Patch: editUnifiedPatch(path, parts)}
	width := len(fmt.Sprint(max(strings.Count(oldText, "\n"), strings.Count(newText, "\n")) + 1))
	oldLine, newLine := 1, 1
	var output []string
	gap := func(count int) {
		if count > 0 {
			output = append(output, " "+strings.Repeat(" ", width)+" ...")
			oldLine += count
			newLine += count
		}
	}
	contextLine := func(line string) {
		output = append(output, fmt.Sprintf(" %*d %s", width, oldLine, strings.TrimSuffix(line, "\n")))
		oldLine++
		newLine++
	}
	for i, part := range parts {
		if part.kind != 0 {
			if details.FirstChangedLine == 0 {
				details.FirstChangedLine = newLine
			}
			for _, line := range part.lines {
				number := oldLine
				if part.kind == '+' {
					number = newLine
					newLine++
				} else {
					oldLine++
				}
				output = append(output, fmt.Sprintf("%c%*d %s", part.kind, width, number, strings.TrimSuffix(line, "\n")))
			}
			continue
		}
		leading, trailing := i > 0 && parts[i-1].kind != 0, i+1 < len(parts) && parts[i+1].kind != 0
		switch {
		case leading && trailing && len(part.lines) > 8:
			for _, line := range part.lines[:4] {
				contextLine(line)
			}
			gap(len(part.lines) - 8)
			for _, line := range part.lines[len(part.lines)-4:] {
				contextLine(line)
			}
		case leading && trailing:
			for _, line := range part.lines {
				contextLine(line)
			}
		case leading:
			for _, line := range part.lines[:min(4, len(part.lines))] {
				contextLine(line)
			}
			gap(len(part.lines) - 4)
		case trailing:
			gap(len(part.lines) - 4)
			for _, line := range part.lines[max(0, len(part.lines)-4):] {
				contextLine(line)
			}
		default:
			oldLine += len(part.lines)
			newLine += len(part.lines)
		}
	}
	details.Diff = strings.Join(output, "\n")
	return details
}

// Unified patch construction follows jsdiff's four-line context and EOF markers.
func editUnifiedPatch(path string, parts []editDiffPart) string {
	var output strings.Builder
	fmt.Fprintf(&output, "--- %s\n+++ %s\n", path, path)
	oldStart, newStart, oldLine, newLine := 0, 0, 1, 1
	var lines []string
	addContext := func(context []string) {
		for _, line := range context {
			lines = append(lines, " "+line)
		}
	}
	for i := 0; i <= len(parts); i++ {
		var part editDiffPart
		if i < len(parts) {
			part = parts[i]
		}
		if part.kind != 0 {
			if oldStart == 0 {
				oldStart, newStart = oldLine, newLine
				if i > 0 {
					previous := parts[i-1].lines
					addContext(previous[max(0, len(previous)-4):])
					oldStart -= len(lines)
					newStart -= len(lines)
				}
			}
			for _, line := range part.lines {
				lines = append(lines, string(part.kind)+line)
			}
			if part.kind == '+' {
				newLine += len(part.lines)
			} else {
				oldLine += len(part.lines)
			}
		} else {
			if oldStart != 0 {
				if len(part.lines) <= 8 && i < len(parts)-1 {
					addContext(part.lines)
				} else {
					size := min(len(part.lines), 4)
					addContext(part.lines[:size])
					oldCount, newCount := oldLine-oldStart+size, newLine-newStart+size
					if oldCount == 0 {
						oldStart--
					}
					if newCount == 0 {
						newStart--
					}
					fmt.Fprintf(&output, "@@ -%d,%d +%d,%d @@\n", oldStart, oldCount, newStart, newCount)
					for _, line := range lines {
						output.WriteString(line)
						if !strings.HasSuffix(line, "\n") {
							output.WriteString("\n\\ No newline at end of file\n")
						}
					}
					oldStart, newStart = 0, 0
					lines = nil
				}
			}
			oldLine += len(part.lines)
			newLine += len(part.lines)
		}
	}
	return output.String()
}
