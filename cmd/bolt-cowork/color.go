package main

import "os"

const (
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiReset  = "\033[0m"
)

func colorEnabled() bool {
	return os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb"
}

func colorGreen(s string) string  { return colorWrap(ansiGreen, s) }
func colorYellow(s string) string { return colorWrap(ansiYellow, s) }

func colorWrap(code, s string) string {
	if !colorEnabled() {
		return s
	}
	return code + s + ansiReset
}
