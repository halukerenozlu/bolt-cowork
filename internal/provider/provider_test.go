package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/halukerenozlu/bolt-cowork/pkg/types"
)

// mockProvider implements LLMProvider for testing.
type mockProvider struct {
	name      string
	available bool
	response  string
	err       error
}

func (m *mockProvider) Chat(_ context.Context, _ []types.Message, _ []ToolSpec) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	return m.response, nil
}

func (m *mockProvider) StreamChat(_ context.Context, _ []types.Message) (<-chan string, error) {
	if m.err != nil {
		return nil, m.err
	}
	ch := make(chan string, 1)
	go func() {
		defer close(ch)
		ch <- m.response
	}()
	return ch, nil
}

func (m *mockProvider) Name() string    { return m.name }
func (m *mockProvider) Available() bool { return m.available }

// --- FallbackChain Tests ---

func TestFallbackChain_FirstAvailable(t *testing.T) {
	chain := NewFallbackChain([]LLMProvider{
		&mockProvider{name: "p1", available: true, response: "from-p1"},
		&mockProvider{name: "p2", available: true, response: "from-p2"},
	})

	resp, err := chain.Chat(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp != "from-p1" {
		t.Errorf("response = %q, want %q", resp, "from-p1")
	}
}

func TestFallbackChain_SkipsUnavailable(t *testing.T) {
	chain := NewFallbackChain([]LLMProvider{
		&mockProvider{name: "p1", available: false},
		&mockProvider{name: "p2", available: true, response: "from-p2"},
	})

	resp, err := chain.Chat(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp != "from-p2" {
		t.Errorf("response = %q, want %q", resp, "from-p2")
	}
}

func TestFallbackChain_AllUnavailable(t *testing.T) {
	chain := NewFallbackChain([]LLMProvider{
		&mockProvider{name: "p1", available: false},
		&mockProvider{name: "p2", available: false},
	})

	_, err := chain.Chat(context.Background(), nil, nil)
	if err == nil {
		t.Fatal("expected error when all providers unavailable")
	}
	if !errors.Is(err, ErrNoAvailableProvider) {
		t.Errorf("expected ErrNoAvailableProvider, got: %v", err)
	}
}

func TestFallbackChain_FallbackOnNotAvailable(t *testing.T) {
	chain := NewFallbackChain([]LLMProvider{
		&mockProvider{name: "p1", available: true, err: fmt.Errorf("rate limited: %w", ErrNotAvailable)},
		&mockProvider{name: "p2", available: true, response: "from-p2"},
	})

	resp, err := chain.Chat(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp != "from-p2" {
		t.Errorf("response = %q, want %q", resp, "from-p2")
	}
}

func TestFallbackChain_NonRetryableError(t *testing.T) {
	chain := NewFallbackChain([]LLMProvider{
		&mockProvider{name: "p1", available: true, err: fmt.Errorf("invalid request")},
		&mockProvider{name: "p2", available: true, response: "from-p2"},
	})

	_, err := chain.Chat(context.Background(), nil, nil)
	if err == nil {
		t.Fatal("expected error for non-retryable failure")
	}
	// Should NOT fall back — error returned directly from p1.
	if errors.Is(err, ErrNoAvailableProvider) {
		t.Error("should not be ErrNoAvailableProvider — should return p1's error directly")
	}
}

func TestFallbackChain_OnFallbackCalled(t *testing.T) {
	var transitions []string
	chain := NewFallbackChain(
		[]LLMProvider{
			&mockProvider{name: "p1", available: false},
			&mockProvider{name: "p2", available: true, response: "ok"},
		},
		WithOnFallback(func(from, to LLMProvider) {
			transitions = append(transitions, from.Name()+"->"+to.Name())
		}),
	)

	_, err := chain.Chat(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(transitions) != 1 {
		t.Fatalf("transitions count = %d, want 1", len(transitions))
	}
	if transitions[0] != "p1->p2" {
		t.Errorf("transition = %q, want %q", transitions[0], "p1->p2")
	}
}

func TestFallbackChain_SingleProvider(t *testing.T) {
	chain := NewFallbackChain([]LLMProvider{
		&mockProvider{name: "solo", available: true, response: "solo-response"},
	})

	resp, err := chain.Chat(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp != "solo-response" {
		t.Errorf("response = %q, want %q", resp, "solo-response")
	}
}

func TestFallbackChain_Empty(t *testing.T) {
	chain := NewFallbackChain(nil)

	_, err := chain.Chat(context.Background(), nil, nil)
	if err == nil {
		t.Fatal("expected error for empty chain")
	}
	if !errors.Is(err, ErrNoAvailableProvider) {
		t.Errorf("expected ErrNoAvailableProvider, got: %v", err)
	}
}

func TestFallbackChain_StreamChat(t *testing.T) {
	chain := NewFallbackChain([]LLMProvider{
		&mockProvider{name: "p1", available: false},
		&mockProvider{name: "p2", available: true, response: "streamed"},
	})

	ch, err := chain.StreamChat(context.Background(), nil)
	if err != nil {
		t.Fatalf("StreamChat: %v", err)
	}

	var chunks []string
	for chunk := range ch {
		chunks = append(chunks, chunk)
	}
	if len(chunks) != 1 || chunks[0] != "streamed" {
		t.Errorf("chunks = %v, want [streamed]", chunks)
	}
}

// --- Provider Stub Tests ---

func TestOpenAI_Name(t *testing.T) {
	p := NewOpenAI("sk-test", "gpt-4o")
	if p.Name() != "openai/gpt-4o" {
		t.Errorf("Name() = %q, want %q", p.Name(), "openai/gpt-4o")
	}
}

func TestOpenAI_Available(t *testing.T) {
	tests := []struct {
		name   string
		apiKey string
		want   bool
	}{
		{"with key", "sk-test", true},
		{"empty key", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewOpenAI(tt.apiKey, "gpt-4o")
			if p.Available() != tt.want {
				t.Errorf("Available() = %v, want %v", p.Available(), tt.want)
			}
		})
	}
}

func TestOpenAI_SetAvailable(t *testing.T) {
	p := NewOpenAI("sk-test", "gpt-4o")
	p.SetAvailable(false)
	if p.Available() {
		t.Error("Available() should be false after SetAvailable(false)")
	}
}

func TestAnthropic_Name(t *testing.T) {
	p := NewAnthropic("sk-ant-test", "claude-opus-4-6")
	if p.Name() != "anthropic/claude-opus-4-6" {
		t.Errorf("Name() = %q, want %q", p.Name(), "anthropic/claude-opus-4-6")
	}
}

func TestAnthropic_Available(t *testing.T) {
	tests := []struct {
		name   string
		apiKey string
		want   bool
	}{
		{"with key", "sk-ant-test", true},
		{"empty key", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewAnthropic(tt.apiKey, "claude-opus-4-6")
			if p.Available() != tt.want {
				t.Errorf("Available() = %v, want %v", p.Available(), tt.want)
			}
		})
	}
}

func TestOpenAI_Chat_Unavailable(t *testing.T) {
	p := NewOpenAI("", "gpt-4o")
	_, err := p.Chat(context.Background(), nil, nil)
	if err == nil {
		t.Fatal("expected error for unavailable provider")
	}
	if !errors.Is(err, ErrNotAvailable) {
		t.Errorf("expected ErrNotAvailable, got: %v", err)
	}
}

func TestAnthropic_Chat_Unavailable(t *testing.T) {
	p := NewAnthropic("", "claude-sonnet-4-6")
	_, err := p.Chat(context.Background(), nil, nil)
	if err == nil {
		t.Fatal("expected error for unavailable provider")
	}
	if !errors.Is(err, ErrNotAvailable) {
		t.Errorf("expected ErrNotAvailable, got: %v", err)
	}
}

func TestAnthropic_Chat_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify headers.
		if got := r.Header.Get("x-api-key"); got != "sk-ant-test" {
			t.Errorf("x-api-key = %q, want %q", got, "sk-ant-test")
		}
		if got := r.Header.Get("anthropic-version"); got != "2023-06-01" {
			t.Errorf("anthropic-version = %q, want %q", got, "2023-06-01")
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want %q", got, "application/json")
		}

		// Verify request body.
		var reqBody anthropicRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if reqBody.Model != "claude-sonnet-4-6" {
			t.Errorf("model = %q, want %q", reqBody.Model, "claude-sonnet-4-6")
		}
		if reqBody.System != "You are helpful." {
			t.Errorf("system = %q, want %q", reqBody.System, "You are helpful.")
		}
		if len(reqBody.Messages) != 1 || reqBody.Messages[0].Role != "user" {
			t.Errorf("messages = %+v, want 1 user message", reqBody.Messages)
		}
		if reqBody.MaxTokens != anthropicMaxTok {
			t.Errorf("max_tokens = %d, want %d", reqBody.MaxTokens, anthropicMaxTok)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(anthropicResponse{
			Content: []anthropicContent{
				{Type: "text", Text: "Hello from Anthropic!"},
			},
			Type: "message",
		})
	}))
	defer srv.Close()

	p := NewAnthropic("sk-ant-test", "claude-sonnet-4-6")
	p.endpoint = srv.URL

	resp, err := p.Chat(context.Background(), []types.Message{
		{Role: types.RoleSystem, Content: "You are helpful."},
		{Role: types.RoleUser, Content: "Hi"},
	}, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp != "Hello from Anthropic!" {
		t.Errorf("response = %q, want %q", resp, "Hello from Anthropic!")
	}
}

func TestAnthropic_Chat_SystemConcat(t *testing.T) {
	var captured anthropicRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&captured)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(anthropicResponse{
			Content: []anthropicContent{{Type: "text", Text: "ok"}},
		})
	}))
	defer srv.Close()

	p := NewAnthropic("sk-ant-test", "claude-sonnet-4-6")
	p.endpoint = srv.URL

	_, err := p.Chat(context.Background(), []types.Message{
		{Role: types.RoleSystem, Content: "First system."},
		{Role: types.RoleSystem, Content: "Second system."},
		{Role: types.RoleUser, Content: "Hi"},
	}, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	want := "First system.\n\nSecond system."
	if captured.System != want {
		t.Errorf("system = %q, want %q", captured.System, want)
	}
	if len(captured.Messages) != 1 {
		t.Errorf("messages count = %d, want 1 (system excluded)", len(captured.Messages))
	}
}

