package codingagent

import (
	"bytes"
	"context"
	"crypto/sha256"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"html"
	"math"
	"os"
	"path/filepath"
	"strings"
)

//go:embed exporthtml/* exporthtml/vendor/*
var exportHTMLAssets embed.FS

type htmlSessionData struct {
	Header       *SessionHeader    `json:"header"`
	Entries      []json.RawMessage `json:"entries"`
	LeafID       *string           `json:"leafId"`
	SystemPrompt string            `json:"systemPrompt,omitempty"`
	Tools        []map[string]any  `json:"tools,omitempty"`
}

// ExportFromFile exports a v3 session without opening or modifying Pig or Pi state.
func ExportFromFile(ctx context.Context, input string, output ...string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	path, err := resolveSessionPath(input)
	if err != nil {
		return "", err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read export input: %w", err)
	}
	data := htmlSessionData{Entries: []json.RawMessage{}}
	for i, line := range bytes.Split(raw, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		if !json.Valid(line) {
			return "", fmt.Errorf("invalid session JSON on line %d", i+1)
		}
		if data.Header == nil {
			if err = json.Unmarshal(line, &data.Header); err != nil {
				return "", fmt.Errorf("invalid session header: %w", err)
			}
			if data.Header == nil {
				return "", fmt.Errorf("invalid session header: null")
			}
		} else {
			data.Entries = append(data.Entries, bytes.Clone(line))
		}
	}
	if len(data.Entries) > 0 {
		var e SessionEntry
		e, err = decodeSessionEntry(data.Entries[len(data.Entries)-1])
		if err != nil {
			return "", err
		}
		data.LeafID = &e.ID
	}
	return writeSessionHTML(ctx, path, data, output)
}

