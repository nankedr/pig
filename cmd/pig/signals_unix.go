//go:build darwin || linux

package main

import (
	"os"
	"syscall"
)

var shutdownSignals = []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP}
