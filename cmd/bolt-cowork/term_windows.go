//go:build windows

package main

import (
	"bufio"
	"fmt"
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
	enableProcessedInput = 0x0001
	enableLineInput      = 0x0002
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
// Switches to raw mode so Ctrl+C is received as byte 0x03 instead of
// generating a signal. Returns errInterrupted on Ctrl+C.
// Console mode is restored before returning so that subsequent reads
// (e.g. approval prompts during command execution) work normally.
func readREPLLine(reader *bufio.Reader) (string, error) {
	handle := os.Stdin.Fd()

	var oldMode uint32
	r, _, _ := procGetConsoleMode.Call(handle, uintptr(unsafe.Pointer(&oldMode)))
	if r == 0 {
		// Not a console — fall back to simple read.
		return readFallbackLine(reader)
	}

	// Disable PROCESSED_INPUT (Ctrl+C becomes 0x03), LINE_INPUT (byte-by-byte),
	// and ECHO_INPUT (we echo manually).
	raw := oldMode &^ (enableProcessedInput | enableLineInput | enableEchoInput)
	r, _, _ = procSetConsoleMode.Call(handle, uintptr(raw))
	if r == 0 {
		return readFallbackLine(reader)
	}
	defer procSetConsoleMode.Call(handle, uintptr(oldMode))

	var buf []byte
	for {
		b, err := reader.ReadByte()
		if err != nil {
			return string(buf), err
		}
		switch {
		case b == 0x03: // Ctrl+C
			fmt.Fprint(os.Stderr, "\n")
			return "", errInterrupted
		case b == '\r':
			// Consume trailing \n in CRLF.
			next, peekErr := reader.Peek(1)
			if peekErr == nil && next[0] == '\n' {
				_, _ = reader.ReadByte()
			}
			fmt.Fprint(os.Stderr, "\n")
			return string(buf), nil
		case b == '\n':
			fmt.Fprint(os.Stderr, "\n")
			return string(buf), nil
		case b == 8 || b == 127: // Backspace
			if len(buf) > 0 {
				buf = buf[:len(buf)-1]
				fmt.Fprint(os.Stderr, "\b \b")
			}
		case b < 32: // Other control characters — ignore
			continue
		default:
			buf = append(buf, b)
			fmt.Fprint(os.Stderr, string(rune(b)))
		}
	}
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
