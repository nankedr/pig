package main

import (
	"fmt"
	"os"

	"github.com/nankedr/pig/codingagent"
)

func main() {
	fmt.Fprintln(os.Stderr, &codingagent.NotImplementedError{Module: "codingagent", Operation: "command"})
	os.Exit(1)
}
