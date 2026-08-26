package codingagent

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

type SourceInfo struct {
	BaseDir string
	Origin  ResourceOrigin
	Path    string
	Scope   SourceScope
	Source  string
}

func CreateSyntheticSourceInfo(path string, options ...PathMetadata) SourceInfo {
	metadata := PathMetadata{Scope: SourceScopeTemporary, Origin: ResourceOriginTopLevel}
	if len(options) != 0 {
		metadata = options[0]
		if metadata.Scope == "" {
			metadata.Scope = SourceScopeTemporary
		}
		if metadata.Origin == "" {
			metadata.Origin = ResourceOriginTopLevel
		}
	}
	return SourceInfo{Path: path, Source: metadata.Source, Scope: metadata.Scope, Origin: metadata.Origin, BaseDir: metadata.BaseDir}
}

type SkillFrontmatter struct {
	Description            string
	DisableModelInvocation bool
	Name                   string
	Additional             map[string]any
}

type Skill struct {
	BaseDir                string
	Description            string
	DisableModelInvocation bool
	FilePath               string
	Name                   string
	SourceInfo             SourceInfo
}

type LoadSkillsFromDirOptions struct {
	Dir    string
	Source string
}

type LoadSkillsResult struct {
	Diagnostics []ResourceDiagnostic
	Skills      []Skill
}

func LoadSkillsFromDir(context.Context, LoadSkillsFromDirOptions) (LoadSkillsResult, error) {
	return LoadSkillsResult{}, notImplemented("LoadSkillsFromDir")
}

func LoadSkills(context.Context, string, string, []string, bool) (LoadSkillsResult, error) {
	return LoadSkillsResult{}, notImplemented("LoadSkills")
}

func FormatSkillsForPrompt(skills []Skill) string {
	lines := []string{
		"\n\nThe following skills provide specialized instructions for specific tasks.",
		"Use the read tool to load a skill's file when the task matches its description.",
		"When a skill file references a relative path, resolve it against the skill directory (parent of SKILL.md / dirname of the path) and use that absolute path in tool commands.",
		"",
		"<available_skills>",
	}
	visible := 0
	for _, skill := range skills {
		if skill.DisableModelInvocation {
			continue
		}
		visible++
		lines = append(lines,
			"  <skill>",
			"    <name>"+escapeXML(skill.Name)+"</name>",
			"    <description>"+escapeXML(skill.Description)+"</description>",
			"    <location>"+escapeXML(skill.FilePath)+"</location>",
			"  </skill>",
		)
	}
	if visible == 0 {
		return ""
	}
	return strings.Join(append(lines, "</available_skills>"), "\n")
}

func escapeXML(value string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;").Replace(value)
}

type PromptTemplate struct {
	ArgumentHint string
	Content      string
	Description  string
	FilePath     string
	Name         string
	SourceInfo   SourceInfo
}

func parseCommandArgs(value string) []string {
	var arguments []string
	var current strings.Builder
	var quote rune
	flush := func() {
		if current.Len() != 0 {
			arguments = append(arguments, current.String())
			current.Reset()
		}
	}
	for _, character := range value {
		if quote != 0 {
			if character == quote {
				quote = 0
			} else {
				current.WriteRune(character)
			}
			continue
		}
		switch {
		case character == '\'' || character == '"':
			quote = character
		case unicode.IsSpace(character):
			flush()
		default:
			current.WriteRune(character)
		}
	}
	flush()
	return arguments
}

var argumentPlaceholder = regexp.MustCompile(`\$\{(\d+|ARGUMENTS|@):-([^}]*)\}|\$\{@:(\d+)(?::(\d+))?\}|\$(ARGUMENTS|@|\d+)`)

func substituteArgs(content string, args []string) string {
	allArgs := strings.Join(args, " ")
	return argumentPlaceholder.ReplaceAllStringFunc(content, func(match string) string {
		parts := argumentPlaceholder.FindStringSubmatch(match)
		if parts[1] != "" {
			value := allArgs
			if parts[1] != "@" && parts[1] != "ARGUMENTS" {
				position, _ := strconv.Atoi(parts[1])
				value = positionalArgument(args, position)
			}
			if value == "" {
				return parts[2]
			}
			return value
		}
		if parts[3] != "" {
			start, _ := strconv.Atoi(parts[3])
			start = max(0, start-1)
			if start >= len(args) {
				return ""
			}
			end := len(args)
			if parts[4] != "" {
				length, _ := strconv.Atoi(parts[4])
				end = min(end, start+max(0, length))
			}
			return strings.Join(args[start:end], " ")
		}
		if parts[5] == "@" || parts[5] == "ARGUMENTS" {
			return allArgs
		}
		position, _ := strconv.Atoi(parts[5])
		return positionalArgument(args, position)
	})
}

func positionalArgument(args []string, position int) string {
	if position <= 0 || position > len(args) {
		return ""
	}
	return args[position-1]
}

