package tui_test

import (
	"encoding/json"
	"github.com/nankedr/pig/tui"
	"os"
	"reflect"
	"testing"
	"time"
)

func TestStdinBufferParity111(t *testing.T) {
	data, err := os.ReadFile("../parity/oracle/fixtures/keys.json")
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		Events    [][2]string
		Remainder string
		Flushed   []string
	}
	var f struct {
		Case        struct{ Input struct{ Buffers [][]string } }
		Observation struct{ Outcome struct{ Buffers []result } }
	}
	if err = json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	for i, chunks := range f.Case.Input.Buffers {
		got := result{Events: [][2]string{}}
		timeout := int64(10000)
		b := tui.NewStdinBuffer(tui.StdinBufferOptions{Timeout: &timeout})
		b.SetHandlers(tui.StdinBufferEventMap{Data: func(s string) { got.Events = append(got.Events, [2]string{"data", s}) }, Paste: func(s string) { got.Events = append(got.Events, [2]string{"paste", s}) }})
		for _, chunk := range chunks {
			if err = b.Process([]byte(chunk)); err != nil {
				t.Fatal(err)
			}
		}
		got.Remainder, _ = b.GetBuffer()
		got.Flushed, _ = b.Flush()
		b.Destroy()
		if !reflect.DeepEqual(got, f.Observation.Outcome.Buffers[i]) {
			t.Fatalf("case %d got %#v want %#v", i, got, f.Observation.Outcome.Buffers[i])
		}
	}
}
func TestStdinBufferUTF8AndLifecycle111(t *testing.T) {
	events := make(chan string, 32)
	b := tui.NewStdinBuffer()
	b.SetHandlers(tui.StdinBufferEventMap{Data: func(s string) { events <- s }})
	for _, value := range []byte("中😀") {
		if err := b.Process([]byte{value}); err != nil {
			t.Fatal(err)
		}
	}
	if a, c := <-events, <-events; a != "中" || c != "😀" {
		t.Fatalf("UTF8 %q %q", a, c)
	}
	b.Process([]byte("\x1b"))
	select {
	case got := <-events:
		if got != "\x1b" {
			t.Fatalf("escape %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("no timeout flush")
	}
	b.Process([]byte("\x1b["))
	b.Clear()
	b.Process([]byte("\x1b"))
	b.Destroy()
	select {
	case got := <-events:
		t.Fatalf("late callback %q", got)
	case <-time.After(40 * time.Millisecond):
	}
}
