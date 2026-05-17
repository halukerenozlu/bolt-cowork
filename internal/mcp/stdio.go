package mcp

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

// StdioTransport implements Transport over a pair of io.Reader / io.Writer
// using the LSP Content-Length framing protocol:
//
//	Content-Length: <n>\r\n
//	\r\n
//	<n bytes of JSON body>
//
// It is safe for concurrent use from multiple goroutines.
type StdioTransport struct {
	r         *bufio.Reader
	w         io.Writer
	rmu       chan struct{} // buffered-1 semaphore guarding r
	wmu       chan struct{} // buffered-1 semaphore guarding w
	closed    atomic.Bool   // 1 after Close
	closeOnce sync.Once     // ensures Close body runs exactly once
	rc        io.Closer     // optional; closed by Close to unblock Receive
	wc        io.Closer     // optional; closed by Close
}

// compile-time interface check
var _ Transport = (*StdioTransport)(nil)

// NewStdioTransport wraps r and w in a StdioTransport. If r or w also
// implement io.Closer, pass them as rc / wc so that Close can unblock
// any goroutine currently blocked inside Receive or Send.
//
// For most callers the convenience is to pass the same value for both
// the reader/writer and the closer:
//
//	t := NewStdioTransport(stdin, stdout, stdin, stdout)
func NewStdioTransport(r io.Reader, w io.Writer, rc, wc io.Closer) *StdioTransport {
	return &StdioTransport{
		r:   bufio.NewReader(r),
		w:   w,
		rmu: make(chan struct{}, 1),
		wmu: make(chan struct{}, 1),
		rc:  rc,
		wc:  wc,
	}
}

// Send transmits msg to the remote peer using Content-Length framing.
// It returns ErrTransportClosed if the transport has been closed, or
// ctx.Err() if the context is cancelled before the write completes.
func (t *StdioTransport) Send(ctx context.Context, msg []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if t.closed.Load() {
		return ErrTransportClosed
	}

	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(msg))

	// Acquire write semaphore, respecting context cancellation.
	select {
	case t.wmu <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-t.wmu }()

	// Re-check after acquiring semaphore; Close may have arrived concurrently.
	if t.closed.Load() {
		return ErrTransportClosed
	}

	cancelDone := make(chan struct{})
	stopCancel := context.AfterFunc(ctx, func() {
		_ = t.Close()
		close(cancelDone)
	})
	stopCancelAndWait := func() {
		if !stopCancel() {
			<-cancelDone
		}
	}

	if _, err := io.WriteString(t.w, header); err != nil {
		stopCancelAndWait()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("mcp/stdio: Send header: %w", err)
	}
	if _, err := t.w.Write(msg); err != nil {
		stopCancelAndWait()
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("mcp/stdio: Send body: %w", err)
	}
	stopCancelAndWait()
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

// Receive waits for the next incoming message and returns its raw bytes.
// It returns ErrTransportClosed when the transport has been closed, or
// ctx.Err() if the context is cancelled before a message arrives.
func (t *StdioTransport) Receive(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if t.closed.Load() {
		return nil, ErrTransportClosed
	}

	// Acquire read semaphore, respecting context cancellation.
	select {
	case t.rmu <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	defer func() { <-t.rmu }()

	// Re-check after acquiring semaphore; Close may have arrived concurrently.
	if t.closed.Load() {
		return nil, ErrTransportClosed
	}

	cancelDone := make(chan struct{})
	stopCancel := context.AfterFunc(ctx, func() {
		_ = t.Close()
		close(cancelDone)
	})
	stopCancelAndWait := func() {
		if !stopCancel() {
			<-cancelDone
		}
	}
	defer stopCancelAndWait()

	// Parse headers until we hit the blank CRLF separator.
	contentLength := -1
	for {
		line, err := t.r.ReadString('\n')
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			if t.closed.Load() {
				return nil, ErrTransportClosed
			}
			return nil, fmt.Errorf("mcp/stdio: Receive header: %w", err)
		}

		// Strip trailing \r\n or \n.
		line = strings.TrimRight(line, "\r\n")

		// Blank line signals end of headers.
		if line == "" {
			break
		}

		const prefix = "Content-Length: "
		if strings.HasPrefix(line, prefix) {
			val := strings.TrimPrefix(line, prefix)
			n, parseErr := strconv.Atoi(val)
			if parseErr != nil || n < 0 {
				return nil, fmt.Errorf("mcp/stdio: invalid Content-Length: %q", val)
			}
			contentLength = n
		}
		// Unknown headers are silently ignored (forward-compatible).
	}

	if contentLength < 0 {
		return nil, fmt.Errorf("mcp/stdio: Receive: missing Content-Length header")
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	body := make([]byte, contentLength)
	if _, err := io.ReadFull(t.r, body); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		if t.closed.Load() {
			return nil, ErrTransportClosed
		}
		return nil, fmt.Errorf("mcp/stdio: Receive body: %w", err)
	}
	return body, nil
}

// Close terminates the transport. It is safe to call concurrently and more
// than once. The first call sets the closed flag and closes the underlying
// reader/writer (if they implement io.Closer), which unblocks any goroutine
// currently blocked inside Receive.
func (t *StdioTransport) Close() error {
	var firstErr error
	t.closeOnce.Do(func() {
		t.closed.Store(true)
		if t.rc != nil {
			if err := t.rc.Close(); err != nil {
				firstErr = err
			}
		}
		if t.wc != nil && t.wc != t.rc {
			if err := t.wc.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	})
	return firstErr
}
