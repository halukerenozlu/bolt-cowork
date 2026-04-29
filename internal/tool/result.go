package tool

// ResultStatus represents the outcome category of a tool call.
type ResultStatus int

const (
	StatusSuccess   ResultStatus = iota // Tool executed successfully.
	StatusDenied                        // Blocked by protection or sandbox rules.
	StatusCancelled                     // Cancelled by user or context.
	StatusFailed                        // Runtime error during execution.
)

// Result holds the outcome of a single tool invocation.
type Result struct {
	Status ResultStatus
	Output string // Human-readable output on success.
	Error  string // Error description on failure/denied.
	Path   string // Affected file path, if any.
}

// Ok reports whether the result represents a successful execution.
func (r Result) Ok() bool {
	return r.Status == StatusSuccess
}
