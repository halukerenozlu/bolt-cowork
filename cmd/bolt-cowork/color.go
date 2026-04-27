package main

import "os"

const (
	ansiGreen  = "\033[32m"
	ansiRed    = "\033[31m"
	ansiYellow = "\033[33m"
	ansiCyan   = "\033[36m"
	ansiReset  = "\033[0m"
)

func colorEnabled() bool {
	return os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb"
}

func colorGreen(s string) string  { return colorWrap(ansiGreen, s) }
func colorRed(s string) string    { return colorWrap(ansiRed, s) }
func colorYellow(s string) string { return colorWrap(ansiYellow, s) }
func colorCyan(s string) string   { return colorWrap(ansiCyan, s) }

func colorWrap(code, s string) string {
	if !colorEnabled() {
		return s
	}
	return code + s + ansiReset
}
