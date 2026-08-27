//go:build !aix && !darwin && !dragonfly && !freebsd && !illumos && !ios && !linux && !netbsd && !openbsd && !solaris

package parity

import "os"

func exitSignal(*os.ProcessState) string { return "" }
