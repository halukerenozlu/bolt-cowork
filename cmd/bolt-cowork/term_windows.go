//go:build windows

package main

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32              = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleMode    = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode    = kernel32.NewProc("SetConsoleMode")
)

const enableEchoInput = 0x0004

// readMasked reads a line with echo disabled on Windows consoles.
// Falls back to readLinePlain if stdin is not a console.
func readMasked() (string, error) {
	handle := os.Stdin.Fd()

	var mode uint32
	r, _, err := procGetConsoleMode.Call(handle, uintptr(unsafe.Pointer(&mode)))
	if r == 0 {
		// Not a console, fall back.
		_ = err
		return readLinePlain()
	}

	newMode := mode &^ enableEchoInput
	r, _, err = procSetConsoleMode.Call(handle, uintptr(newMode))
	if r == 0 {
		_ = err
		return readLinePlain()
	}
	defer procSetConsoleMode.Call(handle, uintptr(mode))

	return readLinePlain()
}
