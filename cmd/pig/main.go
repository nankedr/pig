package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/nankedr/pig/codingagent"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := codingagent.Main(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		exitCode := 1
		if failure, ok := err.(interface{ ExitCode() int }); ok {
			exitCode = failure.ExitCode()
		}
		os.Exit(exitCode)
	}
}
