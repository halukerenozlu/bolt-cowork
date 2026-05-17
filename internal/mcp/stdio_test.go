package mcp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

type blockingReadCloser struct {
	started chan struct{}
	done    chan struct{}
	once    sync.Once
}

func newBlockingReadCloser() *blockingReadCloser {
	return &blockingReadCloser{
		started: make(chan struct{}),
		done:    make(chan struct{}),
	}
}

func (r *blockingReadCloser) Read(_ []byte) (int, error) {
	r.once.Do(func() { close(r.started) })
	<-r.done
	return 0, io.ErrClosedPipe
}

func (r *blockingReadCloser) Close() error {
	close(r.done)
	return nil
}

type blockingWriteCloser struct {
	started chan struct{}
	done    chan struct{}
	once    sync.Once
}

func newBlockingWriteCloser() *blockingWriteCloser {
	return &blockingWriteCloser{
		started: make(chan struct{}),
		done:    make(chan struct{}),
	}
}

func (w *blockingWriteCloser) Write(_ []byte) (int, error) {
	w.once.Do(func() { close(w.started) })
	<-w.done
	return 0, io.ErrClosedPipe
}

func (w *blockingWriteCloser) Close() error {
	close(w.done)
	return nil
}

// slowReader wraps an io.Reader and introduces a small delay on each Read
// call. This forces concurrent Receive calls to overlap inside the read
// path, making race conditions deterministic rather than scheduler-dependent.
type slowReader struct {
	r io.Reader
}

func (s *slowReader) Read(p []byte) (int, error) {
	time.Sleep(1 * time.Millisecond)
	return s.r.Read(p)
}

// loopbackTransport returns a StdioTransport whose Send output is fed
// directly into its own Receive via an in-memory pipe.
func loopbackTransport() (*StdioTransport, func()) {
	pr, pw := io.Pipe()
	t := NewStdioTransport(pr, pw, pr, pw)
	return t, func() { _ = t.Close() }
}

// TestStdioTransport_RoundTrip verifies that a single message sent through
// the transport is received back intact.
func TestStdioTransport_RoundTrip(t *testing.T) {
	cases := []struct {
		name    string
		payload string
	}{
		{"empty_json", `{}`},
		{"simple_object", `{"jsonrpc":"2.0","id":1,"method":"ping"}`},
		{"unicode", `{"value":"konnichiwa"}`},
		{"large_payload", strings.Repeat("x", 4096)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tr, cleanup := loopbackTransport()
			defer cleanup()

			ctx := context.Background()
			sent := []byte(tc.payload)

			errCh := make(chan error, 1)
			go func() {
				errCh <- tr.Send(ctx, sent)
			}()

			got, err := tr.Receive(ctx)
			if err != nil {
				t.Fatalf("Receive error: %v", err)
			}
			if sendErr := <-errCh; sendErr != nil {
				t.Fatalf("Send error: %v", sendErr)
			}
			if string(got) != string(sent) {
				t.Fatalf("payload mismatch\nsent: %q\ngot:  %q", sent, got)
			}
		})
	}
}

// TestStdioTransport_Sequential sends N messages in order and verifies
// they are received in the same order.
func TestStdioTransport_Sequential(t *testing.T) {
	tr, cleanup := loopbackTransport()
	defer cleanup()

	ctx := context.Background()
	const n = 5

	for i := 0; i < n; i++ {
		msg := []byte(fmt.Sprintf(`{"seq":%d}`, i))

		errCh := make(chan error, 1)
		go func(m []byte) { errCh <- tr.Send(ctx, m) }(msg)

		got, err := tr.Receive(ctx)
		if err != nil {
			t.Fatalf("seq %d: Receive error: %v", i, err)
		}
		if sendErr := <-errCh; sendErr != nil {
			t.Fatalf("seq %d: Send error: %v", i, sendErr)
		}
		want := fmt.Sprintf(`{"seq":%d}`, i)
		if string(got) != want {
			t.Fatalf("seq %d: got %q, want %q", i, got, want)
		}
	}
}

