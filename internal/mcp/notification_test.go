package mcp

import (
	"encoding/json"
	"sync"
	"testing"
)

func TestNotificationRegistry_HandleRegistered(t *testing.T) {
	registry := NewNotificationRegistry()
	got := make(chan string, 1)

	registry.OnNotification("notifications/test", func(method string, params json.RawMessage) {
		got <- method + ":" + string(params)
	})

	registry.HandleNotification("notifications/test", json.RawMessage(`{"ok":true}`))

	select {
	case value := <-got:
		want := `notifications/test:{"ok":true}`
		if value != want {
			t.Fatalf("handler value = %q, want %q", value, want)
		}
	default:
		t.Fatal("registered handler was not called")
	}
}

func TestNotificationRegistry_HandleUnregistered(t *testing.T) {
	registry := NewNotificationRegistry()

	// Must not panic when no handler or default handler is registered.
	registry.HandleNotification("notifications/missing", nil)

	got := make(chan string, 1)
	registry.SetDefaultHandler(func(method string, params json.RawMessage) {
		got <- method
	})
	registry.HandleNotification("notifications/missing", nil)

	select {
	case method := <-got:
		if method != "notifications/missing" {
			t.Fatalf("default handler method = %q, want notifications/missing", method)
		}
	default:
		t.Fatal("default handler was not called")
	}
}

func TestNotificationRegistry_HandlerPanicRecovery(t *testing.T) {
	registry := NewNotificationRegistry()

	registry.OnNotification("notifications/panic", func(method string, params json.RawMessage) {
		panic("boom")
	})

	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				t.Fatalf("HandleNotification propagated panic: %v", recovered)
			}
		}()
		registry.HandleNotification("notifications/panic", nil)
	}()

	got := make(chan string, 1)
	registry.OnNotification("notifications/next", func(method string, params json.RawMessage) {
		got <- method
	})
	registry.HandleNotification("notifications/next", nil)

	select {
	case method := <-got:
		if method != "notifications/next" {
			t.Fatalf("subsequent handler method = %q, want notifications/next", method)
		}
	default:
		t.Fatal("subsequent handler was not called")
	}
}

func TestNotificationRegistry_ConcurrentAccess(t *testing.T) {
	registry := NewNotificationRegistry()
	var mu sync.Mutex
	count := 0

	registry.OnNotification("notifications/tick", func(method string, params json.RawMessage) {
		mu.Lock()
		count++
		mu.Unlock()
	})

	const n = 100
	var wg sync.WaitGroup
	wg.Add(n * 2)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			registry.OnNotification("notifications/tick", func(method string, params json.RawMessage) {
				mu.Lock()
				count++
				mu.Unlock()
			})
		}()
		go func() {
			defer wg.Done()
			registry.HandleNotification("notifications/tick", nil)
		}()
	}
	wg.Wait()

	mu.Lock()
	got := count
	mu.Unlock()
	if got == 0 {
		t.Fatal("concurrent handlers were never called")
	}
}
