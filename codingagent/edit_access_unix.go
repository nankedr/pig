//go:build unix

package codingagent

import "syscall"

func editFileAccess(path string) error { return syscall.Access(path, 6) }