func TestAnthropic_Chat_MultipleTextBlocks(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(anthropicResponse{
			Content: []anthropicContent{
				{Type: "text", Text: "Part 1. "},
				{Type: "text", Text: "Part 2."},
			},
		})
	}))
	defer srv.Close()

	p := NewAnthropic("sk-ant-test", "claude-sonnet-4-6")
	p.endpoint = srv.URL

	resp, err := p.Chat(context.Background(), []types.Message{
		{Role: types.RoleUser, Content: "Hi"},
	}, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp != "Part 1. Part 2." {
		t.Errorf("response = %q, want %q", resp, "Part 1. Part 2.")
	}
}

func TestAnthropic_Chat_EmptyContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(anthropicResponse{Content: nil})
	}))
	defer srv.Close()

	p := NewAnthropic("sk-ant-test", "claude-sonnet-4-6")
	p.endpoint = srv.URL

	resp, err := p.Chat(context.Background(), []types.Message{
		{Role: types.RoleUser, Content: "Hi"},
	}, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp != "" {
		t.Errorf("response = %q, want empty", resp)
	}
}

func TestAnthropic_Chat_4xx(t *testing.T) {
	srv := newTestServer(t, 401, `{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`)
	defer srv.Close()

	p := NewAnthropic("bad-key", "claude-sonnet-4-6")
	p.endpoint = srv.URL

	_, err := p.Chat(context.Background(), []types.Message{
		{Role: types.RoleUser, Content: "Hi"},
	}, nil)
	if err == nil {
		t.Fatal("expected error for 401")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 401 {
		t.Errorf("StatusCode = %d, want 401", apiErr.StatusCode)
	}
	// 401 is now fallback-eligible.
	if !errors.Is(err, ErrNotAvailable) {
		t.Error("401 should trigger fallback via ErrNotAvailable")
	}
}

func TestAnthropic_Chat_429_Fallback(t *testing.T) {
	srv := newTestServer(t, 429, `{"type":"error","error":{"type":"rate_limit_error","message":"rate limited"}}`)
	defer srv.Close()

	p := NewAnthropic("sk-ant-test", "claude-sonnet-4-6")
	p.endpoint = srv.URL

	_, err := p.Chat(context.Background(), []types.Message{
		{Role: types.RoleUser, Content: "Hi"},
	}, nil)
	if err == nil {
		t.Fatal("expected error for 429")
	}
	if !errors.Is(err, ErrNotAvailable) {
		t.Error("429 should wrap ErrNotAvailable for fallback")
	}
}

func TestAnthropic_Chat_5xx_Fallback(t *testing.T) {
	srv := newTestServer(t, 529, `{"type":"error","error":{"type":"overloaded_error","message":"overloaded"}}`)
	defer srv.Close()

	p := NewAnthropic("sk-ant-test", "claude-sonnet-4-6")
	p.endpoint = srv.URL

	_, err := p.Chat(context.Background(), []types.Message{
		{Role: types.RoleUser, Content: "Hi"},
	}, nil)
	if err == nil {
		t.Fatal("expected error for 529")
	}
	if !errors.Is(err, ErrNotAvailable) {
		t.Error("529 should wrap ErrNotAvailable for fallback")
	}
}

// --- APIError Tests ---

func TestAPIError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  APIError
		want string
	}{
		{
			"with body",
			APIError{StatusCode: 401, Status: "401 Unauthorized", Body: `{"error":"invalid key"}`, Provider: "openai"},
			`openai: HTTP 401 401 Unauthorized: {"error":"invalid key"}`,
		},
		{
			"without body",
			APIError{StatusCode: 500, Status: "500 Internal Server Error", Provider: "custom"},
			"custom: HTTP 500 500 Internal Server Error",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAPIError_Retryable(t *testing.T) {
	tests := []struct {
		code int
		want bool
	}{
		{200, false},
		{400, false},
		{401, true},  // auth errors are fallback-eligible
		{403, true},  // auth errors are fallback-eligible
		{404, false},
		{422, false},
		{429, true},
		{500, true},
		{502, true},
		{503, true},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d", tt.code), func(t *testing.T) {
			e := &APIError{StatusCode: tt.code}
			if e.Retryable() != tt.want {
				t.Errorf("Retryable() for %d = %v, want %v", tt.code, e.Retryable(), tt.want)
			}
		})
	}
}

