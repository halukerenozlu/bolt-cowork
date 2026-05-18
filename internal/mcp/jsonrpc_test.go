package mcp

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
)

// TestIDGeneration_Uniqueness verifies that IDGenerator never produces
// duplicate IDs, even under concurrent access.
func TestIDGeneration_Uniqueness(t *testing.T) {
	tests := []struct {
		name            string
		goroutines      int
		idsPerGoroutine int
	}{
		{"single goroutine 100 IDs", 1, 100},
		{"10 goroutines 100 IDs each", 10, 100},
		{"100 goroutines 10 IDs each", 100, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gen := &IDGenerator{}
			total := tt.goroutines * tt.idsPerGoroutine
			keys := make(chan string, total)

			var wg sync.WaitGroup
			wg.Add(tt.goroutines)
			for i := 0; i < tt.goroutines; i++ {
				go func() {
					defer wg.Done()
					for j := 0; j < tt.idsPerGoroutine; j++ {
						keys <- gen.Next().Key()
					}
				}()
			}
			wg.Wait()
			close(keys)

			seen := make(map[string]struct{}, total)
			for k := range keys {
				if _, dup := seen[k]; dup {
					t.Errorf("duplicate ID key %q", k)
				}
				seen[k] = struct{}{}
			}
			if len(seen) != total {
				t.Errorf("got %d unique IDs, want %d", len(seen), total)
			}
		})
	}
}

// TestIDGeneration_StartsAtOne verifies that the first generated ID is 1.
func TestIDGeneration_StartsAtOne(t *testing.T) {
	gen := &IDGenerator{}
	id := gen.Next()
	if id.Key() != "1" {
		t.Errorf("first ID Key() = %q, want %q", id.Key(), "1")
	}
}

// TestIDGeneration_Monotonic verifies that each successive ID is larger than
// the previous one (sequential integers).
func TestIDGeneration_Monotonic(t *testing.T) {
	gen := &IDGenerator{}
	prev := gen.Next()
	for i := 0; i < 9; i++ {
		next := gen.Next()
		if next.Key() <= prev.Key() {
			// String comparison works here only because IDs are small ints.
			// Use a numeric comparison instead.
			var prevN, nextN int64
			if err := json.Unmarshal([]byte(prev.Key()), &prevN); err != nil {
				t.Fatalf("unmarshal prev: %v", err)
			}
			if err := json.Unmarshal([]byte(next.Key()), &nextN); err != nil {
				t.Fatalf("unmarshal next: %v", err)
			}
			if nextN <= prevN {
				t.Errorf("ID %d is not greater than previous ID %d", nextN, prevN)
			}
		}
		prev = next
	}
}

