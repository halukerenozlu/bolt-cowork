//go:build linux

package main

import (
	"bufio"
	"os"
	"syscall"
	"unsafe"
)

// readMasked reads a line with echo disabled on Linux terminals.
// Falls back to readLineMasked if stdin is not a terminal.
func readMasked(reader *bufio.Reader) (string, error) {
	fd := os.Stdin.Fd()

	var termios syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(syscall.TCGETS), uintptr(unsafe.Pointer(&termios)))
	if errno != 0 {
		return readLineMasked(reader)
	}

	old := termios
	termios.Lflag &^= syscall.ECHO
	_, _, errno = syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(syscall.TCSETS), uintptr(unsafe.Pointer(&termios)))
	if errno != 0 {
		return readLineMasked(reader)
	}
	defer syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(syscall.TCSETS), uintptr(unsafe.Pointer(&old)))

	return readLineMasked(reader)
}
