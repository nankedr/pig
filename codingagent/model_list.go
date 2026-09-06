package codingagent

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

func listRuntimeModels(ctx context.Context, parsed Args) (string, error) {
	runtime, err := NewModelRuntime(ctx, CreateModelRuntimeOptions{Offline: ResolveOffline(parsed.Offline)})
	if err != nil {
		return "", err
	}
	models, err := runtime.GetAvailable(ctx)
	if err != nil {
		return "", err
	}
	if len(models) == 0 {
		return "No models available. Configure credentials or use --api-key with --provider and --model.\n", nil
	}
	sort.Slice(models, func(i, j int) bool {
		if models[i].Provider != models[j].Provider {
			return models[i].Provider < models[j].Provider
		}
		return models[i].ID < models[j].ID
	})
	search, _ := parsed.ListModels.Search()
	rows := [][6]string{{"provider", "model", "context", "max-out", "thinking", "images"}}
	count := func(n int64) string {
		if n >= 1000000 {
			return fmt.Sprintf("%gM", float64(n)/1000000)
		}
		if n >= 1000 {
			return fmt.Sprintf("%gK", float64(n)/1000)
		}
		return fmt.Sprint(n)
	}
	for _, m := range models {
		candidate := strings.ToLower(string(m.Provider) + " " + m.ID)
		match := true
		for _, part := range strings.Fields(strings.ToLower(search)) {
			rest := candidate
			for _, r := range part {
				index := strings.IndexRune(rest, r)
				if index < 0 {
					match = false
					break
				}
				rest = rest[index+len(string(r)):]
			}
		}
		if !match {
			continue
		}
		thinking, images := "no", "no"
		if m.Reasoning {
			thinking = "yes"
		}
		for _, input := range m.Input {
			if input == "image" {
				images = "yes"
			}
		}
		rows = append(rows, [6]string{string(m.Provider), m.ID, count(m.ContextWindow), count(m.MaxTokens), thinking, images})
	}
	if len(rows) == 1 {
		return fmt.Sprintf("No models matching %q\n", search), nil
	}
	var widths [6]int
	for _, row := range rows {
		for i, v := range row {
			widths[i] = max(widths[i], len(v))
		}
	}
	var out strings.Builder
	for _, row := range rows {
		for i, v := range row {
			if i > 0 {
				out.WriteString("  ")
			}
			fmt.Fprintf(&out, "%-*s", widths[i], v)
		}
		out.WriteByte('\n')
	}
	return out.String(), nil
}