// TestPendingRegistry_Lifecycle covers the full lifecycle of pending requests.
func TestPendingRegistry_Lifecycle(t *testing.T) {
	t.Run("register and resolve delivers response", func(t *testing.T) {
		reg := NewPendingRegistry()
		id := IDFromInt(1)
		ch, err := reg.Register(id)
		if err != nil {
			t.Fatalf("Register: %v", err)
		}

		want := Response{
			JSONRPC: "2.0",
			ID:      id,
			Result:  json.RawMessage(`"ok"`),
		}
		reg.Resolve(want)

		got, open := <-ch
		if !open {
			t.Fatal("channel was closed before response was received")
		}
		if got.ID.Key() != want.ID.Key() {
			t.Errorf("ID = %s, want %s", got.ID.Key(), want.ID.Key())
		}
		if string(got.Result) != string(want.Result) {
			t.Errorf("Result = %s, want %s", got.Result, want.Result)
		}
	})

	t.Run("resolved entry is removed from registry", func(t *testing.T) {
		reg := NewPendingRegistry()
		id := IDFromInt(10)
		ch, err := reg.Register(id)
		if err != nil {
			t.Fatalf("Register: %v", err)
		}
		reg.Resolve(Response{JSONRPC: "2.0", ID: id})
		<-ch // consume
		// Resolving again must not deliver a second value or panic.
		reg.Resolve(Response{JSONRPC: "2.0", ID: id})
	})

	t.Run("cancel closes channel", func(t *testing.T) {
		reg := NewPendingRegistry()
		id := IDFromInt(2)
		ch, err := reg.Register(id)
		if err != nil {
			t.Fatalf("Register: %v", err)
		}

		reg.Cancel(id)

		_, open := <-ch
		if open {
			t.Error("channel must be closed after Cancel")
		}
	})

	t.Run("cancel is idempotent for unknown ID", func(t *testing.T) {
		reg := NewPendingRegistry()
		// Must not panic.
		reg.Cancel(IDFromInt(999))
	})

	t.Run("register after CloseAll returns error", func(t *testing.T) {
		reg := NewPendingRegistry()
		reg.CloseAll(ErrConnectionClosed)

		ch, err := reg.Register(IDFromInt(4))
		if err == nil {
			t.Fatal("Register after CloseAll returned nil error")
		}
		if ch != nil {
			t.Fatal("Register after CloseAll returned a channel")
		}
		if !errors.Is(err, ErrConnectionClosed) {
			t.Errorf("Register error = %v, want to wrap ErrConnectionClosed", err)
		}
	})

	t.Run("resolve unknown ID does not panic", func(t *testing.T) {
		reg := NewPendingRegistry()
		// Must not panic; should only log a warning.
		reg.Resolve(Response{JSONRPC: "2.0", ID: IDFromInt(404)})
	})

	t.Run("resolve does not block when caller has not read channel yet", func(t *testing.T) {
		reg := NewPendingRegistry()
		id := IDFromInt(3)
		_, err := reg.Register(id) // caller does not read from this channel
		if err != nil {
			t.Fatalf("Register: %v", err)
		}

		done := make(chan struct{})
		go func() {
			// The channel is buffered(1); Resolve must return immediately.
			reg.Resolve(Response{JSONRPC: "2.0", ID: id})
			close(done)
		}()
		<-done // if Resolve blocks, this hangs and -timeout catches it
	})

	t.Run("concurrent register and resolve", func(t *testing.T) {
		reg := NewPendingRegistry()
		const n = 50
		var wg sync.WaitGroup
		wg.Add(n)
		for i := int64(1); i <= n; i++ {
			id := IDFromInt(i)
			ch, err := reg.Register(id)
			if err != nil {
				t.Fatalf("Register id %d: %v", i, err)
			}
			go func(id ID, ch <-chan Response) {
				defer wg.Done()
				reg.Resolve(Response{JSONRPC: "2.0", ID: id})
				<-ch
			}(id, ch)
		}
		wg.Wait()
	})

	t.Run("duplicate registration returns error", func(t *testing.T) {
		reg := NewPendingRegistry()
		id := IDFromInt(7)

		ch1, err := reg.Register(id)
		if err != nil {
			t.Fatalf("first Register: %v", err)
		}

		_, err = reg.Register(id)
		if err == nil {
			t.Fatal("second Register with same ID must return an error, got nil")
		}

		// The original channel must still be functional after the failed duplicate.
		reg.Cancel(id)
		_, open := <-ch1
		if open {
			t.Error("channel must be closed after Cancel")
		}
	})
}

// TestNotificationDispatcher covers registration and dispatch of notifications.
func TestNotificationDispatcher(t *testing.T) {
	t.Run("dispatch calls registered handler", func(t *testing.T) {
		d := NewNotificationDispatcher()
		received := make(chan Notification, 1)
		d.RegisterHandler("test/event", func(n Notification) {
			received <- n
		})

		n := NewNotification("test/event", json.RawMessage(`{"key":"value"}`))
		d.Dispatch(n)

		got := <-received
		if got.Method != "test/event" {
			t.Errorf("Method = %q, want %q", got.Method, "test/event")
		}
		if string(got.Params) != `{"key":"value"}` {
			t.Errorf("Params = %s, want %s", got.Params, `{"key":"value"}`)
		}
	})

	t.Run("dispatch to unknown method does not panic", func(t *testing.T) {
		d := NewNotificationDispatcher()
		// Must not panic; should only log a warning.
		d.Dispatch(NewNotification("no/handler", nil))
	})

	t.Run("second RegisterHandler replaces first", func(t *testing.T) {
		d := NewNotificationDispatcher()
		calls := 0
		d.RegisterHandler("m", func(n Notification) { calls++ })
		d.RegisterHandler("m", func(n Notification) { calls += 10 })
		d.Dispatch(NewNotification("m", nil))
		if calls != 10 {
			t.Errorf("calls = %d, want 10 (second handler should replace first)", calls)
		}
	})

	t.Run("handlers for different methods are independent", func(t *testing.T) {
		d := NewNotificationDispatcher()
		var aGot, bGot bool
		d.RegisterHandler("a", func(n Notification) { aGot = true })
		d.RegisterHandler("b", func(n Notification) { bGot = true })

		d.Dispatch(NewNotification("a", nil))
		if !aGot {
			t.Error("handler 'a' was not called")
		}
		if bGot {
			t.Error("handler 'b' must not be called for method 'a'")
		}
	})

	t.Run("concurrent dispatch is safe", func(t *testing.T) {
		d := NewNotificationDispatcher()
		var mu sync.Mutex
		count := 0
		d.RegisterHandler("tick", func(n Notification) {
			mu.Lock()
			count++
			mu.Unlock()
		})

		const n = 100
		var wg sync.WaitGroup
		wg.Add(n)
		for i := 0; i < n; i++ {
			go func() {
				defer wg.Done()
				d.Dispatch(NewNotification("tick", nil))
			}()
		}
		wg.Wait()

		mu.Lock()
		got := count
		mu.Unlock()
		if got != n {
			t.Errorf("handler called %d times, want %d", got, n)
		}
	})
}

