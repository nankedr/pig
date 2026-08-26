//go:build dragonfly || freebsd || netbsd || openbsd

package codingagent

const terminalGetAttributes = 0x402c7413 // TIOCGETA on supported BSD release targets
