package ai

// SplitDeferredTools partitions current definitions by transcript usage. A nil
// normalizer matches names exactly; normalization never rewrites definitions.
// Both groups retain first occurrence order, with the last duplicate winning.
// Only added tools never called in the transcript are deferred (ADR-0016).
func SplitDeferredTools(input Context, enabled bool, normalizeName func(string) string) (immediate, deferred []Tool) {
	if normalizeName == nil {
		normalizeName = func(name string) string { return name }
	}
	names := make([]string, 0, len(input.Tools))
	tools := make(map[string]Tool, len(input.Tools))
	for _, tool := range input.Tools {
		name := normalizeName(tool.Name)
		if _, exists := tools[name]; !exists {
			names = append(names, name)
		}
		tools[name] = tool
	}
	used, added := map[string]bool{}, map[string]bool{}
	if enabled {
		for _, message := range input.Messages {
			var content []AssistantContent
			var addedNames Optional[[]string]
			switch message := message.(type) {
			case AssistantMessage:
				content = message.Content
			case *AssistantMessage:
				if message != nil {
					content = message.Content
				}
			case ToolResultMessage:
				addedNames = message.AddedToolNames
			case *ToolResultMessage:
				if message != nil {
					addedNames = message.AddedToolNames
				}
			}
			for _, block := range content {
				switch call := block.(type) {
				case ToolCall:
					used[normalizeName(call.Name)] = true
				case *ToolCall:
					if call != nil {
						used[normalizeName(call.Name)] = true
					}
				}
			}
			if values, ok := addedNames.Value(); ok {
				for _, value := range values {
					added[normalizeName(value)] = true
				}
			}
		}
	}
	immediate, deferred = []Tool{}, []Tool{}
	for _, name := range names {
		if added[name] && !used[name] {
			deferred = append(deferred, tools[name])
		} else {
			immediate = append(immediate, tools[name])
		}
	}
	return immediate, deferred
}
