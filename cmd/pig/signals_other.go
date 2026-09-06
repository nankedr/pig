//go:build !darwin && !linux

package main

import "os"

var shutdownSignals = []os.Signal{os.Interrupt}
