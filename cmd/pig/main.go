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
		os.Exit(1)
	}
}
