package main

import (
	"fmt"
	"os"

	"github.com/nankedr/pig/internal/pigaicli"
)

func main() {
	if err := pigaicli.Run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
