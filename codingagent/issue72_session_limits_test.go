package codingagent_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
)

func TestOpenHistoricalSessionHasNoDiscoveryOrLineLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large-lines.jsonl")
	content := "{broken" + strings.Repeat("x", 2<<20) + "\n" + `{"type":"session","version":2,"id":"large","cwd":"/project","timestamp":"2025-01-01T00:00:00.000Z","padding":"` + strings.Repeat("x", 2<<20) + "\"}\n" + `{"type":"message","id":"user","parentId":null,"timestamp":"2025-01-01T00:00:00.000Z","message":{"role":"user","content":"` + strings.Repeat("中", 1<<20) + `","timestamp":1}}`
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	manager, err := codingagent.OpenSessionManager(path, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if manager.GetCWD() != "/project" || len(manager.BuildSessionContext().Messages) != 1 {
		t.Fatal("large session lost cwd/history")
	}
	if _, err := manager.AppendMessage(ai.UserMessage{Role: ai.MessageRoleUser, Content: ai.UserText("next"), Timestamp: 2}); err != nil {
		t.Fatal(err)
	}
	reopened, err := codingagent.OpenSessionManager(path, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	messages := reopened.BuildSessionContext().Messages
	text, _ := messages[0].(ai.UserMessage).Content.Text()
	if len(messages) != 2 || text != strings.Repeat("中", 1<<20) {
		t.Fatal("long line or unterminated historical input was truncated")
	}
}

func TestOpenSessionLargerThanNodeStringLimitWithManyEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large-file.jsonl")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.WriteString("{\"type\":\"session\",\"version\":3,\"id\":\"large\",\"cwd\":\"/project\"}\n"); err != nil {
		t.Fatal(err)
	}
	// Sparse malformed lines reproduce the fixed Pi file-operations regression.
	for offset := int64(16 << 20); offset <= int64(528<<20); offset += 16 << 20 {
		if _, err := file.WriteAt([]byte{'\n'}, offset); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := file.Seek(0, 2); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10001; i++ {
		parent := "null"
		if i > 0 {
			parent = fmt.Sprintf("\"%d\"", i-1)
		}
		line := fmt.Sprintf(`{"type":"custom","id":"%d","parentId":%s,"customType":"state","data":%d}`+"\n", i, parent, i)
		if _, err := file.WriteString(line); err != nil {
			t.Fatal(err)
		}
	}
	manager, err := codingagent.OpenSessionManager(path, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(manager.GetEntries()) != 10001 || *manager.GetLeafID() != "10000" || len(manager.GetBranch()) != 10001 {
		t.Fatal("large file or many-entry history was truncated")
	}
}
