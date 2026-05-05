package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
)

// errInterrupted is returned when Ctrl+C (0x03) is detected during input.
var errInterrupted = errors.New("interrupted")

// readLineMasked reads a line from reader byte-by-byte, printing '*' for each
// character to stderr. It returns the input without the trailing newline.
func readLineMasked(reader *bufio.Reader) (string, error) {
	var buf []byte
	for {
		b, err := reader.ReadByte()
		if err != nil {
			return string(buf), err
		}
		if b == '\n' {
			fmt.Fprint(os.Stderr, "\n")
			return string(buf), nil
		}
		if b == '\r' {
			// Consume trailing \n if this is a \r\n sequence.
			next, err := reader.Peek(1)
			if err == nil && next[0] == '\n' {
				_, _ = reader.ReadByte()
			}
			fmt.Fprint(os.Stderr, "\n")
			return string(buf), nil
		}
		// Handle backspace.
		if b == 127 || b == 8 {
			if len(buf) > 0 {
				buf = buf[:len(buf)-1]
				fmt.Fprint(os.Stderr, "\b \b")
			}
			continue
		}
		buf = append(buf, b)
		fmt.Fprint(os.Stderr, "*")
	}
}