// TestRPCError_Serialization verifies JSON round-trips for RPCError.
func TestRPCError_Serialization(t *testing.T) {
	tests := []struct {
		name string
		err  RPCError
	}{
		{
			name: "method not found without data",
			err:  RPCError{Code: CodeMethodNotFound, Message: "method not found"},
		},
		{
			name: "invalid params with data",
			err:  RPCError{Code: CodeInvalidParams, Message: "invalid params", Data: json.RawMessage(`{"param":"x"}`)},
		},
		{
			name: "internal error",
			err:  RPCError{Code: CodeInternalError, Message: "internal error"},
		},
		{
			name: "parse error",
			err:  RPCError{Code: CodeParseError, Message: "parse error"},
		},
		{
			name: "invalid request",
			err:  RPCError{Code: CodeInvalidRequest, Message: "invalid request"},
		},
		{
			name: "custom server error with array data",
			err:  RPCError{Code: -32001, Message: "custom", Data: json.RawMessage(`[1,2,3]`)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := json.Marshal(tt.err)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}

			var got RPCError
			if err := json.Unmarshal(b, &got); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}

			if got.Code != tt.err.Code {
				t.Errorf("Code = %d, want %d", got.Code, tt.err.Code)
			}
			if got.Message != tt.err.Message {
				t.Errorf("Message = %q, want %q", got.Message, tt.err.Message)
			}
			if string(got.Data) != string(tt.err.Data) {
				t.Errorf("Data = %s, want %s", got.Data, tt.err.Data)
			}
		})
	}
}

// TestRPCError_ErrorInterface verifies that RPCError implements the error interface.
func TestRPCError_ErrorInterface(t *testing.T) {
	tests := []struct {
		name     string
		err      RPCError
		wantSubs []string
	}{
		{
			name:     "without data",
			err:      RPCError{Code: -32601, Message: "method not found"},
			wantSubs: []string{"-32601", "method not found"},
		},
		{
			name:     "with data",
			err:      RPCError{Code: -32602, Message: "bad params", Data: json.RawMessage(`"extra"`)},
			wantSubs: []string{"-32602", "bad params", "extra"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := tt.err.Error()
			for _, sub := range tt.wantSubs {
				found := false
				for i := 0; i+len(sub) <= len(msg); i++ {
					if msg[i:i+len(sub)] == sub {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Error() = %q, want it to contain %q", msg, sub)
				}
			}
		})
	}
}

// TestID_JSONRoundTrip verifies that ID correctly serialises and deserialises
// for integer, string, and null values.
func TestID_JSONRoundTrip(t *testing.T) {
	tests := []struct {
		name     string
		id       ID
		wantKey  string
		wantJSON string
		wantNull bool
	}{
		{
			name:     "integer",
			id:       IDFromInt(42),
			wantKey:  "42",
			wantJSON: "42",
		},
		{
			name:     "negative integer",
			id:       IDFromInt(-1),
			wantKey:  "-1",
			wantJSON: "-1",
		},
		{
			name:     "zero",
			id:       IDFromInt(0),
			wantKey:  "0",
			wantJSON: "0",
		},
		{
			name:     "string",
			id:       IDFromString("req-abc"),
			wantKey:  `"req-abc"`,
			wantJSON: `"req-abc"`,
		},
		{
			name:     "empty string",
			id:       IDFromString(""),
			wantKey:  `""`,
			wantJSON: `""`,
		},
		{
			name:     "explicit null",
			id:       IDNull(),
			wantKey:  "null",
			wantJSON: "null",
			wantNull: true,
		},
		{
			name:     "zero value is null",
			id:       ID{},
			wantKey:  "null",
			wantJSON: "null",
			wantNull: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.id.Key() != tt.wantKey {
				t.Errorf("Key() = %q, want %q", tt.id.Key(), tt.wantKey)
			}
			if tt.id.IsNull() != tt.wantNull {
				t.Errorf("IsNull() = %v, want %v", tt.id.IsNull(), tt.wantNull)
			}

			b, err := json.Marshal(tt.id)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if string(b) != tt.wantJSON {
				t.Errorf("marshalled JSON = %q, want %q", b, tt.wantJSON)
			}

			var got ID
			if err := json.Unmarshal(b, &got); err != nil {
				t.Fatalf("Unmarshal: %v", err)
			}
			if got.Key() != tt.wantKey {
				t.Errorf("round-trip Key() = %q, want %q", got.Key(), tt.wantKey)
			}
		})
	}
}

