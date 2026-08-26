//go:build solaris

package codingagent

import "os"

// Solaris is outside the M1 native-platform gate. Defaulting to non-terminal
// preserves safe headless behavior without relying on unsupported raw syscalls.
func isTerminalFile(_ *os.File) bool {
	return false
}
