//go:build darwin || linux

package codingagent_test

import (
	"context"
	"fmt"
	"github.com/nankedr/pig/codingagent"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestSessionBashAbortKillsProcessTree(t *testing.T) {
	for _, dispose := range []bool{false, true} {
		t.Run(fmt.Sprint(dispose), func(t *testing.T) {
			session := newMessage85Session(t, nil)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			var shell, child int
			var output strings.Builder
			r, err := session.ExecuteBash(ctx, `sleep 30 & child=$!; printf '%s %s\n' $$ $child; wait`, codingagent.ExecuteBashOptions{OnChunk: func(chunk string) {
				output.WriteString(chunk)
				if child == 0 {
					fmt.Sscanf(output.String(), "%d %d", &shell, &child)
					if child > 0 {
						if dispose {
							session.Dispose()
						} else {
							session.AbortBash()
						}
					}
				}
			}})
			if shell <= 0 || child <= 0 {
				t.Fatalf("missing process IDs: %q, %v", output.String(), err)
			}
			t.Cleanup(func() { syscall.Kill(shell, syscall.SIGKILL); syscall.Kill(child, syscall.SIGKILL) })
			if err != nil || !r.Cancelled || r.ExitCode != nil || r.Output != output.String() {
				t.Fatalf("cancelled result: %+v %v", r, err)
			}
			for _, pid := range []int{shell, child} {
				deadline := time.Now().Add(2 * time.Second)
				for syscall.Kill(pid, 0) == nil && time.Now().Before(deadline) {
					time.Sleep(10 * time.Millisecond)
				}
				if err := syscall.Kill(pid, 0); err != syscall.ESRCH {
					t.Fatalf("process %d survived: %v", pid, err)
				}
			}
			if len(session.Messages()) != 1 || session.Messages()[0].MessageRole() != "bashExecution" {
				t.Fatal("cancelled Bash history missing")
			}
		})
	}
}
