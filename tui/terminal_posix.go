//go:build darwin || linux

package tui

import (
	"errors"
	"io"
	"os"

	"golang.org/x/sys/unix"
	"golang.org/x/term"
)

func terminalSupported() error { return nil }
func readTerminal(file *os.File, stop <-chan struct{}, onInput func(string), onResize func()) error {
	fd := int(file.Fd())
	buffer := make([]byte, 4096)
	width, height, _ := term.GetSize(fd)
	for {
		select {
		case <-stop:
			return nil
		default:
		}
		descriptors := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN}}
		_, err := unix.Poll(descriptors, 50)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return err
		}
		select {
		case <-stop:
			return nil
		default:
		}
		if w, h, err := term.GetSize(fd); err == nil && (w != width || h != height) {
			width, height = w, h
			if onResize != nil {
				onResize()
			}
		}
		if descriptors[0].Revents == 0 {
			continue
		}
		n, err := unix.Read(fd, buffer)
		if n > 0 && onInput != nil {
			onInput(string(buffer[:n]))
		}
		if errors.Is(err, unix.EINTR) || errors.Is(err, unix.EAGAIN) {
			continue
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return io.EOF
		}
	}
}

func suspendProcess() error { return unix.Kill(os.Getpid(), unix.SIGTSTP) }
