package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"

	"github.com/nankedr/pig/codingagent"
	"golang.org/x/term"
)

func main() {
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(context.Canceled)
	received := make(chan os.Signal, 1)
	signal.Notify(received, shutdownSignals...)
	defer signal.Stop(received)
	interrupted := errors.New("received SIGINT")
	go func() {
		select {
		case sig := <-received:
			cause := context.Canceled
			if sig == os.Interrupt {
				cause = errors.Join(cause, interrupted)
			}
			cancel(cause)
		case <-ctx.Done():
		}
	}()
	if err := codingagent.Main(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		exitCode := 1
		if failure, ok := err.(interface{ ExitCode() int }); ok {
			exitCode = failure.ExitCode()
		}
		os.Exit(exitCode)
	}
	parsed := codingagent.ParseArgs(os.Args[1:])
	if errors.Is(context.Cause(ctx), interrupted) && !parsed.Print && parsed.Mode != codingagent.ModeJSON && parsed.Mode != codingagent.ModeRPC && term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd())) {
		signal.Stop(received)
		signal.Reset(os.Interrupt)
		self, err := os.FindProcess(os.Getpid())
		if err == nil && self.Signal(os.Interrupt) == nil {
			select {}
		}
		os.Exit(130)
	}
}
