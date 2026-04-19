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
	enableEchoInput      = 0x0004
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

// readREPLLine reads a line for the REPL prompt on Windows.
// We keep the console in cooked mode so native line editing works
// (left/right arrows, home/end, paste shortcuts, etc.).
// Ctrl+C is handled by the REPL signal handler.
func readREPLLine(reader *bufio.Reader) (string, error) {
	return readFallbackLine(reader)
}

// readFallbackLine is used when stdin is not a console (piped input).
func readFallbackLine(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	// Trim trailing \r\n or \n.
	line = line[:len(line)-1]
	if len(line) > 0 && line[len(line)-1] == '\r' {
		line = line[:len(line)-1]
	}
	return line, nil
}
