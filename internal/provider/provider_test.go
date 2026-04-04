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

func (m *mockProvider) Chat(_ context.Context, _ []types.Message) (string, error) {
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

	resp, err := chain.Chat(context.Background(), nil)
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

	resp, err := chain.Chat(context.Background(), nil)
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

	_, err := chain.Chat(context.Background(), nil)
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

	resp, err := chain.Chat(context.Background(), nil)
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

	_, err := chain.Chat(context.Background(), nil)
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

	_, err := chain.Chat(context.Background(), nil)
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

	resp, err := chain.Chat(context.Background(), nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp != "solo-response" {
		t.Errorf("response = %q, want %q", resp, "solo-response")
	}
}

func TestFallbackChain_Empty(t *testing.T) {
	chain := NewFallbackChain(nil)

	_, err := chain.Chat(context.Background(), nil)
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
	_, err := p.Chat(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for unavailable provider")
	}
	if !errors.Is(err, ErrNotAvailable) {
		t.Errorf("expected ErrNotAvailable, got: %v", err)
	}
}

func TestAnthropic_Chat_Unavailable(t *testing.T) {
	p := NewAnthropic("", "claude-sonnet-4-6")
	_, err := p.Chat(context.Background(), nil)
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
	})
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
	})
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
	})
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
	})
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
	})
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
	if errors.Is(err, ErrNotAvailable) {
		t.Error("401 should not trigger fallback")
	}
}

func TestAnthropic_Chat_429_Fallback(t *testing.T) {
	srv := newTestServer(t, 429, `{"type":"error","error":{"type":"rate_limit_error","message":"rate limited"}}`)
	defer srv.Close()

	p := NewAnthropic("sk-ant-test", "claude-sonnet-4-6")
	p.endpoint = srv.URL

	_, err := p.Chat(context.Background(), []types.Message{
		{Role: types.RoleUser, Content: "Hi"},
	})
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
	})
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
		{401, false},
		{403, false},
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
	if apiErr.Retryable() {
		t.Error("401 should not be retryable")
	}
	// Non-retryable errors should NOT wrap ErrNotAvailable.
	if errors.Is(err, ErrNotAvailable) {
		t.Error("401 should not wrap ErrNotAvailable")
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
	resp := &http.Response{
		StatusCode: 401,
		Status:     "401 Unauthorized",
		Body:       io.NopCloser(&failingReader{}),
	}
	err := CheckResponse("test", resp)
	if err == nil {
		t.Fatal("expected error for 401 with body read failure")
	}
	if !strings.Contains(err.Error(), "read response body") {
		t.Errorf("expected 'read response body' in error, got: %v", err)
	}
	if errors.Is(err, ErrNotAvailable) {
		t.Error("401 with body read error should NOT wrap ErrNotAvailable")
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
	})
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
	})
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
	// 401 is NOT retryable — should NOT trigger fallback.
	if errors.Is(err, ErrNotAvailable) {
		t.Error("401 should not trigger fallback")
	}
}

func TestCustomProvider_429_Triggers_Fallback(t *testing.T) {
	srv := newTestServer(t, 429, `{"error":"rate limited"}`)
	defer srv.Close()

	p := NewCustom("test-llm", srv.URL, "key", "model-1")
	_, err := p.Chat(context.Background(), []types.Message{
		{Role: types.RoleUser, Content: "hi"},
	})
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
	})
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
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp != "fallback response" {
		t.Errorf("response = %q, want %q", resp, "fallback response")
	}
}

func TestCustomProvider_4xx_No_Fallback(t *testing.T) {
	// 401 should NOT trigger fallback — it's a client error.
	srv401 := newTestServer(t, 401, `{"error":"bad key"}`)
	defer srv401.Close()

	chain := NewFallbackChain([]LLMProvider{
		NewCustom("primary", srv401.URL, "bad", "m1"),
		&mockProvider{name: "backup", available: true, response: "should not reach"},
	})

	_, err := chain.Chat(context.Background(), []types.Message{
		{Role: types.RoleUser, Content: "hi"},
	})
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
}