// TestID_UnicodeEquivalence verifies that \u-escaped string IDs produce the
// same Key() as their directly-encoded equivalents.
func TestID_UnicodeEquivalence(t *testing.T) {
	tests := []struct {
		name    string
		escaped string // raw JSON bytes using \u escapes
		plain   ID     // ID created directly from the decoded string
	}{
		{
			name:    `\u0041 is A`,
			escaped: `"\u0041"`,
			plain:   IDFromString("A"),
		},
		{
			name:    `\u-encoded hello`,
			escaped: `"\u0068\u0065\u006c\u006c\u006f"`,
			plain:   IDFromString("hello"),
		},
		{
			name:    `mixed escaped prefix`,
			escaped: `"\u0072eq-1"`,
			plain:   IDFromString("req-1"),
		},
		{
			name:    `upper-case hex digits`,
			escaped: `"\u004B"`,
			plain:   IDFromString("K"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got ID
			if err := json.Unmarshal([]byte(tt.escaped), &got); err != nil {
				t.Fatalf("Unmarshal(%s): %v", tt.escaped, err)
			}
			if got.Key() != tt.plain.Key() {
				t.Errorf("escaped Key() = %q, plain Key() = %q, want equal",
					got.Key(), tt.plain.Key())
			}
		})
	}
}

// TestID_UnmarshalInvalid verifies that UnmarshalJSON rejects unsupported types.
func TestID_UnmarshalInvalid(t *testing.T) {
	inputs := []string{
		`true`,
		`false`,
		`[]`,
		`{}`,
	}
	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			var id ID
			if err := json.Unmarshal([]byte(input), &id); err == nil {
				t.Errorf("expected error for input %q, got nil", input)
			}
		})
	}
}

// TestRequest_JSONVersion verifies that Request serialises with jsonrpc "2.0"
// and that nil params are omitted from the output.
func TestRequest_JSONVersion(t *testing.T) {
	req := NewRequest(IDFromInt(1), "tools/list", nil)

	b, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("Unmarshal to map: %v", err)
	}

	if string(m["jsonrpc"]) != `"2.0"` {
		t.Errorf("jsonrpc = %s, want %q", m["jsonrpc"], "2.0")
	}
	if string(m["method"]) != `"tools/list"` {
		t.Errorf("method = %s, want %q", m["method"], "tools/list")
	}
	if _, ok := m["params"]; ok {
		t.Error("params should be omitted when nil")
	}
}

// TestRequest_WithParams verifies params are included when non-nil.
func TestRequest_WithParams(t *testing.T) {
	params := json.RawMessage(`{"name":"foo"}`)
	req := NewRequest(IDFromString("r1"), "tools/call", params)

	b, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("Unmarshal to map: %v", err)
	}

	if string(m["params"]) != `{"name":"foo"}` {
		t.Errorf("params = %s, want %s", m["params"], `{"name":"foo"}`)
	}
}

// TestResponse_JSONVersion verifies that Response serialises with jsonrpc "2.0"
// and that the error field is omitted on a successful response.
func TestResponse_JSONVersion(t *testing.T) {
	resp := Response{
		JSONRPC: "2.0",
		ID:      IDFromInt(1),
		Result:  json.RawMessage(`{"tools":[]}`),
	}

	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("Unmarshal to map: %v", err)
	}

	if string(m["jsonrpc"]) != `"2.0"` {
		t.Errorf("jsonrpc = %s, want %q", m["jsonrpc"], "2.0")
	}
	if _, ok := m["error"]; ok {
		t.Error("error field must be omitted when nil")
	}
	if string(m["result"]) != `{"tools":[]}` {
		t.Errorf("result = %s, want %s", m["result"], `{"tools":[]}`)
	}
}

// TestResponse_ErrorOmitsResult verifies that on an error response the result
// field is omitted.
func TestResponse_ErrorOmitsResult(t *testing.T) {
	resp := Response{
		JSONRPC: "2.0",
		ID:      IDFromInt(2),
		Error:   &RPCError{Code: CodeMethodNotFound, Message: "not found"},
	}

	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("Unmarshal to map: %v", err)
	}

	if _, ok := m["result"]; ok {
		t.Error("result field must be omitted when nil")
	}
	if _, ok := m["error"]; !ok {
		t.Error("error field must be present")
	}
}

