package mcp

import (
	"encoding/json"
	"log"
	"sync"
)

// NotificationHandler handles a JSON-RPC notification method and its raw params.
type NotificationHandler func(method string, params json.RawMessage)

// NotificationRegistry routes notifications to method-specific callbacks.
// It is safe for concurrent use from multiple goroutines.
type NotificationRegistry struct {
	mu              sync.RWMutex
	builtinHandlers map[string]NotificationHandler
	handlers        map[string]NotificationHandler
	defaultHandler  NotificationHandler
}

// NewNotificationRegistry creates an empty NotificationRegistry.
func NewNotificationRegistry() *NotificationRegistry {
	return &NotificationRegistry{
		builtinHandlers: make(map[string]NotificationHandler),
		handlers:        make(map[string]NotificationHandler),
	}
}

// OnBuiltinNotification registers an internal handler for method.
// Built-in handlers run before user handlers and cannot be overwritten by
// OnNotification.
func (r *NotificationRegistry) OnBuiltinNotification(method string, handler NotificationHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if handler == nil {
		delete(r.builtinHandlers, method)
		return
	}
	r.builtinHandlers[method] = handler
}

// OnNotification registers handler for method. Registering a nil handler
// removes any existing handler for method.
func (r *NotificationRegistry) OnNotification(method string, handler NotificationHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if handler == nil {
		delete(r.handlers, method)
		return
	}
	r.handlers[method] = handler
}

// SetDefaultHandler sets the optional fallback used for unregistered methods.
func (r *NotificationRegistry) SetDefaultHandler(handler NotificationHandler) {
	r.mu.Lock()
	r.defaultHandler = handler
	r.mu.Unlock()
}

// HandleNotification dispatches method and params to a registered handler.
// Handlers are called outside the lock so they may register other handlers.
func (r *NotificationRegistry) HandleNotification(method string, params json.RawMessage) {
	r.mu.RLock()
	builtinHandler := r.builtinHandlers[method]
	handler := r.handlers[method]
	defaultHandler := r.defaultHandler
	r.mu.RUnlock()

	if builtinHandler != nil {
		callNotificationHandler(method, params, builtinHandler)
	}
	if handler != nil {
		callNotificationHandler(method, params, handler)
		return
	}
	if builtinHandler == nil && defaultHandler != nil {
		callNotificationHandler(method, params, defaultHandler)
	}
}

func callNotificationHandler(method string, params json.RawMessage, handler NotificationHandler) {
	defer func() {
		if recovered := recover(); recovered != nil {
			log.Printf("mcp/notification: handler for %q panicked: %v", method, recovered)
		}
	}()
	handler(method, params)
}