// TestStdioTransport_Closed verifies that both Send and Receive return
// ErrTransportClosed after Close is called.
func TestStdioTransport_Closed(t *testing.T) {
	t.Run("Send_after_Close", func(t *testing.T) {
		tr, cleanup := loopbackTransport()
		defer cleanup()

		if err := tr.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		err := tr.Send(context.Background(), []byte(`{}`))
		if !errors.Is(err, ErrTransportClosed) {
			t.Fatalf("Send after Close: got %v, want ErrTransportClosed", err)
		}
	})

	t.Run("Receive_after_Close", func(t *testing.T) {
		tr, cleanup := loopbackTransport()
		defer cleanup()

		if err := tr.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
		_, err := tr.Receive(context.Background())
		if !errors.Is(err, ErrTransportClosed) {
			t.Fatalf("Receive after Close: got %v, want ErrTransportClosed", err)
		}
	})

	t.Run("Close_is_idempotent", func(t *testing.T) {
		tr, cleanup := loopbackTransport()
		defer cleanup()

		for i := 0; i < 3; i++ {
			if err := tr.Close(); err != nil {
				t.Fatalf("Close call %d: %v", i+1, err)
			}
		}
	})

	t.Run("Receive_unblocked_by_Close", func(t *testing.T) {
		tr, cleanup := loopbackTransport()
		defer cleanup()

		errCh := make(chan error, 1)
		go func() {
			_, err := tr.Receive(context.Background())
			errCh <- err
		}()

		// Close the transport; Receive should unblock.
		if err := tr.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}

		err := <-errCh
		if !errors.Is(err, ErrTransportClosed) {
			t.Fatalf("Receive after concurrent Close: got %v, want ErrTransportClosed", err)
		}
	})
}

// TestStdioTransport_MalformedHeader verifies that Receive returns a non-nil
// error when the framing headers are invalid.
func TestStdioTransport_MalformedHeader(t *testing.T) {
	cases := []struct {
		name  string
		frame string // raw bytes written to the pipe
	}{
		{
			name:  "missing_Content-Length",
			frame: "X-Foo: bar\r\n\r\n{}",
		},
		{
			name:  "negative_Content-Length",
			frame: "Content-Length: -1\r\n\r\n{}",
		},
		{
			name:  "non_numeric_Content-Length",
			frame: "Content-Length: abc\r\n\r\n{}",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pr, pw := io.Pipe()
			tr := NewStdioTransport(pr, pw, pr, pw)
			defer func() { _ = tr.Close() }()

			go func() {
				_, _ = io.WriteString(pw, tc.frame)
			}()

			_, err := tr.Receive(context.Background())
			if err == nil {
				t.Fatal("expected error for malformed header, got nil")
			}
		})
	}
}

// TestStdioTransport_ShortBody verifies that Receive returns an error when
// the body is shorter than the declared Content-Length.
func TestStdioTransport_ShortBody(t *testing.T) {
	pr, pw := io.Pipe()
	tr := NewStdioTransport(pr, pw, pr, pw)
	// Don't defer tr.Close() here - we close pw early to trigger the error.

	go func() {
		// Claim 100 bytes but write only 5 bytes, then close the write end.
		_, _ = io.WriteString(pw, "Content-Length: 100\r\n\r\nhello")
		_ = pw.Close()
	}()

	_, err := tr.Receive(context.Background())
	if err == nil {
		t.Fatal("expected error for short body, got nil")
	}
	// Cleanup the read end.
	_ = pr.Close()
}

// TestStdioTransport_ContextCancelled verifies that a pre-cancelled context causes
// Send to return the context error without writing.
func TestStdioTransport_ContextCancelled(t *testing.T) {
	tr, cleanup := loopbackTransport()
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	err := tr.Send(ctx, []byte(`{}`))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Send with cancelled ctx: got %v, want context.Canceled", err)
	}
}

