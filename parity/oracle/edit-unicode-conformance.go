//go:build ignore

package main

import (
	"bufio"
	"context"
	"fmt"
	"github.com/nankedr/pig/codingagent"
	"os"
	"strconv"
	"strings"
)

type ops struct{ content string }

func (o *ops) Access(context.Context, string) error             { return nil }
func (o *ops) ReadFile(context.Context, string) ([]byte, error) { return []byte(o.content), nil }
func (o *ops) WriteFile(_ context.Context, _ string, b []byte) error {
	o.content = string(b)
	return nil
}
func main() {
	if len(os.Args) != 2 {
		panic("usage: go run parity/oracle/edit-unicode-conformance.go <NormalizationTest-16.0.0.txt>")
	}
	file, err := os.Open(os.Args[1])
	if err != nil {
		panic(err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	operation := &ops{}
	tool, _ := codingagent.CreateEditToolDefinition("/tmp", codingagent.EditToolOptions{Operations: operation})
	count := 0
	for scanner.Scan() {
		line := strings.TrimSpace(strings.Split(scanner.Text(), "#")[0])
		if line == "" || line[0] == '@' {
			continue
		}
		fields := strings.Split(line, ";")
		var cases []string
		for _, field := range fields[:5] {
			var out strings.Builder
			for _, code := range strings.Fields(field) {
				r, err := strconv.ParseInt(code, 16, 32)
				if err != nil {
					panic(err)
				}
				out.WriteRune(rune(r))
			}
			cases = append(cases, out.String())
		}
		for _, content := range cases {
			operation.content = "START:" + content + ":END"
			_, err := tool.Execute(context.Background(), "test", map[string]any{"path": "normalization-conformance", "edits": []any{map[string]any{"oldText": "START:" + cases[3] + ":END", "newText": "done"}}}, nil)
			if err != nil || operation.content != "done" {
				panic(fmt.Sprintf("case %d %s: %v got %q", count, line, err, operation.content))
			}
			count++
		}
	}
	if err := scanner.Err(); err != nil {
		panic(err)
	}
	fmt.Printf("PASS %d Unicode16 cases through public edit definition\n", count)
}