// --- CheckResponse Tests ---

func TestCheckResponse_Success(t *testing.T) {
	resp := &http.Response{
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader("")),
	}
	if err := CheckResponse("test", resp); err != nil {
		t.Errorf("expected nil for 200, got: %v", err)
	}
}

func TestCheckResponse_ClientError(t *testing.T) {
	resp := &http.Response{
		StatusCode: 401,
		Status:     "401 Unauthorized",
		Body:       io.NopCloser(strings.NewReader(`{"error":"bad key"}`)),
	}
	err := CheckResponse("openai", resp)
	if err == nil {
		t.Fatal("expected error for 401")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 401 {
		t.Errorf("StatusCode = %d, want 401", apiErr.StatusCode)
	}
	// 401 is now fallback-eligible — should wrap ErrNotAvailable.
	if !errors.Is(err, ErrNotAvailable) {
		t.Error("401 should wrap ErrNotAvailable for fallback")
	}
}

func TestCheckResponse_RateLimited(t *testing.T) {
	resp := &http.Response{
		StatusCode: 429,
		Status:     "429 Too Many Requests",
		Body:       io.NopCloser(strings.NewReader("rate limited")),
	}
	err := CheckResponse("openai", resp)
	if err == nil {
		t.Fatal("expected error for 429")
	}
	// 429 is retryable — should wrap ErrNotAvailable for fallback.
	if !errors.Is(err, ErrNotAvailable) {
		t.Error("429 should wrap ErrNotAvailable for fallback chain")
	}
}

func TestCheckResponse_ServerError(t *testing.T) {
	resp := &http.Response{
		StatusCode: 503,
		Status:     "503 Service Unavailable",
		Body:       io.NopCloser(strings.NewReader("overloaded")),
	}
	err := CheckResponse("anthropic", resp)
	if err == nil {
		t.Fatal("expected error for 503")
	}
	if !errors.Is(err, ErrNotAvailable) {
		t.Error("503 should wrap ErrNotAvailable for fallback chain")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T", err)
	}
	if apiErr.Body != "overloaded" {
		t.Errorf("Body = %q, want %q", apiErr.Body, "overloaded")
	}
}

func TestCheckResponse_BodyReadError_Retryable(t *testing.T) {
	for _, code := range []int{429, 500, 503} {
		t.Run(fmt.Sprintf("%d", code), func(t *testing.T) {
			resp := &http.Response{
				StatusCode: code,
				Status:     fmt.Sprintf("%d Error", code),
				Body:       io.NopCloser(&failingReader{}),
			}
			err := CheckResponse("test", resp)
			if err == nil {
				t.Fatalf("expected error for %d with body read failure", code)
			}
			if !strings.Contains(err.Error(), "read response body") {
				t.Errorf("expected 'read response body' in error, got: %v", err)
			}
			if !errors.Is(err, ErrNotAvailable) {
				t.Errorf("%d with body read error should wrap ErrNotAvailable for fallback", code)
			}
		})
	}
}

func TestCheckResponse_BodyReadError_NonRetryable(t *testing.T) {
	// Use 400 (Bad Request) which is NOT fallback-eligible.
	resp := &http.Response{
		StatusCode: 400,
		Status:     "400 Bad Request",
		Body:       io.NopCloser(&failingReader{}),
	}
	err := CheckResponse("test", resp)
	if err == nil {
		t.Fatal("expected error for 400 with body read failure")
	}
	if !strings.Contains(err.Error(), "read response body") {
		t.Errorf("expected 'read response body' in error, got: %v", err)
	}
	if errors.Is(err, ErrNotAvailable) {
		t.Error("400 with body read error should NOT wrap ErrNotAvailable")
	}
}

// failingReader always returns an error on Read.
type failingReader struct{}

func (f *failingReader) Read(_ []byte) (int, error) {
	return 0, fmt.Errorf("simulated read error")
}

// --- CustomProvider HTTP Tests ---

func TestCustomProvider_Available(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		model    string
		want     bool
	}{
		{"both set", "http://localhost", "model-1", true},
		{"empty endpoint", "", "model-1", false},
		{"empty model", "http://localhost", "", false},
		{"both empty", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewCustom("test", tt.endpoint, "key", tt.model)
			if p.Available() != tt.want {
				t.Errorf("Available() = %v, want %v", p.Available(), tt.want)
			}
		})
	}
}

