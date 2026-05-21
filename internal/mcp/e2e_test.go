package mcp_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/halukerenozlu/bolt-cowork/internal/mcp"
)

// fakeBin is the path to the compiled fakeserver binary.
// Set by TestMain; empty string means the binary could not be built and all
// e2e tests will be skipped.
var fakeBin string

// TestMain builds the fakeserver binary once before running all tests in the
// mcp_test package. If the build fails (e.g. no Go toolchain in PATH), e2e
// tests are skipped but all other tests in the package still run.
func TestMain(m *testing.M) {
	code := func() int {
		tmpDir, err := os.MkdirTemp("", "bolt-mcp-e2e-*")
		if err != nil {
			fmt.Fprintln(os.Stderr, "e2e: MkdirTemp:", err)
			return m.Run()
		}
		defer os.RemoveAll(tmpDir)

		binPath := filepath.Join(tmpDir, "fakeserver")
		if runtime.GOOS == "windows" {
			binPath += ".exe"
		}

		cwd, _ := os.Getwd() // test binary cwd == package source dir (internal/mcp/)
		cmd := exec.Command("go", "build", "-o", binPath, "./testutil/fakeserver")
		cmd.Dir = cwd
		if out, buildErr := cmd.CombinedOutput(); buildErr != nil {
			fmt.Fprintf(os.Stderr, "e2e: fakeserver build failed: %v\n%s\n", buildErr, out)
			// Non-fatal: e2e tests will skip themselves via the fakeBin guard.
		} else {
			fakeBin = binPath
		}

		return m.Run()
	}()
	os.Exit(code)
}

// e2eResource mirrors the fakeserver wire format for the FAKE_MCP_RESOURCES
// env var. A local type is used to keep JSON field names lowercase without
// depending on testutil.FakeResource (which has no JSON tags).
type e2eResource struct {
	URI      string `json:"uri"`
	Name     string `json:"name"`
	MimeType string `json:"mimeType,omitempty"`
	Content  string `json:"content,omitempty"`
}

// fakeServerOpts configures a fakeserver subprocess for a single test.
type fakeServerOpts struct {
	// Tools is the list of tools the server will advertise via tools/list.
	// Defaults to one "echo" tool if empty.
	Tools []mcp.Tool

	// Resources is the list of resources the server will advertise via
	// resources/list and serve via resources/read.
	Resources []e2eResource

	// DelayMS is the number of milliseconds the server sleeps before each
	// response. Set to a large value to trigger context timeout tests.
	DelayMS int

	// InvalidOn is the JSON-RPC method name for which the server will return
	// invalid JSON. Empty string disables this behaviour.
	InvalidOn string

	// NotifyAfter is the JSON-RPC method name after which the server sends a
	// notifications/resources/updated notification to the client.
	NotifyAfter string

	// RequireInitialized makes the server reject ordinary requests until it
	// receives notifications/initialized from the client.
	RequireInitialized bool
}

// startFakeServerProcess launches a fakeserver subprocess, wires its
// stdin/stdout to a StdioTransport, registers cleanup with t, and returns
// the transport. The caller is responsible for calling mcp.Client.Connect
// with the returned transport.
func startFakeServerProcess(t *testing.T, opts fakeServerOpts) mcp.Transport {
	t.Helper()

	toolsJSON := []byte(`[{"name":"echo","description":"Echo tool"}]`)
	if len(opts.Tools) > 0 {
		var err error
		toolsJSON, err = json.Marshal(opts.Tools)
		if err != nil {
			t.Fatalf("startFakeServerProcess: marshal tools: %v", err)
		}
	}

	resourcesJSON := []byte(`[]`)
	if len(opts.Resources) > 0 {
		var err error
		resourcesJSON, err = json.Marshal(opts.Resources)
		if err != nil {
			t.Fatalf("startFakeServerProcess: marshal resources: %v", err)
		}
	}

	env := append(os.Environ(),
		"FAKE_MCP_TOOLS="+string(toolsJSON),
		"FAKE_MCP_RESOURCES="+string(resourcesJSON),
		fmt.Sprintf("FAKE_MCP_DELAY_MS=%d", opts.DelayMS),
		"FAKE_MCP_INVALID_ON="+opts.InvalidOn,
		"FAKE_MCP_NOTIFY_AFTER="+opts.NotifyAfter,
	)
	if opts.RequireInitialized {
		env = append(env, "FAKE_MCP_REQUIRE_INITIALIZED=1")
	}

	cmd := exec.Command(fakeBin)
	cmd.Env = env
	cmd.Stderr = os.Stderr // surface fakeserver log output during test failures

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("startFakeServerProcess: StdinPipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("startFakeServerProcess: StdoutPipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("startFakeServerProcess: Start: %v", err)
	}

	transport := mcp.NewStdioTransport(stdout, stdin, stdout, stdin)

	t.Cleanup(func() {
		_ = transport.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	})

	return transport
}

