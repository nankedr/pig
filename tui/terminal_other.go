//go:build !darwin && !linux

package tui

import "os"

func terminalSupported() error                                           { return newNotImplemented("ProcessTerminal.start.platform") }
func readTerminal(*os.File, <-chan struct{}, func(string), func()) error { return terminalSupported() }