// TestNewSuccessResponse verifies that the constructor sets jsonrpc and result
// and leaves error nil, enforcing mutual exclusivity.
func TestNewSuccessResponse(t *testing.T) {
	id := IDFromInt(1)
	result := json.RawMessage(`{"value":42}`)
	resp := NewSuccessResponse(id, result)

	if resp.JSONRPC != "2.0" {
		t.Errorf("JSONRPC = %q, want %q", resp.JSONRPC, "2.0")
	}
	if resp.Error != nil {
		t.Errorf("Error must be nil in a success response, got %v", resp.Error)
	}
	if string(resp.Result) != string(result) {
		t.Errorf("Result = %s, want %s", resp.Result, result)
	}

	// Verify the error field is absent from the serialised form.
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if _, ok := m["error"]; ok {
		t.Error("error field must be absent in a success response")
	}
	if string(m["result"]) != `{"value":42}` {
		t.Errorf("serialised result = %s, want %s", m["result"], `{"value":42}`)
	}
}

// TestNewErrorResponse verifies that the constructor sets jsonrpc and error
// and leaves result nil, enforcing mutual exclusivity.
func TestNewErrorResponse(t *testing.T) {
	id := IDFromInt(2)
	rpcErr := &RPCError{Code: CodeMethodNotFound, Message: "not found"}
	resp, err := NewErrorResponse(id, rpcErr)
	if err != nil {
		t.Fatalf("NewErrorResponse: %v", err)
	}

	if resp.JSONRPC != "2.0" {
		t.Errorf("JSONRPC = %q, want %q", resp.JSONRPC, "2.0")
	}
	if resp.Result != nil {
		t.Errorf("Result must be nil in an error response, got %s", resp.Result)
	}
	if resp.Error == nil {
		t.Fatal("Error must not be nil in an error response")
	}
	if resp.Error.Code != CodeMethodNotFound {
		t.Errorf("Error.Code = %d, want %d", resp.Error.Code, CodeMethodNotFound)
	}

	// Verify the result field is absent from the serialised form.
	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if _, ok := m["result"]; ok {
		t.Error("result field must be absent in an error response")
	}
	if _, ok := m["error"]; !ok {
		t.Error("error field must be present in an error response")
	}
}

// TestNewSuccessResponse_NilResultIsNull verifies that a nil result is
// serialised as {"result":null,...} rather than having the field omitted,
// which would violate the JSON-RPC 2.0 spec requirement that result always
// be present on a success response.
func TestNewSuccessResponse_NilResultIsNull(t *testing.T) {
	resp := NewSuccessResponse(IDFromInt(3), nil)

	b, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	raw, ok := m["result"]
	if !ok {
		t.Fatal("result field must be present even when value is null")
	}
	if string(raw) != "null" {
		t.Errorf("result = %s, want null", raw)
	}
	if _, ok := m["error"]; ok {
		t.Error("error field must be absent in a success response")
	}
}

// TestNewErrorResponse_NilRPCError verifies that passing a nil *RPCError
// returns an error instead of silently producing a zero-value response.
func TestNewErrorResponse_NilRPCError(t *testing.T) {
	_, err := NewErrorResponse(IDFromInt(4), nil)
	if err == nil {
		t.Fatal("NewErrorResponse(id, nil) must return an error, got nil")
	}
}

// TestNotification_NoID verifies that Notification does not produce an id
// field in its JSON representation.
func TestNotification_NoID(t *testing.T) {
	n := NewNotification("$/progress", json.RawMessage(`{"token":1,"value":50}`))

	b, err := json.Marshal(n)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("Unmarshal to map: %v", err)
	}

	if _, ok := m["id"]; ok {
		t.Error("Notification must not contain an id field")
	}
	if string(m["jsonrpc"]) != `"2.0"` {
		t.Errorf("jsonrpc = %s, want %q", m["jsonrpc"], "2.0")
	}
	if string(m["method"]) != `"$/progress"` {
		t.Errorf("method = %s, want %q", m["method"], "$/progress")
	}
}

// TestNotification_NilParamsOmitted verifies that nil params are omitted from
// a Notification.
func TestNotification_NilParamsOmitted(t *testing.T) {
	n := NewNotification("ping", nil)
	b, err := json.Marshal(n)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("Unmarshal to map: %v", err)
	}

	if _, ok := m["params"]; ok {
		t.Error("params must be omitted when nil")
	}
}
