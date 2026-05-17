package mcp

import (
	"context"
	"errors"
)

// ErrTransportClosed is returned by Send and Receive when the transport has
// already been closed.
var ErrTransportClosed = errors.New("transport closed")

// Transport abstracts the low-level byte channel between the JSON-RPC client
// and a remote peer. Implementations must be safe for concurrent use.
//
// Contract:
//   - Send and Receive block until the operation completes or ctx is cancelled.
//   - Send and Receive operate on complete message payloads. Framing (e.g.
//     Content-Length headers) is the implementation's responsibility.
//   - After Close returns, all subsequent Send and Receive calls must return
//     ErrTransportClosed.
//   - Close is idempotent: calling it more than once must not panic or return
//     an error on subsequent calls.
type Transport interface {
	// Send transmits msg to the remote peer. It returns an error if the
	// transport is closed or ctx is cancelled before the write completes.
	Send(ctx context.Context, msg []byte) error

	// Receive waits for the next incoming message and returns its raw bytes.
	// It returns ErrTransportClosed when the transport has been closed, or
	// ctx.Err() if the context is cancelled before a message arrives.
	Receive(ctx context.Context) ([]byte, error)

	// Close terminates the transport. Implementations should unblock any
	// goroutine currently blocked inside Receive. Close is safe to call
	// concurrently and more than once.
	Close() error
}
