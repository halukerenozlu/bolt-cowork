package mcp

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"sync"
	"sync/atomic"
)

// jsonRPCVersion is the only supported protocol version string.
const jsonRPCVersion = "2.0"

// Standard JSON-RPC 2.0 pre-defined error codes.
const (
	// CodeParseError is returned when the server receives invalid JSON.
	CodeParseError = -32700

	// CodeInvalidRequest is returned when the JSON sent is not a valid Request object.
	CodeInvalidRequest = -32600

	// CodeMethodNotFound is returned when the requested method does not exist.
	CodeMethodNotFound = -32601

	// CodeInvalidParams is returned when invalid method parameters are supplied.
	CodeInvalidParams = -32602

	// CodeInternalError is returned on an internal JSON-RPC error.
	CodeInternalError = -32603
)

// ID represents a JSON-RPC 2.0 request identifier.
// The spec allows a string, an integer number, or null.
// The zero value of ID is treated as null.
//
// The internal typed representation ensures that Key() always returns the
// decoded canonical form regardless of how the value was encoded in JSON.
// For example, the escape sequences "\u0041" and "A" represent the same
// string and will produce identical Key() values after unmarshaling.
type ID struct {
	kind   string // "int" | "str" | "" (null or zero value)
	n      int64
	s      string
	isNull bool // true when explicitly created as null via IDNull or UnmarshalJSON
}

// IDFromInt creates an ID from an integer value.
func IDFromInt(n int64) ID {
	return ID{kind: "int", n: n}
}

// IDFromString creates an ID from a string value.
func IDFromString(s string) ID {
	return ID{kind: "str", s: s}
}

// IDNull creates an explicit null ID.
func IDNull() ID {
	return ID{isNull: true}
}

// IsNull reports whether the ID is null. Both the zero value and IDs created
// via IDNull return true.
func (id ID) IsNull() bool {
	return id.kind != "int" && id.kind != "str"
}

// Key returns a canonical string representation of the ID suitable for use
// as a map key.
//
// Integer IDs return their decimal form (e.g. "42"). String IDs return their
// JSON-quoted, unicode-decoded form (e.g. the four characters "foo" enclosed
// in double-quotes). Null and zero-value IDs return "null".
//
// Because the string value is decoded before being re-encoded, two IDs that
// carry the same string via different JSON encodings (e.g. "A" and "\u0041")
// always produce the same key.
func (id ID) Key() string {
	switch id.kind {
	case "int":
		return strconv.FormatInt(id.n, 10)
	case "str":
		b, _ := json.Marshal(id.s)
		return string(b)
	default:
		return "null"
	}
}

// MarshalJSON implements json.Marshaler.
func (id ID) MarshalJSON() ([]byte, error) {
	switch id.kind {
	case "int":
		return json.Marshal(id.n)
	case "str":
		return json.Marshal(id.s)
	default:
		return []byte("null"), nil
	}
}

// UnmarshalJSON implements json.Unmarshaler.
// It accepts a JSON string, a JSON integer, or null.
func (id *ID) UnmarshalJSON(data []byte) error {
	if len(data) == 0 {
		return fmt.Errorf("jsonrpc: cannot unmarshal empty bytes as ID")
	}
	if string(data) == "null" {
		*id = IDNull()
		return nil
	}
	if data[0] == '"' {
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return fmt.Errorf("jsonrpc: invalid ID string: %w", err)
		}
		*id = IDFromString(s)
		return nil
	}
	if data[0] == '-' || (data[0] >= '0' && data[0] <= '9') {
		var n int64
		if err := json.Unmarshal(data, &n); err != nil {
			return fmt.Errorf("jsonrpc: invalid ID number: %w", err)
		}
		*id = IDFromInt(n)
		return nil
	}
	return fmt.Errorf("jsonrpc: invalid ID value: %s", data)
}

