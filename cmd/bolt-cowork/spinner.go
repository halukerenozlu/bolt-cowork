package main

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

var spinChars = []string{"|", "/", "-", "\\"}

// Spinner prints an animated spinner to a writer while a task is in progress.
// It is a no-op when the output is not a TTY (e.g. piped output).
type Spinner struct {
	msg  string
	out  io.Writer
	done chan struct{}
	once sync.Once
	tty  bool
}

func newSpinner(out io.Writer, msg string) *Spinner {
	tty := false
	if f, ok := out.(*os.File); ok {
		if st, err := f.Stat(); err == nil {
			tty = (st.Mode() & os.ModeCharDevice) != 0
		}
	}
	return &Spinner{msg: msg, out: out, done: make(chan struct{}), tty: tty}
}

func (s *Spinner) Start() {
	if !s.tty {
		return
	}
	go s.run()
}

func (s *Spinner) Stop() {
	s.once.Do(func() {
		close(s.done)
		if s.tty {
			fmt.Fprintf(s.out, "\r%-*s\r", len(s.msg)+6, "")
		}
	})
}

func (s *Spinner) run() {
	i := 0
	for {
		select {
		case <-s.done:
			return
		case <-time.After(120 * time.Millisecond):
			fmt.Fprintf(s.out, "\r  %s %s", spinChars[i%len(spinChars)], s.msg)
			i++
		}
	}
}
