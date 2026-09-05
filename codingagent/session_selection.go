package codingagent

import (
	"context"
	"fmt"
	"os"
	"strings"
)

func resolveSessionSelection(ctx context.Context, value, cwd string, dir *string) (path string, global bool, err error) {
	if strings.ContainsAny(value, `/\`) || strings.HasSuffix(value, ".jsonl") {
		path, err = resolveSessionPath(value)
		return
	}
	local, err := ListSessions(ctx, cwd, SessionListOptions{SessionDir: dir})
	if err != nil {
		return "", false, err
	}
	if path := matchSessionID(local, value); path != "" {
		return path, false, nil
	}
	all, err := ListAllSessions(ctx, SessionListOptions{SessionDir: dir})
	if err != nil {
		return "", false, err
	}
	if path := matchSessionID(all, value); path != "" {
		return path, true, nil
	}
	return "", false, fmt.Errorf("No session found matching '%s'", value)
}
func matchSessionID(sessions []SessionInfo, id string) string {
	for _, s := range sessions {
		if s.ID == id {
			return s.Path
		}
	}
	for _, s := range sessions {
		if strings.HasPrefix(s.ID, id) {
			return s.Path
		}
	}
	return ""
}

func selectHeadlessSession(ctx context.Context, parsed Args, cwd string, dir *string, option NewSessionOptions) (*SessionManager, error) {
	switch {
	case parsed.NoSession:
		return NewInMemorySessionManager(cwd, option), nil
	case parsed.Fork != nil:
		if option.ID != "" {
			sessions, err := ListSessions(ctx, cwd, SessionListOptions{SessionDir: dir})
			if err != nil {
				return nil, err
			}
			for _, s := range sessions {
				if s.ID == option.ID {
					return nil, fmt.Errorf("Session already exists with id '%s'", option.ID)
				}
			}
		}
		path, _, err := resolveSessionSelection(ctx, *parsed.Fork, cwd, dir)
		if err != nil {
			return nil, err
		}
		return ForkSessionManager(path, cwd, dir, option)
	case parsed.Session != nil:
		path, global, err := resolveSessionSelection(ctx, *parsed.Session, cwd, dir)
		if err != nil {
			return nil, err
		}
		if global {
			info := readSessionInfo(path)
			if info == nil {
				return nil, fmt.Errorf("Cannot read session %s", path)
			}
			fmt.Fprintf(os.Stdout, "Session found in different project: %s\nFork this session into current directory? [y/N] ", info.CWD)
			var answer strings.Builder
			buf := make([]byte, 1)
			for {
				n, err := os.Stdin.Read(buf)
				if n > 0 {
					if buf[0] == '\n' {
						break
					}
					answer.WriteByte(buf[0])
				}
				if err != nil {
					break
				}
			}
			value := strings.ToLower(strings.TrimSuffix(answer.String(), "\r"))
			if value != "y" && value != "yes" {
				fmt.Fprintln(os.Stdout, "Aborted.")
				return nil, nil
			}
			return ForkSessionManager(path, cwd, dir)
		}
		return OpenSessionManager(path, dir, nil)
	case parsed.Continue:
		return ContinueRecentSessionManager(cwd, dir)
	default:
		return NewSessionManager(cwd, dir, option)
	}
}