// RPCError represents the error object defined in JSON-RPC 2.0 section 5.1.
type RPCError struct {
	// Code is a number indicating the error type. Codes in the range
	// -32768 to -32000 are reserved for pre-defined errors.
	Code int `json:"code"`

	// Message is a short, human-readable description of the error.
	Message string `json:"message"`

	// Data contains additional information about the error (optional).
	Data json.RawMessage `json:"data,omitempty"`
}

// Error implements the error interface.
func (e RPCError) Error() string {
	if len(e.Data) > 0 {
		return fmt.Sprintf("jsonrpc error %d: %s (data: %s)", e.Code, e.Message, e.Data)
	}
	return fmt.Sprintf("jsonrpc error %d: %s", e.Code, e.Message)
}

// Request represents a JSON-RPC 2.0 request message.
// Use NewRequest to ensure the jsonrpc field is set correctly.
type Request struct {
	// JSONRPC must always be "2.0".
	JSONRPC string `json:"jsonrpc"`

	// ID identifies the request. The server MUST reply with the same ID.
	ID ID `json:"id"`

	// Method is the name of the remote method to invoke.
	Method string `json:"method"`

	// Params holds the optional structured parameter value.
	Params json.RawMessage `json:"params,omitempty"`
}

// NewRequest creates a Request with jsonrpc set to "2.0".
func NewRequest(id ID, method string, params json.RawMessage) Request {
	return Request{
		JSONRPC: jsonRPCVersion,
		ID:      id,
		Method:  method,
		Params:  params,
	}
}

// Response represents a JSON-RPC 2.0 response message.
// Exactly one of Result or Error must be non-nil for a well-formed response.
// Use NewSuccessResponse or NewErrorResponse to construct well-formed responses
// that enforce this mutual exclusivity.
type Response struct {
	// JSONRPC must always be "2.0".
	JSONRPC string `json:"jsonrpc"`

	// ID echoes the ID from the corresponding Request.
	ID ID `json:"id"`

	// Result holds the result of a successful method invocation.
	Result json.RawMessage `json:"result,omitempty"`

	// Error holds the error object when the invocation failed.
	Error *RPCError `json:"error,omitempty"`
}

// NewSuccessResponse creates a Response with jsonrpc "2.0" and only the
// result field set. The error field is nil, enforcing mutual exclusivity
// per the JSON-RPC 2.0 spec.
//
// A nil result is normalised to the JSON literal null so that the result
// field is always present in the serialised output. The JSON-RPC 2.0 spec
// requires result to exist on every success response, even when the value
// is null.
func NewSuccessResponse(id ID, result json.RawMessage) Response {
	if result == nil {
		result = json.RawMessage("null")
	}
	return Response{
		JSONRPC: jsonRPCVersion,
		ID:      id,
		Result:  result,
	}
}

// NewErrorResponse creates a Response with jsonrpc "2.0" and only the
// error field set. The result field is nil, enforcing mutual exclusivity
// per the JSON-RPC 2.0 spec.
//
// NewErrorResponse returns an error if rpcErr is nil, because a well-formed
// JSON-RPC 2.0 error response must always carry a non-null error object.
func NewErrorResponse(id ID, rpcErr *RPCError) (Response, error) {
	if rpcErr == nil {
		return Response{}, fmt.Errorf("jsonrpc: NewErrorResponse: rpcErr must not be nil")
	}
	return Response{
		JSONRPC: jsonRPCVersion,
		ID:      id,
		Error:   rpcErr,
	}, nil
}

// Notification represents a JSON-RPC 2.0 notification message.
// Notifications have no ID and expect no response from the receiver.
// Use NewNotification to ensure the jsonrpc field is set correctly.
type Notification struct {
	// JSONRPC must always be "2.0".
	JSONRPC string `json:"jsonrpc"`

	// Method is the name of the notification.
	Method string `json:"method"`

	// Params holds the optional structured parameter value.
	Params json.RawMessage `json:"params,omitempty"`
}

// NewNotification creates a Notification with jsonrpc set to "2.0".
func NewNotification(method string, params json.RawMessage) Notification {
	return Notification{
		JSONRPC: jsonRPCVersion,
		Method:  method,
		Params:  params,
	}
}

