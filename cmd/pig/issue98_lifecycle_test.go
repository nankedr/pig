package main

import (
	"context"
	"runtime"
	"runtime/pprof"
	"strings"
	"testing"
	"time"
)

func TestRPC98RepeatedStopReclaimsGoroutines(t *testing.T) {
	client, ctx := rpcClient95(t, "http://127.0.0.1:1", `{"retry":{"enabled":false}}`)
	if err := client.Stop(ctx); err != nil {
		t.Fatal(err)
	}
	baseline := runtime.NumGoroutine()
	for i := 0; i < 10; i++ {
		if err := client.Start(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err := client.GetState(ctx); err != nil {
			t.Fatal(err)
		}
		if err := client.Stop(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.Now().Add(3 * time.Second)
	for runtime.NumGoroutine() > baseline+2 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if got := runtime.NumGoroutine(); got > baseline+2 {
		var stack strings.Builder
		_ = pprof.Lookup("goroutine").WriteTo(&stack, 1)
		t.Fatalf("RPC start/stop leaked goroutines: before=%d after=%d\n%s", baseline, got, stack.String())
	}
}
