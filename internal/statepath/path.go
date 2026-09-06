package statepath

import (
	"os"
	"path/filepath"
	"strings"
)

func Resolve(path string) (string, error) {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~"))
	}
	return filepath.Abs(path)
}

func AgentDir() (string, error) {
	if path := strings.TrimSpace(os.Getenv("PIG_CODING_AGENT_DIR")); path != "" {
		return Resolve(path)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".pig", "agent"), nil
}

func AuthPath(path string) (string, error) {
	if path != "" {
		return Resolve(path)
	}
	dir, err := AgentDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "auth.json"), nil
}
