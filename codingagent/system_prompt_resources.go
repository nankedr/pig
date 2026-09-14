package codingagent

import (
	"fmt"
	"os"
	"path/filepath"
)

func (l *DefaultResourceLoader) discoverPrompt(name string, trusted bool) *string {
	paths := []string{}
	if trusted {
		paths = append(paths, filepath.Join(l.cwd, ".pig", name))
	}
	paths = append(paths, filepath.Join(l.agentDir, name))
	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return &path
		}
	}
	return nil
}

func resolveResourcePrompt(input *string, description string, diagnostics *[]ResourceDiagnostic) (*string, *ResourcePathSource) {
	if input == nil || *input == "" {
		return nil, nil
	}
	value := *input
	info, err := os.Stat(value)
	if err != nil {
		return &value, nil
	}
	path, err := filepath.Abs(value)
	if err != nil {
		return &value, nil
	}
	source := &ResourcePathSource{Path: path}
	var data []byte
	if !info.Mode().IsRegular() {
		err = fmt.Errorf("not a regular file")
	} else {
		data, err = os.ReadFile(value)
	}
	if err != nil {
		*diagnostics = append(*diagnostics, ResourceDiagnostic{Type: "warning", Path: path, Message: fmt.Sprintf("Could not read %s file %s: %v", description, value, err)})
		return &value, source
	}
	value = string(data)
	return &value, source
}