func expandPromptTemplate(text string, templates []PromptTemplate) string {
	if !strings.HasPrefix(text, "/") {
		return text
	}
	rest := text[1:]
	nameEnd := strings.IndexFunc(rest, unicode.IsSpace)
	name, argsText := rest, ""
	if nameEnd >= 0 {
		name = rest[:nameEnd]
		argsText = strings.TrimSpace(rest[nameEnd:])
	}
	if name == "" {
		return text
	}
	for _, template := range templates {
		if template.Name == name {
			return substituteArgs(template.Content, parseCommandArgs(argsText))
		}
	}
	return text
}

type SlashCommandSource string

const (
	SlashCommandSourceExtension SlashCommandSource = "extension"
	SlashCommandSourcePrompt    SlashCommandSource = "prompt"
	SlashCommandSourceSkill     SlashCommandSource = "skill"
)

type SlashCommandInfo struct {
	Description string
	Name        string
	Source      SlashCommandSource
	SourceInfo  SourceInfo
}

type BuildSystemPromptOptions struct {
	AppendSystemPrompt string
	ContextFiles       []AgentsFile
	CustomPrompt       string
	CWD                string
	PromptGuidelines   []string
	SelectedTools      []string
	Skills             []Skill
	ToolSnippets       map[string]string
}

func buildSystemPrompt(options BuildSystemPromptOptions) string {
	cwd := strings.ReplaceAll(options.CWD, `\`, "/")
	appendSection := ""
	if options.AppendSystemPrompt != "" {
		appendSection = "\n\n" + options.AppendSystemPrompt
	}
	if options.CustomPrompt != "" {
		prompt := options.CustomPrompt + appendSection
		prompt += formatProjectContext(options.ContextFiles)
		if containsTool(options.SelectedTools, "read", true) && len(options.Skills) != 0 {
			prompt += FormatSkillsForPrompt(options.Skills)
		}
		return prompt + "\nCurrent working directory: " + cwd
	}

	tools := options.SelectedTools
	if tools == nil {
		tools = []string{"read", "bash", "edit", "write"}
	}
	var available []string
	for _, name := range tools {
		if snippet := options.ToolSnippets[name]; snippet != "" {
			available = append(available, "- "+name+": "+snippet)
		}
	}
	toolsList := "(none)"
	if len(available) != 0 {
		toolsList = strings.Join(available, "\n")
	}
	guidelines := make([]string, 0, len(options.PromptGuidelines)+3)
	seen := map[string]struct{}{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		guidelines = append(guidelines, value)
	}
	if containsTool(tools, "bash", false) && !containsTool(tools, "grep", false) && !containsTool(tools, "find", false) && !containsTool(tools, "ls", false) {
		add("Use bash for file operations like ls, rg, find")
	}
	for _, guideline := range options.PromptGuidelines {
		add(guideline)
	}
	add("Be concise in your responses")
	add("Show file paths clearly when working with files")
	for index := range guidelines {
		guidelines[index] = "- " + guidelines[index]
	}
	prompt := "You are an expert coding assistant operating inside pig, a coding agent harness. You help users by reading files, executing commands, editing code, and writing new files.\n\nAvailable tools:\n" + toolsList + "\n\nIn addition to the tools above, you may have access to other custom tools depending on the project.\n\nGuidelines:\n" + strings.Join(guidelines, "\n")
	prompt += appendSection
	prompt += formatProjectContext(options.ContextFiles)
	if containsTool(tools, "read", false) && len(options.Skills) != 0 {
		prompt += FormatSkillsForPrompt(options.Skills)
	}
	return prompt + "\nCurrent working directory: " + cwd
}

func containsTool(tools []string, name string, nilMeansDefault bool) bool {
	if tools == nil && nilMeansDefault {
		return true
	}
	for _, tool := range tools {
		if tool == name {
			return true
		}
	}
	return false
}

func formatProjectContext(files []AgentsFile) string {
	if len(files) == 0 {
		return ""
	}
	var prompt strings.Builder
	prompt.WriteString("\n\n<project_context>\n\nProject-specific instructions and guidelines:\n\n")
	for _, file := range files {
		prompt.WriteString(`<project_instructions path="`)
		prompt.WriteString(file.Path)
		prompt.WriteString(`">` + "\n")
		prompt.WriteString(file.Content)
		prompt.WriteString("\n</project_instructions>\n\n")
	}
	prompt.WriteString("</project_context>\n")
	return prompt.String()
}

type ParsedFrontmatter struct {
	Body        string
	Frontmatter map[string]any
}

// ParseFrontmatter is an explicit capability stub until codingagent has a YAML
// implementation compatible with the pinned upstream parser.
func ParseFrontmatter(content string) (ParsedFrontmatter, error) {
	return ParsedFrontmatter{}, notImplemented("ParseFrontmatter")
}

func StripFrontmatter(content string) (string, error) {
	parsed, err := ParseFrontmatter(content)
	if err != nil {
		return "", err
	}
	return parsed.Body, nil
}