// TestReceive_ContextCancelledWhileBlocked verifies that cancelling ctx while
// Receive is blocked waiting for a header unblocks it with context.Canceled.
// Close is NOT called by this test; context cancellation alone must suffice.
func TestReceive_ContextCancelledWhileBlocked(t *testing.T) {
	br := newBlockingReadCloser()
	tr := NewStdioTransport(br, io.Discard, br, nil)
	defer func() { _ = tr.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		_, err := tr.Receive(ctx)
		errCh <- err
	}()

	<-br.started
	// Cancel the context; the goroutine-close pattern must unblock Receive.
	cancel()

	err := <-errCh
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Receive unblocked by ctx cancel: got %v, want context.Canceled", err)
	}
}

// TestReceive_AfterCancelledReceive verifies that cancelling a blocked
// Receive transitions the whole transport to the closed state.
func TestReceive_AfterCancelledReceive(t *testing.T) {
	br := newBlockingReadCloser()
	tr := NewStdioTransport(br, io.Discard, br, nil)
	defer func() { _ = tr.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		_, err := tr.Receive(ctx)
		errCh <- err
	}()

	<-br.started
	cancel()

	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Receive: got %v, want context.Canceled", err)
	}

	_, err := tr.Receive(context.Background())
	if !errors.Is(err, ErrTransportClosed) {
		t.Fatalf("Receive after cancelled Receive: got %v, want ErrTransportClosed", err)
	}
}

// TestSend_ContextCancelledWhileBlocked verifies that cancelling ctx while
// Send is blocked on a write unblocks it with context.Canceled.
// Close is NOT called by this test; context cancellation alone must suffice.
func TestSend_ContextCancelledWhileBlocked(t *testing.T) {
	bw := newBlockingWriteCloser()
	tr := NewStdioTransport(strings.NewReader(""), bw, nil, bw)
	defer func() { _ = tr.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- tr.Send(ctx, []byte(`{"method":"ping"}`))
	}()

	<-bw.started
	// Cancel the context; the goroutine-close pattern must unblock Send.
	cancel()

	err := <-errCh
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Send unblocked by ctx cancel: got %v, want context.Canceled", err)
	}
}

// TestSend_AfterCancelledSend verifies that cancelling a blocked Send
// transitions the whole transport to the closed state.
func TestSend_AfterCancelledSend(t *testing.T) {
	bw := newBlockingWriteCloser()
	tr := NewStdioTransport(strings.NewReader(""), bw, nil, bw)
	defer func() { _ = tr.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- tr.Send(ctx, []byte(`{"method":"ping"}`))
	}()

	<-bw.started
	cancel()

	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Send: got %v, want context.Canceled", err)
	}

	err := tr.Send(context.Background(), []byte(`{"method":"again"}`))
	if !errors.Is(err, ErrTransportClosed) {
		t.Fatalf("Send after cancelled Send: got %v, want ErrTransportClosed", err)
	}
}

// TestConcurrentReceive verifies that concurrent Receive calls read complete
// messages one at a time without corrupting the shared bufio.Reader state.
// A slowReader forces both goroutines to overlap inside Read, making the
// race deterministic rather than scheduler-dependent.
func TestConcurrentReceive(t *testing.T) {
	pr, pw := io.Pipe()
	sr := &slowReader{r: pr}
	tr := NewStdioTransport(sr, pw, pr, pw) // slow reader for I/O, pr for Close
	defer func() { _ = tr.Close() }()

	messages := []string{`{"seq":1}`, `{"seq":2}`}
	results := make(chan string, len(messages))
	errs := make(chan error, len(messages))

	for i := 0; i < len(messages); i++ {
		go func() {
			got, err := tr.Receive(context.Background())
			if err != nil {
				errs <- err
				return
			}
			results <- string(got)
		}()
	}

	go func() {
		for _, msg := range messages {
			_, _ = fmt.Fprintf(pw, "Content-Length: %d\r\n\r\n%s", len(msg), msg)
		}
	}()

	seen := make(map[string]int, len(messages))
	for i := 0; i < len(messages); i++ {
		select {
		case err := <-errs:
			t.Fatalf("Receive error: %v", err)
		case got := <-results:
			seen[got]++
		}
	}

	for _, want := range messages {
		if seen[want] != 1 {
			t.Fatalf("message %q received %d times, want once; seen=%v", want, seen[want], seen)
		}
	}
}
