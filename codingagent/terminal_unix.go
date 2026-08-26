//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package codingagent

import (
	"os"
	"syscall"
	"unsafe"
)

// isTerminalFile asks the kernel for terminal attributes. A character device
// alone is insufficient: /dev/null is a character device but not a terminal.
func isTerminalFile(file *os.File) bool {
	if file == nil {
		return false
	}
	var attributes [256]byte
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, file.Fd(), terminalGetAttributes, uintptr(unsafe.Pointer(&attributes[0])))
	return errno == 0
}