// skipIfNoBinary skips the calling test if the fakeserver binary is unavailable.
func skipIfNoBinary(t *testing.T) {
	t.Helper()
	if fakeBin == "" {
		t.Skip("fakeserver binary unavailable; skipping e2e test")
	}
}

func waitUntil(t *testing.T, timeout time.Duration, check func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if check() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}

func TestE2E_InitializeHandshake(t *testing.T) {
	skipIfNoBinary(t)

	transport := startFakeServerProcess(t, fakeServerOpts{
		Tools:              []mcp.Tool{{Name: "echo"}},
		RequireInitialized: true,
	})

	c := mcp.NewClient()
	if _, err := c.ConnectAndInitialize(context.Background(), "srv", transport); err != nil {
		t.Fatalf("ConnectAndInitialize: %v", err)
	}
	t.Cleanup(func() { _ = c.Close() })

	if err := c.DiscoverTools(context.Background()); err != nil {
		t.Fatalf("DiscoverTools after Initialize: %v", err)
	}
}

func TestE2E_ResourceUpdatedNotification(t *testing.T) {
	skipIfNoBinary(t)

	transport := startFakeServerProcess(t, fakeServerOpts{
		Tools:       []mcp.Tool{{Name: "echo"}},
		NotifyAfter: "tools/call",
	})

	c := mcp.NewClient()
	c.Connect("srv", transport)
	t.Cleanup(func() { _ = c.Close() })

	if c.ResourcesStale() {
		t.Fatal("ResourcesStale = true before notification, want false")
	}
	if _, err := c.CallTool(context.Background(), "srv", "echo", nil); err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	waitUntil(t, time.Second, c.ResourcesStale)
}

// TestE2E_FullLifecycle verifies the complete MCP client lifecycle against a
// real fakeserver subprocess:
//
//	Connect → DiscoverTools → CallTool → Disconnect
func TestE2E_FullLifecycle(t *testing.T) {
	skipIfNoBinary(t)

	transport := startFakeServerProcess(t, fakeServerOpts{
		Tools: []mcp.Tool{
			{Name: "echo", Description: "Echo tool"},
		},
	})

	c := mcp.NewClient()
	c.Connect("srv", transport)
	t.Cleanup(func() { _ = c.Close() })

	ctx := context.Background()

	if err := c.DiscoverTools(ctx); err != nil {
		t.Fatalf("DiscoverTools: %v", err)
	}

	all := c.Tools().ListAll()
	tools, ok := all["srv"]
	if !ok || len(tools) == 0 {
		t.Fatalf("DiscoverTools: no tools for server %q; registry: %+v", "srv", all)
	}
	if tools[0].Name != "echo" {
		t.Errorf("tools[0].Name = %q, want %q", tools[0].Name, "echo")
	}

	result, err := c.CallTool(ctx, "srv", "echo", map[string]any{"msg": "hello"})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if len(result.Content) == 0 {
		t.Fatal("CallTool: empty content")
	}
	// The echo handler returns {"tool":"echo","args":{"msg":"hello"}}.
	if !strings.Contains(result.Content[0].Text, "echo") {
		t.Errorf("CallTool content = %q; want it to contain %q", result.Content[0].Text, "echo")
	}

	if err := c.Disconnect("srv"); err != nil {
		t.Fatalf("Disconnect: %v", err)
	}
}

