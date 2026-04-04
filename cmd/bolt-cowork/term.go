package main

import (
	"fmt"
	"os"
)

// readLinePlain reads a line from stdin byte-by-byte, printing '*' for each
// character. It returns the input without the trailing newline.
func readLinePlain() (string, error) {
	var buf []byte
	b := make([]byte, 1)
	for {
		n, err := os.Stdin.Read(b)
		if err != nil {
			return string(buf), err
		}
		if n == 0 {
			continue
		}
		ch := b[0]
		if ch == '\n' || ch == '\r' {
			fmt.Fprint(os.Stderr, "\n")
			return string(buf), nil
		}
		// Handle backspace.
		if ch == 127 || ch == 8 {
			if len(buf) > 0 {
				buf = buf[:len(buf)-1]
				fmt.Fprint(os.Stderr, "\b \b")
			}
			continue
		}
		buf = append(buf, ch)
		fmt.Fprint(os.Stderr, "*")
	}
}