func newTestServer(t *testing.T, statusCode int, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		fmt.Fprint(w, body)
	}))
}

func TestCustomProvider_Success(t *testing.T) {
	respBody, _ := json.Marshal(chatResponse{
		Choices: []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		}{
			{Message: struct {
				Content string `json:"content"`
			}{Content: "hello from LLM"}},
		},
	})
	srv := newTestServer(t, 200, string(respBody))
	defer srv.Close()

	p := NewCustom("test-llm", srv.URL, "key", "model-1")
	resp, err := p.Chat(context.Background(), []types.Message{
		{Role: types.RoleUser, Content: "hi"},
	}, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp != "hello from LLM" {
		t.Errorf("response = %q, want %q", resp, "hello from LLM")
	}
}

func TestCustomProvider_4xx(t *testing.T) {
	srv := newTestServer(t, 401, `{"error":{"message":"invalid api key"}}`)
	defer srv.Close()

	p := NewCustom("test-llm", srv.URL, "bad-key", "model-1")
	_, err := p.Chat(context.Background(), []types.Message{
		{Role: types.RoleUser, Content: "hi"},
	}, nil)
	if err == nil {
		t.Fatal("expected error for 401")
	}

	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected *APIError, got %T: %v", err, err)
	}
	if apiErr.StatusCode != 401 {
		t.Errorf("StatusCode = %d, want 401", apiErr.StatusCode)
	}
	// 401 is now fallback-eligible.
	if !errors.Is(err, ErrNotAvailable) {
		t.Error("401 should trigger fallback via ErrNotAvailable")
	}
}