// TestE2E_Timeout verifies that a context deadline propagates correctly when
// the fakeserver is configured to delay responses longer than the deadline.
func TestE2E_Timeout(t *testing.T) {
	skipIfNoBinary(t)

	// 2 s delay >> 100 ms deadline — the server will never respond in time.
	transport := startFakeServerProcess(t, fakeServerOpts{
		Tools:   []mcp.Tool{{Name: "echo"}},
		DelayMS: 2000,
	})

	c := mcp.NewClient()
	c.Connect("srv", transport)
	t.Cleanup(func() { _ = c.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := c.DiscoverTools(ctx)
	if err == nil {
		t.Fatal("DiscoverTools: expected timeout error, got nil")
	}
	// The error must be rooted in a context cancellation, not an unrelated failure.
	if !strings.Contains(err.Error(), "context") &&
		!strings.Contains(err.Error(), "deadline") &&
		!strings.Contains(err.Error(), "cancel") {
		t.Errorf("DiscoverTools error = %q; expected a context/deadline/cancel error", err)
	}
}

// TestE2E_InvalidJSONResponse verifies that malformed JSON from the server is
// handled gracefully: the client's read loop skips the bad message and the
// caller eventually receives a timeout error — no panic, no deadlock.
func TestE2E_InvalidJSONResponse(t *testing.T) {
	skipIfNoBinary(t)

	// tools/list responds normally so DiscoverTools succeeds.
	// tools/call returns invalid JSON, which the readLoop will skip.
	transport := startFakeServerProcess(t, fakeServerOpts{
		Tools:     []mcp.Tool{{Name: "echo"}},
		InvalidOn: "tools/call",
	})

	c := mcp.NewClient()
	c.Connect("srv", transport)
	t.Cleanup(func() { _ = c.Close() })

	if err := c.DiscoverTools(context.Background()); err != nil {
		t.Fatalf("DiscoverTools: %v", err)
	}

	// The server sends back garbage JSON for tools/call; the readLoop skips it.
	// The caller times out waiting for a response that never arrives.
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_, err := c.CallTool(ctx, "srv", "echo", nil)
	if err == nil {
		t.Fatal("CallTool: expected error from invalid JSON, got nil")
	}
	// Must not be a server-side error — the call itself must have failed.
	if _, ok := err.(*mcp.RPCError); ok {
		t.Errorf("CallTool: got an RPCError %v, want a transport/timeout error", err)
	}
}

// TestE2E_PermissionDenylist verifies that a tool denied by PermissionProfile
// is rejected by the client before any network I/O occurs.
func TestE2E_PermissionDenylist(t *testing.T) {
	skipIfNoBinary(t)

	transport := startFakeServerProcess(t, fakeServerOpts{
		Tools: []mcp.Tool{{Name: "echo"}},
	})

	c := mcp.NewClient()
	c.Connect("srv", transport)
	t.Cleanup(func() { _ = c.Close() })

	if err := c.DiscoverTools(context.Background()); err != nil {
		t.Fatalf("DiscoverTools: %v", err)
	}

	c.SetPermissions("srv", mcp.PermissionProfile{
		DeniedTools: []string{"echo"},
	})

	// The permission check fires immediately in CallTool, before any send.
	start := time.Now()
	_, err := c.CallTool(context.Background(), "srv", "echo", nil)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("CallTool: expected permission-denied error, got nil")
	}
	if !strings.Contains(err.Error(), "denied") {
		t.Errorf("CallTool error = %q; want it to mention %q", err, "denied")
	}
	// Rejection must be immediate — no round-trip to the server.
	if elapsed > 500*time.Millisecond {
		t.Errorf("CallTool took %v; expected near-instant rejection", elapsed)
	}
}

// TestE2E_MultiServer verifies that a single Client correctly routes tool
// calls to two concurrently connected fakeserver subprocesses.
func TestE2E_MultiServer(t *testing.T) {
	skipIfNoBinary(t)

	transportA := startFakeServerProcess(t, fakeServerOpts{
		Tools: []mcp.Tool{{Name: "tool-a", Description: "Server A tool"}},
	})
	transportB := startFakeServerProcess(t, fakeServerOpts{
		Tools: []mcp.Tool{{Name: "tool-b", Description: "Server B tool"}},
	})

	c := mcp.NewClient()
	c.Connect("srvA", transportA)
	c.Connect("srvB", transportB)
	t.Cleanup(func() { _ = c.Close() })

	ctx := context.Background()

	if err := c.DiscoverTools(ctx); err != nil {
		t.Fatalf("DiscoverTools: %v", err)
	}

	all := c.Tools().ListAll()
	if len(all["srvA"]) != 1 || all["srvA"][0].Name != "tool-a" {
		t.Errorf("srvA tools = %+v; want [{tool-a}]", all["srvA"])
	}
	if len(all["srvB"]) != 1 || all["srvB"][0].Name != "tool-b" {
		t.Errorf("srvB tools = %+v; want [{tool-b}]", all["srvB"])
	}

	// Call tool-a on srvA: response must echo "tool-a" back.
	rA, err := c.CallTool(ctx, "srvA", "tool-a", map[string]any{"msg": "from-a"})
	if err != nil {
		t.Fatalf("CallTool srvA/tool-a: %v", err)
	}
	if len(rA.Content) == 0 || !strings.Contains(rA.Content[0].Text, "tool-a") {
		t.Errorf("srvA response = %+v; want content containing %q", rA, "tool-a")
	}

	// Call tool-b on srvB: response must echo "tool-b" back.
	rB, err := c.CallTool(ctx, "srvB", "tool-b", map[string]any{"msg": "from-b"})
	if err != nil {
		t.Fatalf("CallTool srvB/tool-b: %v", err)
	}
	if len(rB.Content) == 0 || !strings.Contains(rB.Content[0].Text, "tool-b") {
		t.Errorf("srvB response = %+v; want content containing %q", rB, "tool-b")
	}

	// After disconnecting srvA, srvB must continue to serve requests.
	if err := c.Disconnect("srvA"); err != nil {
		t.Fatalf("Disconnect srvA: %v", err)
	}

	rB2, err := c.CallTool(ctx, "srvB", "tool-b", map[string]any{"msg": "still-b"})
	if err != nil {
		t.Fatalf("CallTool srvB after srvA disconnect: %v", err)
	}
	if len(rB2.Content) == 0 {
		t.Error("srvB after srvA disconnect: empty content")
	}
}

// TestE2E_ResourceLifecycle verifies the complete resource discovery and read
// flow against a real fakeserver subprocess:
//
//	Connect → DiscoverResources → ReadResource
func TestE2E_ResourceLifecycle(t *testing.T) {
	skipIfNoBinary(t)

	transport := startFakeServerProcess(t, fakeServerOpts{
		Tools: []mcp.Tool{{Name: "echo"}},
		Resources: []e2eResource{
			{URI: "file://test.txt", Name: "Test File", Content: "hello resource"},
		},
	})

	c := mcp.NewClient()
	c.Connect("srv", transport)
	t.Cleanup(func() { _ = c.Close() })

	ctx := context.Background()

	if err := c.DiscoverResources(ctx); err != nil {
		t.Fatalf("DiscoverResources: %v", err)
	}

	all := c.Resources().ListAll()
	resources, ok := all["srv"]
	if !ok || len(resources) == 0 {
		t.Fatalf("DiscoverResources: no resources for server %q; registry: %+v", "srv", all)
	}
	if resources[0].URI != "file://test.txt" {
		t.Errorf("resources[0].URI = %q, want %q", resources[0].URI, "file://test.txt")
	}

	contents, err := c.ReadResource(ctx, "srv", "file://test.txt")
	if err != nil {
		t.Fatalf("ReadResource: %v", err)
	}
	if contents.Text != "hello resource" {
		t.Errorf("ReadResource text = %q, want %q", contents.Text, "hello resource")
	}
}

// TestE2E_ResourceNotFound verifies that requesting a non-existent resource
// URI returns an error from the server.
func TestE2E_ResourceNotFound(t *testing.T) {
	skipIfNoBinary(t)

	transport := startFakeServerProcess(t, fakeServerOpts{
		Tools: []mcp.Tool{{Name: "echo"}},
		Resources: []e2eResource{
			{URI: "file://exists.txt", Name: "Exists", Content: "present"},
		},
	})

	c := mcp.NewClient()
	c.Connect("srv", transport)
	t.Cleanup(func() { _ = c.Close() })

	_, err := c.ReadResource(context.Background(), "srv", "file://does-not-exist.txt")
	if err == nil {
		t.Fatal("ReadResource: expected error for non-existent URI, got nil")
	}
}
