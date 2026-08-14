package main

import (
	"fmt"
	"os"

	"github.com/nankedr/pig/ai"
)

func main() {
	fmt.Fprintln(os.Stderr, &ai.NotImplementedError{Module: "ai", Operation: "command"})
	os.Exit(1)
}
