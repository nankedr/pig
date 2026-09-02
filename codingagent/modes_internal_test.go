package codingagent

import (
	"io"
	"strings"
	"testing"
)

func TestJSONLineWriterRejectsShortWrites(t *testing.T) {
	writer := &jsonLineWriter{output: shortWriter{}}
	writer.Write(map[string]string{"type": "event"})

	if got := writer.Err(); got == nil || got.Error() != "Error: Failed to write stdout." {
		t.Fatalf("JSONL short-write error = %v, want stable stdout failure", got)
	}
}

func TestJSONLineWriterPreservesEncodingErrors(t *testing.T) {
	writer := &jsonLineWriter{output: io.Discard}
	writer.Write(func() {})

	if got := writer.Err(); got == nil || !strings.Contains(got.Error(), "json: unsupported type: func()") {
		t.Fatalf("JSONL encoding error = %v, want wrapped unsupported-type error", got)
	}
}

type shortWriter struct{}

func (shortWriter) Write(value []byte) (int, error) {
	return len(value) - 1, nil
}
