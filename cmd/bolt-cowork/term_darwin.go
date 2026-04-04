//go:build darwin

package main

import (
	"os"
	"syscall"
	"unsafe"
)

const (
	ioctlGetTermios = syscall.TIOCGETA
	ioctlSetTermios = syscall.TIOCSETA
)

// readMasked reads a line with echo disabled on macOS terminals.
// Falls back to readLinePlain if stdin is not a terminal.
func readMasked() (string, error) {
	fd := os.Stdin.Fd()

	var termios syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(ioctlGetTermios), uintptr(unsafe.Pointer(&termios)))
	if errno != 0 {
		return readLinePlain()
	}

	old := termios
	termios.Lflag &^= syscall.ECHO
	_, _, errno = syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(ioctlSetTermios), uintptr(unsafe.Pointer(&termios)))
	if errno != 0 {
		return readLinePlain()
	}
	defer syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(ioctlSetTermios), uintptr(unsafe.Pointer(&old)))

	return readLinePlain()
}
