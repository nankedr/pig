//go:build darwin || linux

package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPigFullscreenReplyAfterNotices125(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":\"REPLY_%d\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n", calls.Add(1))
	}))
	defer server.Close()
	tty, _, done := startQueues116(t, server.URL, `{"tuiMode":"fullscreen","quietStartup":true,"compaction":{"enabled":false}}`)
	tty.Send(t, "first\r")
	tty.Wait(t, "REPLY_1")
	for range 3 {
		tty.Send(t, "/session\r")
		tty.Wait(t, "Session Info")
		time.Sleep(150 * time.Millisecond)
	}
	tty.Send(t, "second\r")
	tty.Wait(t, "REPLY_2")
	if !strings.Contains(tty.ScreenText(), "REPLY_2") {
		t.Fatal("reply generated but hidden below old notices")
	}
	exitQueues116(t, tty, done)
}
