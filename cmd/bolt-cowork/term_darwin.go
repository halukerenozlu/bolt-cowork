//go:build darwin

package main

import (
	"bufio"
	"os"
	"syscall"
	"unsafe"
)

const (
	ioctlGetTermios = syscall.TIOCGETA
	ioctlSetTermios = syscall.TIOCSETA
)

// readREPLLine reads a line for the REPL prompt. On macOS, Ctrl+C is handled
// by the signal handler so this is a straightforward buffered read.
func readREPLLine(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	line = line[:len(line)-1]
	if len(line) > 0 && line[len(line)-1] == '\r' {
		line = line[:len(line)-1]
	}
	return line, nil
}

// readMasked reads a line with echo disabled on macOS terminals.
// Falls back to readLineMasked if stdin is not a terminal.
func readMasked(reader *bufio.Reader) (string, error) {
	fd := os.Stdin.Fd()

	var termios syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(ioctlGetTermios), uintptr(unsafe.Pointer(&termios)))
	if errno != 0 {
		return readLineMasked(reader)
	}

	old := termios
	termios.Lflag &^= syscall.ECHO
	_, _, errno = syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(ioctlSetTermios), uintptr(unsafe.Pointer(&termios)))
	if errno != 0 {
		return readLineMasked(reader)
	}
	defer syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(ioctlSetTermios), uintptr(unsafe.Pointer(&old)))

	return readLineMasked(reader)
}
