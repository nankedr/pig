package codingagent

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func ContinueRecentSessionManager(cwd string, sessionDir *string) (*SessionManager, error) {
	cwd, err := resolveSessionPath(cwd)
	if err != nil {
		return nil, err
	}
	dir, _, err := resolveSessionDir(cwd, sessionDir)
	if err != nil {
		return nil, err
	}
	defaultDir, _, err := resolveSessionDir(cwd, nil)
	if err != nil {
		return nil, err
	}
	var recent string
	var modified time.Time
	for _, path := range sessionFiles(dir) {
		header := discoveryHeader(path)
		if header == nil {
			continue
		}
		if sessionDir != nil && dir != defaultDir && !sessionCWDMatches(header.CWD, cwd) {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			return NewSessionManager(cwd, sessionDir)
		}
		if recent == "" || info.ModTime().After(modified) {
			recent, modified = path, info.ModTime()
		}
	}
	if recent != "" {
		return OpenSessionManager(recent, &dir, &cwd)
	}
	return NewSessionManager(cwd, sessionDir)
}

func ListSessions(ctx context.Context, cwd string, options ...SessionListOptions) ([]SessionInfo, error) {
	option := SessionListOptions{}
	if len(options) > 0 {
		option = options[0]
	}
	cwd, err := resolveSessionPath(cwd)
	if err != nil {
		return nil, err
	}
	dir, _, err := resolveSessionDir(cwd, option.SessionDir)
	if err != nil {
		return nil, err
	}
	sessions, err := listSessionFiles(ctx, sessionFiles(dir), option.OnProgress)
	if err != nil {
		return nil, err
	}
	defaultDir, _, err := resolveSessionDir(cwd, nil)
	if err != nil {
		return nil, err
	}
	if option.SessionDir != nil && dir != defaultDir {
		filtered := sessions[:0]
		for _, s := range sessions {
			if sessionCWDMatches(s.CWD, cwd) {
				filtered = append(filtered, s)
			}
		}
		sessions = filtered
	}
	return sessions, nil
}

func ListAllSessions(ctx context.Context, options ...SessionListOptions) ([]SessionInfo, error) {
	option := SessionListOptions{}
	if len(options) > 0 {
		option = options[0]
	}
	files := []string{}
	if option.SessionDir != nil {
		dir, err := resolveSessionPath(*option.SessionDir)
		if err != nil {
			return nil, err
		}
		files = sessionFiles(dir)
	} else {
		dir, err := GetAgentDir()
		if err != nil {
			return nil, err
		}
		dir = filepath.Join(dir, "sessions")
		entries, _ := os.ReadDir(dir)
		for _, entry := range entries {
			if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
				files = append(files, sessionFiles(filepath.Join(dir, entry.Name()))...)
			}
		}
	}
	return listSessionFiles(ctx, files, option.OnProgress)
}

func sessionFiles(dir string) []string {
	entries, _ := os.ReadDir(dir)
	files := []string{}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".jsonl") {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	return files
}

func sessionCWDMatches(cwd, target string) bool {
	if cwd == "" {
		return false
	}
	resolved, err := resolveSessionPath(cwd)
	return err == nil && resolved == target
}

func discoveryHeader(path string) *SessionHeader {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	reader := bufio.NewReader(io.LimitReader(file, 1024*1024+1))
	scanned := 0
	for {
		line, err := reader.ReadString('\n')
		scanned += len(line)
		if scanned > 1024*1024 {
			return nil
		}
		entries := ParseSessionEntries(line)
		if len(entries) > 0 {
			return entries[0].Header
		}
		if err != nil {
			return nil
		}
	}
}

func listSessionFiles(ctx context.Context, files []string, progress SessionListProgress) ([]SessionInfo, error) {
	sessions := []SessionInfo{}
	for i, path := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if info := readSessionInfo(path); info != nil {
			sessions = append(sessions, *info)
		}
		if progress != nil {
			progress(i+1, len(files))
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(sessions, func(i, j int) bool { return sessions[i].Modified.After(sessions[j].Modified) })
	return sessions, nil
}

func readSessionInfo(path string) *SessionInfo {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		return nil
	}
	reader := bufio.NewReader(file)
	var info *SessionInfo
	var activity int64
	hasActivity := false
	texts := []string{}
	for {
		line, err := reader.ReadString('\n')
		for _, record := range ParseSessionEntries(line) {
			if info == nil {
				if record.Header == nil {
					return nil
				}
				h := record.Header
				created, _ := time.Parse(time.RFC3339Nano, h.Timestamp)
				info = &SessionInfo{Path: path, ID: h.ID, CWD: h.CWD, Created: created, Modified: created, ParentSessionPath: cloneStringPointer(h.ParentSession)}
				if created.IsZero() {
					info.Modified = stat.ModTime()
				}
				continue
			}
			entry := record.Entry
			if entry == nil {
				continue
			}
			if entry.Type == "session_info" {
				info.Name = nil
				if entry.Name != nil {
					if name := strings.TrimSpace(*entry.Name); name != "" {
						info.Name = &name
					}
				}
			}
			if entry.Type != "message" {
				continue
			}
			info.MessageCount++
			var raw struct {
				Message struct {
					Role      string
					Content   json.RawMessage
					Timestamp *int64
				}
			}
			if json.Unmarshal(record.Raw, &raw) != nil || (raw.Message.Role != "user" && raw.Message.Role != "assistant") {
				continue
			}
			stamp := sessionTimestampMillis(entry.Timestamp)
			if raw.Message.Timestamp != nil {
				stamp = *raw.Message.Timestamp
			}
			if !hasActivity || stamp > activity {
				activity = stamp
				hasActivity = true
			}
			text := sessionContentText(raw.Message.Content, " ")
			if text == "" {
				continue
			}
			texts = append(texts, text)
			if info.FirstMessage == "" && raw.Message.Role == "user" {
				info.FirstMessage = text
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil
		}
	}
	if info != nil {
		if hasActivity && activity > 0 {
			info.Modified = time.UnixMilli(activity)
		}
		info.AllMessagesText = strings.Join(texts, " ")
		if info.FirstMessage == "" {
			info.FirstMessage = "(no messages)"
		}
	}
	return info
}

func sessionContentText(raw json.RawMessage, separator string) string {
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text
	}
	var blocks []struct{ Type, Text string }
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	texts := []string{}
	for _, b := range blocks {
		if b.Type == "text" {
			texts = append(texts, b.Text)
		}
	}
	return strings.Join(texts, separator)
}
