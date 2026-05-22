//go:build windows

package main

import (
	"bufio"
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleMode = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode = kernel32.NewProc("SetConsoleMode")
)

const (
	enableEchoInput = 0x0004
)

// readMasked reads a line with echo disabled on Windows consoles.
// Falls back to readLineMasked if stdin is not a console.
func readMasked(reader *bufio.Reader) (string, error) {
	handle := os.Stdin.Fd()

	var mode uint32
	r, _, err := procGetConsoleMode.Call(handle, uintptr(unsafe.Pointer(&mode)))
	if r == 0 {
		_ = err
		return readLineMasked(reader)
	}

	newMode := mode &^ enableEchoInput
	r, _, err = procSetConsoleMode.Call(handle, uintptr(newMode))
	if r == 0 {
		_ = err
		return readLineMasked(reader)
	}
	defer procSetConsoleMode.Call(handle, uintptr(mode))

	return readLineMasked(reader)
}