func TestCustomProvider_429_Triggers_Fallback(t *testing.T) {
	srv := newTestServer(t, 429, `{"error":"rate limited"}`)
	defer srv.Close()

	p := NewCustom("test-llm", srv.URL, "key", "model-1")
	_, err := p.Chat(context.Background(), []types.Message{
		{Role: types.RoleUser, Content: "hi"},
	}, nil)
	if err == nil {
		t.Fatal("expected error for 429")
	}
	// 429 IS retryable — wraps ErrNotAvailable.
	if !errors.Is(err, ErrNotAvailable) {
		t.Error("429 should wrap ErrNotAvailable for fallback chain")
	}
}

func TestCustomProvider_5xx_Triggers_Fallback(t *testing.T) {
	srv := newTestServer(t, 500, "internal error")
	defer srv.Close()

	p := NewCustom("test-llm", srv.URL, "key", "model-1")
	_, err := p.Chat(context.Background(), []types.Message{
		{Role: types.RoleUser, Content: "hi"},
	}, nil)
	if err == nil {
		t.Fatal("expected error for 500")
	}
	if !errors.Is(err, ErrNotAvailable) {
		t.Error("500 should wrap ErrNotAvailable for fallback chain")
	}
}

func TestCustomProvider_FallbackChain_Integration(t *testing.T) {
	// First provider returns 429 → chain should fall back to second.
	srv429 := newTestServer(t, 429, `{"error":"rate limited"}`)
	defer srv429.Close()

	goodResp, _ := json.Marshal(chatResponse{
		Choices: []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		}{
			{Message: struct {
				Content string `json:"content"`
			}{Content: "fallback response"}},
		},
	})
	srvOK := newTestServer(t, 200, string(goodResp))
	defer srvOK.Close()

	chain := NewFallbackChain([]LLMProvider{
		NewCustom("primary", srv429.URL, "key", "m1"),
		NewCustom("backup", srvOK.URL, "key", "m2"),
	})

	resp, err := chain.Chat(context.Background(), []types.Message{
		{Role: types.RoleUser, Content: "hi"},
	}, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp != "fallback response" {
		t.Errorf("response = %q, want %q", resp, "fallback response")
	}
}