func writeSessionHTML(ctx context.Context, source string, data htmlSessionData, output []string) (string, error) {
	if err := validateHTMLSession(data); err != nil {
		return "", err
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	asset := func(name string) string { b, _ := exportHTMLAssets.ReadFile("exporthtml/" + name); return string(b) }
	js, marked, highlight := asset("template.js"), asset("vendor/marked.min.js"), asset("vendor/highlight.min.js")
	scriptPolicy := ""
	for _, script := range []string{marked, highlight, "\n" + js + "\n  "} {
		h := sha256.Sum256([]byte(script))
		scriptPolicy += " 'sha256-" + base64.StdEncoding.EncodeToString(h[:]) + "'"
	}
	csp := "default-src 'none'; script-src" + scriptPolicy + "; style-src 'unsafe-inline'; img-src 'none'; connect-src 'none'; base-uri 'none'; form-action 'none'"
	document := strings.NewReplacer("{{CSS}}", asset("template.css"), "{{JS}}", js, "{{SESSION_DATA}}", base64.StdEncoding.EncodeToString(encoded), "{{MARKED_JS}}", marked, "{{HIGHLIGHT_JS}}", highlight, "{{CSP}}", csp, "{{LICENSES}}", html.EscapeString(asset("LICENSES.txt"))).Replace(asset("template.html"))
	path := "pig-session-" + strings.TrimSuffix(filepath.Base(source), ".jsonl") + ".html"
	if len(output) > 0 && output[0] != "" {
		path = output[0]
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, e := os.UserHomeDir()
		if e != nil {
			return "", e
		}
		if path == "~" {
			path = home
		} else {
			path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	target, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	sourceInfo, err := os.Stat(source)
	if err != nil {
		return "", err
	}
	if target == source {
		return "", fmt.Errorf("export output must differ from session input")
	}
	if info, e := os.Stat(target); e == nil && os.SameFile(info, sourceInfo) {
		return "", fmt.Errorf("export output must differ from session input")
	}
	if err = ctx.Err(); err != nil {
		return "", err
	}
	if err = os.WriteFile(path, []byte(document), 0600); err != nil {
		return "", fmt.Errorf("write HTML export: %w", err)
	}
	return path, nil
}

func validateHTMLSession(data htmlSessionData) error {
	h := data.Header
	if h == nil || h.Type != "session" || h.Version == nil || *h.Version != 3 || h.ID == "" {
		return fmt.Errorf("HTML export requires a valid v3 session header")
	}
	ids := map[string]bool{}
	for i, raw := range data.Entries {
		var e map[string]any
		if err := json.Unmarshal(raw, &e); err != nil || e == nil {
			return fmt.Errorf("invalid export entry %d", i+1)
		}
		id, ok := e["id"].(string)
		if !ok || id == "" || ids[id] {
			return fmt.Errorf("invalid or duplicate export entry id at %d", i+1)
		}
		if parent := e["parentId"]; parent != nil {
			p, ok := parent.(string)
			if !ok || !ids[p] {
				return fmt.Errorf("invalid export parent for %q", id)
			}
		}
		ids[id] = true
		if _, ok := e["timestamp"].(string); !ok {
			return fmt.Errorf("invalid export timestamp for %q", id)
		}
		typ, _ := e["type"].(string)
		switch typ {
		case "message":
			m, ok := e["message"].(map[string]any)
			if !ok {
				return fmt.Errorf("invalid export message %q", id)
			}
			role, _ := m["role"].(string)
			switch role {
			case "user", "assistant", "toolResult":
				if err := validateHTMLContent(m["content"], role == "user"); err != nil {
					return fmt.Errorf("entry %q: %w", id, err)
				}
			case "bashExecution":
				if _, ok := m["command"].(string); !ok {
					return fmt.Errorf("invalid bash command in %q", id)
				}
				for _, field := range []string{"output", "exitCode", "cancelled", "truncated"} {
					value, exists := m[field]
					if !exists || (field == "exitCode" && value == nil) {
						continue
					}
					valid := false
					switch field {
					case "output":
						_, valid = value.(string)
					case "exitCode":
						number, ok := value.(float64)
						valid = ok && number == math.Trunc(number)
					default:
						_, valid = value.(bool)
					}
					if !valid {
						return fmt.Errorf("invalid bash %s in %q", field, id)
					}
				}
			default:
				return notImplemented("export.message." + role)
			}
		case "compaction", "branch_summary":
			if _, ok := e["summary"].(string); !ok {
				return fmt.Errorf("invalid summary in %q", id)
			}
			if typ == "compaction" {
				if _, ok := e["tokensBefore"].(float64); !ok {
					return fmt.Errorf("invalid tokensBefore in %q", id)
				}
			}
		case "custom_message":
			if err := validateHTMLContent(e["content"], true); err != nil {
				return err
			}
		case "model_change", "thinking_level_change", "custom", "label", "session_info":
		default:
			return notImplemented("export.entry." + typ)
		}
		if err := validateHTMLFieldTypes(e); err != nil {
			return fmt.Errorf("entry %q: %w", id, err)
		}
	}
	if data.LeafID != nil && !ids[*data.LeafID] {
		return fmt.Errorf("invalid export leaf")
	}
	return nil
}

func validateHTMLContent(content any, allowText bool) error {
	if _, ok := content.(string); ok && allowText {
		return nil
	}
	blocks, ok := content.([]any)
	if !ok {
		return fmt.Errorf("invalid export message content")
	}
	for _, value := range blocks {
		block, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("invalid export content block")
		}
		typ, _ := block["type"].(string)
		switch typ {
		case "text", "thinking":
			if _, ok := block[typ].(string); !ok {
				return fmt.Errorf("invalid %s block", typ)
			}
		case "toolCall":
			if _, ok := block["id"].(string); !ok {
				return fmt.Errorf("invalid tool call id")
			}
			if _, ok := block["name"].(string); !ok {
				return fmt.Errorf("invalid tool call name")
			}
			if _, ok := block["arguments"].(map[string]any); !ok {
				return fmt.Errorf("invalid tool call arguments")
			}
		default:
			return notImplemented("export.content." + typ)
		}
	}
	return nil
}

// Validate fields used by the browser; tool arguments and opaque details retain their own schemas.
func validateHTMLFieldTypes(value map[string]any) error {
	for k, v := range value {
		if v == nil {
			continue
		}
		switch k {
		case "errorMessage", "stopReason", "toolName", "provider", "model", "modelId", "thinkingLevel", "label", "name", "diff":
			if _, ok := v.(string); !ok {
				return fmt.Errorf("invalid export field %s", k)
			}
		case "usage", "cost":
			m, ok := v.(map[string]any)
			if !ok {
				return fmt.Errorf("invalid export field %s", k)
			}
			for key, number := range m {
				if key == "cost" {
					continue
				}
				if _, ok := number.(float64); !ok {
					return fmt.Errorf("invalid export usage %s", key)
				}
			}
			if err := validateHTMLFieldTypes(m); err != nil {
				return err
			}
		case "details":
			if m, ok := v.(map[string]any); ok {
				if diff, ok := m["diff"]; ok {
					if _, ok := diff.(string); !ok {
						return fmt.Errorf("invalid export diff")
					}
				}
			}
		case "message":
			m, ok := v.(map[string]any)
			if !ok {
				return fmt.Errorf("invalid export field %s", k)
			}
			if err := validateHTMLFieldTypes(m); err != nil {
				return err
			}
		case "input", "cacheRead", "cacheWrite", "totalTokens", "total":
			if _, ok := v.(float64); !ok {
				return fmt.Errorf("invalid export field %s", k)
			}
		}
	}
	return nil
}

func (s *AgentSession) ExportToHTML(ctx context.Context, output ...string) (string, error) {
	if s == nil || s.sessionManager == nil {
		return "", fmt.Errorf("Cannot export in-memory session to HTML")
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	m := s.sessionManager
	m.mu.RLock()
	source := m.sessionFile
	header := cloneSessionHeader(m.header)
	data := htmlSessionData{Header: &header, Entries: make([]json.RawMessage, 0, len(m.entries)), LeafID: cloneStringPointer(m.leafID)}
	for _, entry := range m.entries {
		raw, err := marshalSessionEntry(entry)
		if err != nil {
			m.mu.RUnlock()
			return "", err
		}
		data.Entries = append(data.Entries, raw)
	}
	m.mu.RUnlock()
	if source == "" {
		return "", fmt.Errorf("Cannot export in-memory session to HTML")
	}
	if _, err := os.Stat(source); err != nil {
		return "", fmt.Errorf("Nothing to export yet - start a conversation first: %w", err)
	}
	state := s.State()
	data.SystemPrompt = state.SystemPrompt
	for _, tool := range state.Tools {
		data.Tools = append(data.Tools, map[string]any{"name": tool.Tool.Name, "description": tool.Tool.Description, "parameters": tool.Tool.Parameters})
	}
	return writeSessionHTML(ctx, source, data, output)
}
