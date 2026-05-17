package mcp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
)

// StartProcess launches the executable at name with args, wires its stdin/stdout
// to a new StdioTransport, and starts the process. The process lifecycle is
// tied to ctx: when ctx is cancelled, the child process receives SIGKILL (on
// Unix) or is terminated (on Windows) via exec.CommandContext.
//
// The caller is responsible for calling t.Close() when done to release
// resources, and for reaping the process (cmd.Wait or equivalent).
//
// Returns the transport, a handle to the running process, and any error that
// occurred during startup.
func StartProcess(ctx context.Context, name string, args []string) (*StdioTransport, *os.Process, error) {
	cmd := exec.CommandContext(ctx, name, args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("mcp/process: StdinPipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, nil, fmt.Errorf("mcp/process: StdoutPipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, nil, fmt.Errorf("mcp/process: Start %q: %w", name, err)
	}

	// rc = stdout (unblock Receive on Close), wc = stdin (flush writes on Close).
	t := NewStdioTransport(stdout, stdin, stdout, stdin)
	return t, cmd.Process, nil
}
