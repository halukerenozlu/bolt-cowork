package tool

import "context"

// Tool is the interface that all agent-callable tools implement.
type Tool interface {
	// Name returns the tool's unique identifier (e.g. "read", "write").
	Name() string

	// Description returns a short human-readable summary of the tool.
	Description() string

	// InputSchema returns a simplified JSON Schema describing the expected
	// keys and their types for the input map passed to Call.
	InputSchema() map[string]any

	// Call executes the tool with the given input parameters.
	Call(ctx context.Context, input map[string]any) (Result, error)

	// IsReadOnly reports whether the tool only reads data without modifying it.
	IsReadOnly() bool

	// IsDestructive reports whether the tool may permanently remove or
	// overwrite data (e.g. delete, move).
	IsDestructive() bool
}
