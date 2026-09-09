package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRPC97ExportHTML(t *testing.T) {
	c, ctx := rpcClient95(t, "http://127.0.0.1:1", `{"compaction":{"enabled":false}}`)
	defer c.Stop(context.Background())
	dir := t.TempDir()
	source := filepath.Join(dir, "session.jsonl")
	output := filepath.Join(dir, "rpc.html")
	data, err := os.ReadFile("../../parity/export-html/session.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	cwd, _ := json.Marshal(dir)
	data = []byte(strings.ReplaceAll(string(data), `"cwd":"/project"`, `"cwd":`+string(cwd)))
	if err = os.WriteFile(source, data, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = c.SwitchSession(ctx, source); err != nil {
		t.Fatal(err)
	}
	got, err := c.ExportHTML(ctx, output)
	if err != nil {
		t.Fatal(err)
	}
	html, err := os.ReadFile(got)
	if err != nil || got != output || !strings.Contains(string(html), "Pig Session Export") {
		t.Fatal(got, err)
	}
	if _, err = c.ExportHTML(ctx, filepath.Join(dir, "missing", "out.html")); err == nil {
		t.Fatal("write failure succeeded")
	}
	if _, err = c.GetState(ctx); err != nil {
		t.Fatal("export failure killed RPC", err)
	}
}

func TestRPC97ExportWireErrors(t *testing.T) {
	p := startRPC94(t, buildPigBinary(t))
	for _, command := range []string{`{"id":"bad-path","type":"export_html","outputPath":123}`, `{"id":"memory","type":"export_html"}`} {
		if _, err := p.input.Write([]byte(command + "\n")); err != nil {
			t.Fatal(err)
		}
		got := p.record(t)
		if got["success"] != false || got["command"] != "export_html" || got["error"] == nil {
			t.Fatalf("wrong export failure: %#v", got)
		}
		if strings.Contains(command, "bad-path") && got["id"] != "bad-path" {
			t.Fatal("lost error id")
		}
		if strings.Contains(command, "memory") && !strings.Contains(got["error"].(string), "in-memory") {
			t.Fatal(got)
		}
	}
}