func TestCustomProvider_401_Fallback(t *testing.T) {
	// 401 should now trigger fallback to the next provider.
	srv401 := newTestServer(t, 401, `{"error":"bad key"}`)
	defer srv401.Close()

	chain := NewFallbackChain([]LLMProvider{
		NewCustom("primary", srv401.URL, "bad", "m1"),
		&mockProvider{name: "backup", available: true, response: "from backup"},
	})

	resp, err := chain.Chat(context.Background(), []types.Message{
		{Role: types.RoleUser, Content: "hi"},
	}, nil)
	if err != nil {
		t.Fatalf("expected fallback to succeed, got error: %v", err)
	}
	if resp != "from backup" {
		t.Errorf("response = %q, want %q", resp, "from backup")
	}
}

// --- OpenAI HTTP Tests ---

func TestOpenAI_Chat_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify headers.
		if got := r.Header.Get("Authorization"); got != "Bearer sk-openai-test" {
			t.Errorf("Authorization = %q, want %q", got, "Bearer sk-openai-test")
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want %q", got, "application/json")
		}

		// Verify request body.
		var reqBody openaiRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if reqBody.Model != "gpt-4o" {
			t.Errorf("model = %q, want %q", reqBody.Model, "gpt-4o")
		}
		// System message should be passed as role "system".
		if len(reqBody.Messages) < 2 {
			t.Fatalf("messages count = %d, want >= 2", len(reqBody.Messages))
		}
		if reqBody.Messages[0].Role != "system" {
			t.Errorf("messages[0].role = %q, want %q", reqBody.Messages[0].Role, "system")
		}
		if reqBody.Messages[1].Role != "user" {
			t.Errorf("messages[1].role = %q, want %q", reqBody.Messages[1].Role, "user")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(openaiResponse{
			Choices: []openaiChoice{
				{Message: openaiMessage{Role: "assistant", Content: "Hello from OpenAI!"}},
			},
		})
	}))
	defer srv.Close()

	p := NewOpenAI("sk-openai-test", "gpt-4o")
	p.endpoint = srv.URL

	resp, err := p.Chat(context.Background(), []types.Message{
		{Role: types.RoleSystem, Content: "You are helpful."},
		{Role: types.RoleUser, Content: "Hi"},
	}, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp != "Hello from OpenAI!" {
		t.Errorf("response = %q, want %q", resp, "Hello from OpenAI!")
	}
}

