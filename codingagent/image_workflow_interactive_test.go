//go:build darwin || linux

package codingagent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/nankedr/pig/ai"
	"github.com/nankedr/pig/codingagent"
	"github.com/nankedr/pig/internal/terminaltest"
	"github.com/nankedr/pig/tui"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestImageWorkflowInteractive136(t *testing.T) {
	path, image := imageFile134(t)
	request := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		raw, _ := json.Marshal(body)
		request <- string(raw)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"type":"response.output_text.delta","delta":"IMAGE_DONE"}`+"\n\n"+`data: {"type":"response.completed","response":{"status":"completed"}}`+"\n\n")
	}))
	defer server.Close()
	key := "offline"
	runtime, err := codingagent.CreateHeadlessSession(t.Context(), codingagent.CreateHeadlessSessionOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), Model: "deepseek-flash", Provider: ai.ProviderIDDeepSeek, API: ai.APIOpenAIResponses, APIKey: &key, BaseURL: &server.URL, NoTools: codingagent.NoToolsAll, NoContextFiles: true})
	if err != nil {
		t.Fatal(err)
	}
	tty := terminaltest.Open(t)
	mode := codingagent.NewInteractiveMode(runtime, codingagent.InteractiveModeOptions{Terminal: tui.NewProcessTerminal(tty.Slave, tty.Slave)})
	t.Cleanup(func() { _ = mode.Stop() })
	done := make(chan error, 1)
	go func() { done <- mode.Run(context.Background()) }()
	tty.Wait(t, "> ")
	tty.Send(t, "/image add "+path+"\r")
	tty.Wait(t, "Pending image 1")
	tty.Send(t, "/image list\r")
	tty.Wait(t, "image/png")
	tty.Send(t, "/image remove 1\r")
	tty.Wait(t, "Removed image 1")
	tty.Send(t, "/image add "+path+"\r")
	tty.Send(t, "describe picture\r")
	tty.Wait(t, "IMAGE_DONE")
	raw := <-request
	if !strings.Contains(raw, "data:image/png;base64,"+image.Data) || !strings.Contains(raw, "describe picture") {
		t.Fatal("text/image not submitted together")
	}
	tty.Send(t, "/image add "+path+"\r")
	tty.Send(t, "/image send\r")
	second := <-request
	if !strings.Contains(second, "data:image/png;base64,"+image.Data) {
		t.Fatal("image-only submission lost attachment")
	}
	if err := mode.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := awaitInteractive(t, done); err != nil {
		t.Fatal(err)
	}
	if !tty.Restored(t) {
		t.Fatal("terminal not restored")
	}
}

func TestImageWorkflowPreflightRestore136(t *testing.T) {
	_, image := imageFile134(t)
	runtime := interactiveRuntime(t) // Faux is text-only: rejection must preserve the pending attachment.
	tty := terminaltest.Open(t)
	prompt := "inspect this image"
	mode := codingagent.NewInteractiveMode(runtime, codingagent.InteractiveModeOptions{Terminal: tui.NewProcessTerminal(tty.Slave, tty.Slave), InitialImages: []ai.ImageContent{image}, InitialMessage: &prompt})
	t.Cleanup(func() { _ = mode.Stop() })
	done := make(chan error, 1)
	go func() { done <- mode.Run(t.Context()) }()
	tty.Wait(t, "Attachments not submitted; restored to pending list")
	tty.Send(t, "\x15/image list\r")
	tty.Wait(t, "Pending image 1")
	if len(runtime.Session().Messages()) != 0 {
		t.Fatal("preflight failure changed session")
	}
	if err := mode.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := awaitInteractive(t, done); err != nil {
		t.Fatal(err)
	}
	if !tty.Restored(t) {
		t.Fatal("failed image submission left raw mode")
	}
}

func TestImageWorkflowCancelAndQueue136(t *testing.T) {
	path, image := imageFile134(t)
	entered := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `data: {"type":"response.output_text.delta","delta":"VISION_WAIT"}`+"\n\n")
		w.(http.Flusher).Flush()
		close(entered)
		<-r.Context().Done()
	}))
	defer server.Close()
	key := "offline"
	runtime, err := codingagent.CreateHeadlessSession(t.Context(), codingagent.CreateHeadlessSessionOptions{CWD: t.TempDir(), AgentDir: t.TempDir(), Model: "deepseek-flash", Provider: ai.ProviderIDDeepSeek, API: ai.APIOpenAIResponses, APIKey: &key, BaseURL: &server.URL, NoTools: codingagent.NoToolsAll, NoContextFiles: true})
	if err != nil {
		t.Fatal(err)
	}
	tty := terminaltest.Open(t)
	prompt := "inspect"
	mode := codingagent.NewInteractiveMode(runtime, codingagent.InteractiveModeOptions{Terminal: tui.NewProcessTerminal(tty.Slave, tty.Slave), InitialMessage: &prompt, InitialImages: []ai.ImageContent{image}})
	defer mode.Stop()
	done := make(chan error, 1)
	go func() { done <- mode.Run(t.Context()) }()
	tty.Wait(t, "VISION_WAIT")
	<-entered
	tty.Send(t, "/image add "+path+"\r")
	tty.Wait(t, "Pending image 1")
	tty.Send(t, "queue with attachment\r")
	tty.Wait(t, "Images cannot enter text-only queues")
	tty.Send(t, "\x1b")
	tty.Wait(t, "Attachments submitted and retained inline in session")
	if count, _ := runtime.Session().PendingMessageCount(); count != 0 {
		t.Fatal("image entered text queue")
	}
	blocks, _ := runtime.Session().Messages()[0].(ai.UserMessage).Content.Blocks()
	if len(blocks) != 2 || blocks[1].(ai.ImageContent) != image {
		t.Fatal("cancel lost accepted session image")
	}
	if err := mode.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := awaitInteractive(t, done); err != nil {
		t.Fatal(err)
	}
	if !tty.Restored(t) {
		t.Fatal("cancel did not restore terminal")
	}
}