// IDGenerator produces unique, monotonically increasing integer IDs.
// It is safe for concurrent use from multiple goroutines.
type IDGenerator struct {
	counter atomic.Int64
}

// Next returns the next unique ID. IDs start at 1 and are never reused
// within the lifetime of the generator.
func (g *IDGenerator) Next() ID {
	return IDFromInt(g.counter.Add(1))
}

// PendingRegistry tracks in-flight requests that are waiting for a response.
// It is safe for concurrent use from multiple goroutines.
type PendingRegistry struct {
	mu      sync.Mutex
	pending map[string]chan Response
}

// NewPendingRegistry creates an empty PendingRegistry.
func NewPendingRegistry() *PendingRegistry {
	return &PendingRegistry{
		pending: make(map[string]chan Response),
	}
}

// Register inserts id into the registry and returns a channel that will
// receive the matching Response exactly once. The channel is buffered so
// that Resolve never blocks even if the caller is slow to read.
// Callers MUST call Cancel if they abandon a request, to avoid a memory leak.
//
// Register returns an error if id is already registered. Duplicate
// registration is always a caller bug and is never silently overwritten.
func (r *PendingRegistry) Register(id ID) (<-chan Response, error) {
	key := id.Key()
	ch := make(chan Response, 1)

	r.mu.Lock()
	_, exists := r.pending[key]
	if !exists {
		r.pending[key] = ch
	}
	r.mu.Unlock()

	if exists {
		return nil, fmt.Errorf("jsonrpc: Register: ID %s is already pending", key)
	}
	return ch, nil
}

// Resolve delivers resp to the channel registered for resp.ID.
// The mutex is released before sending so that a slow consumer cannot
// block the registry. If no pending request matches resp.ID, a warning is
// logged and resp is discarded.
func (r *PendingRegistry) Resolve(resp Response) {
	key := resp.ID.Key()

	r.mu.Lock()
	ch, ok := r.pending[key]
	if ok {
		delete(r.pending, key)
	}
	r.mu.Unlock()

	if !ok {
		log.Printf("jsonrpc: Resolve: no pending request for ID %s - discarding", key)
		return
	}
	ch <- resp
}

// Cancel removes the entry for id from the registry and closes its channel,
// unblocking any goroutine waiting on the channel. If id is not registered,
// Cancel is a no-op.
func (r *PendingRegistry) Cancel(id ID) {
	key := id.Key()

	r.mu.Lock()
	ch, ok := r.pending[key]
	if ok {
		delete(r.pending, key)
	}
	r.mu.Unlock()

	if ok {
		close(ch)
	}
}

// NotificationDispatcher routes incoming Notification messages to registered
// handler functions. It is safe for concurrent use from multiple goroutines.
type NotificationDispatcher struct {
	mu       sync.RWMutex
	handlers map[string]func(Notification)
}

// NewNotificationDispatcher creates an empty NotificationDispatcher.
func NewNotificationDispatcher() *NotificationDispatcher {
	return &NotificationDispatcher{
		handlers: make(map[string]func(Notification)),
	}
}

// RegisterHandler subscribes fn to notifications whose Method equals method.
// Registering a second handler for the same method replaces the first.
func (d *NotificationDispatcher) RegisterHandler(method string, fn func(Notification)) {
	d.mu.Lock()
	d.handlers[method] = fn
	d.mu.Unlock()
}

// Dispatch calls the handler registered for n.Method. The handler is called
// outside the read lock so that handlers may themselves call RegisterHandler
// without deadlocking. If no handler is registered for n.Method, a warning
// is logged and the notification is discarded without error.
func (d *NotificationDispatcher) Dispatch(n Notification) {
	d.mu.RLock()
	fn, ok := d.handlers[n.Method]
	d.mu.RUnlock()

	if !ok {
		log.Printf("jsonrpc: Dispatch: no handler for method %q - discarding", n.Method)
		return
	}
	fn(n)
}