func TestOpenAI_Chat_ErrorHandling(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantRetry  bool
	}{
		{"429 rate limit", 429, true},
		{"500 server error", 500, true},
		{"401 unauthorized", 401, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newTestServer(t, tt.statusCode, `{"error":"test error"}`)
			defer srv.Close()

			p := NewOpenAI("sk-test", "gpt-4o")
			p.endpoint = srv.URL

			_, err := p.Chat(context.Background(), []types.Message{
				{Role: types.RoleUser, Content: "Hi"},
			}, nil)
			if err == nil {
				t.Fatalf("expected error for %d", tt.statusCode)
			}
			if tt.wantRetry && !errors.Is(err, ErrNotAvailable) {
				t.Errorf("%d should wrap ErrNotAvailable for fallback", tt.statusCode)
			}
			if !tt.wantRetry && errors.Is(err, ErrNotAvailable) {
				t.Errorf("%d should NOT wrap ErrNotAvailable", tt.statusCode)
			}
		})
	}
}

func TestOpenAI_BuildRequest(t *testing.T) {
	p := NewOpenAI("sk-test", "gpt-4o")
	req := p.buildRequest([]types.Message{
		{Role: types.RoleSystem, Content: "system prompt"},
		{Role: types.RoleUser, Content: "hello"},
		{Role: types.RoleAssistant, Content: "hi there"},
		{Role: types.RoleUser, Content: "follow-up"},
	})

	if req.Model != "gpt-4o" {
		t.Errorf("model = %q, want %q", req.Model, "gpt-4o")
	}
	if len(req.Messages) != 4 {
		t.Fatalf("messages count = %d, want 4", len(req.Messages))
	}
	// OpenAI passes system as role "system" directly.
	if req.Messages[0].Role != "system" || req.Messages[0].Content != "system prompt" {
		t.Errorf("messages[0] = %+v, want system role", req.Messages[0])
	}
	if req.Messages[1].Role != "user" {
		t.Errorf("messages[1].role = %q, want %q", req.Messages[1].Role, "user")
	}
	if req.Messages[2].Role != "assistant" {
		t.Errorf("messages[2].role = %q, want %q", req.Messages[2].Role, "assistant")
	}
}

// --- Gemini Tests ---

func TestGemini_Name(t *testing.T) {
	p := NewGemini("key", "gemini-2.5-pro")
	if p.Name() != "gemini/gemini-2.5-pro" {
		t.Errorf("Name() = %q, want %q", p.Name(), "gemini/gemini-2.5-pro")
	}
}

func TestGemini_Available(t *testing.T) {
	tests := []struct {
		name   string
		apiKey string
		model  string
		want   bool
	}{
		{"with key and model", "key", "gemini-2.5-pro", true},
		{"empty key", "", "gemini-2.5-pro", false},
		{"empty model", "key", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewGemini(tt.apiKey, tt.model)
			if p.Available() != tt.want {
				t.Errorf("Available() = %v, want %v", p.Available(), tt.want)
			}
		})
	}
}

func TestGemini_Chat_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify URL contains model and key.
		if !strings.Contains(r.URL.Path, "gemini-2.5-pro") {
			t.Errorf("URL path = %q, want it to contain model name", r.URL.Path)
		}
		if r.URL.Query().Get("key") != "test-key" {
			t.Errorf("key param = %q, want %q", r.URL.Query().Get("key"), "test-key")
		}

		// Verify request body.
		var reqBody geminiRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		// System message should be in systemInstruction.
		if reqBody.SystemInstruction == nil {
			t.Fatal("expected systemInstruction to be set")
		}
		if reqBody.SystemInstruction.Parts[0].Text != "You are helpful." {
			t.Errorf("systemInstruction = %q, want %q", reqBody.SystemInstruction.Parts[0].Text, "You are helpful.")
		}
		// User message in contents.
		if len(reqBody.Contents) != 1 || reqBody.Contents[0].Role != "user" {
			t.Errorf("contents = %+v, want 1 user message", reqBody.Contents)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(geminiResponse{
			Candidates: []geminiCandidate{
				{Content: geminiContent{
					Role:  "model",
					Parts: []geminiPart{{Text: "Hello from Gemini!"}},
				}},
			},
		})
	}))
	defer srv.Close()

	p := NewGemini("test-key", "gemini-2.5-pro")
	p.endpoint = srv.URL

	resp, err := p.Chat(context.Background(), []types.Message{
		{Role: types.RoleSystem, Content: "You are helpful."},
		{Role: types.RoleUser, Content: "Hi"},
	}, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp != "Hello from Gemini!" {
		t.Errorf("response = %q, want %q", resp, "Hello from Gemini!")
	}
}

func TestGemini_Chat_ErrorHandling(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantRetry  bool
	}{
		{"429 rate limit", 429, true},
		{"500 server error", 500, true},
		{"400 bad request", 400, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newTestServer(t, tt.statusCode, `{"error":"test"}`)
			defer srv.Close()

			p := NewGemini("key", "gemini-2.5-pro")
			p.endpoint = srv.URL

			_, err := p.Chat(context.Background(), []types.Message{
				{Role: types.RoleUser, Content: "Hi"},
			}, nil)
			if err == nil {
				t.Fatalf("expected error for %d", tt.statusCode)
			}
			if tt.wantRetry && !errors.Is(err, ErrNotAvailable) {
				t.Errorf("%d should wrap ErrNotAvailable", tt.statusCode)
			}
			if !tt.wantRetry && errors.Is(err, ErrNotAvailable) {
				t.Errorf("%d should NOT wrap ErrNotAvailable", tt.statusCode)
			}
		})
	}
}

func TestGemini_RoleMapping(t *testing.T) {
	p := NewGemini("key", "gemini-2.5-pro")
	req := p.buildRequest([]types.Message{
		{Role: types.RoleSystem, Content: "system prompt"},
		{Role: types.RoleUser, Content: "hello"},
		{Role: types.RoleAssistant, Content: "hi there"},
		{Role: types.RoleUser, Content: "follow-up"},
	})

	// System → systemInstruction.
	if req.SystemInstruction == nil {
		t.Fatal("expected systemInstruction to be set")
	}
	if req.SystemInstruction.Parts[0].Text != "system prompt" {
		t.Errorf("systemInstruction = %q, want %q", req.SystemInstruction.Parts[0].Text, "system prompt")
	}

	// User → "user", assistant → "model".
	if len(req.Contents) != 3 {
		t.Fatalf("contents count = %d, want 3 (system excluded)", len(req.Contents))
	}
	if req.Contents[0].Role != "user" {
		t.Errorf("contents[0].role = %q, want %q", req.Contents[0].Role, "user")
	}
	if req.Contents[1].Role != "model" {
		t.Errorf("contents[1].role = %q, want %q", req.Contents[1].Role, "model")
	}
	if req.Contents[2].Role != "user" {
		t.Errorf("contents[2].role = %q, want %q", req.Contents[2].Role, "user")
	}
}

func TestGemini_NoSystemInstruction(t *testing.T) {
	p := NewGemini("key", "gemini-2.5-pro")
	req := p.buildRequest([]types.Message{
		{Role: types.RoleUser, Content: "hello"},
	})
	if req.SystemInstruction != nil {
		t.Error("systemInstruction should be nil when no system messages")
	}
	if len(req.Contents) != 1 {
		t.Fatalf("contents count = %d, want 1", len(req.Contents))
	}
}

// --- Three-Provider Fallback Test ---

func TestFallbackChain_ThreeProviders(t *testing.T) {
	chain := NewFallbackChain([]LLMProvider{
		&mockProvider{name: "anthropic", available: false},
		&mockProvider{name: "openai", available: false},
		&mockProvider{name: "gemini", available: true, response: "from-gemini"},
	})

	resp, err := chain.Chat(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp != "from-gemini" {
		t.Errorf("response = %q, want %q", resp, "from-gemini")
	}
}
