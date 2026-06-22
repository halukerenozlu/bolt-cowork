package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProviderState_String(t *testing.T) {
	tests := []struct {
		state ProviderState
		want  string
	}{
		{StateNotConfigured, "not configured"},
		{StateConfigured, "configured"},
		{StateConnected, "connected"},
		{StateError, "error"},
		{ProviderState(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("ProviderState(%d).String() = %q, want %q", tt.state, got, tt.want)
		}
	}
}

func TestOpenAI_Verify(t *testing.T) {
	tests := []struct {
		name       string
		apiKey     string
		status     int
		wantErr    bool
		skipServer bool
	}{
		{name: "success", apiKey: "test-key", status: http.StatusOK},
		{name: "unauthorized", apiKey: "bad-key", status: http.StatusUnauthorized, wantErr: true},
		{name: "no key", apiKey: "", wantErr: true, skipServer: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := NewOpenAI(tt.apiKey, "gpt-4o")
			if !tt.skipServer {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path != "/v1/models" {
						t.Errorf("unexpected path: %s", r.URL.Path)
					}
					w.WriteHeader(tt.status)
					_, _ = w.Write([]byte(`{"data":[]}`))
				}))
				defer srv.Close()
				p.endpoint = srv.URL + "/v1/chat/completions"
				p.client = srv.Client()
			}
			err := p.Verify(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("Verify() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAnthropic_Verify(t *testing.T) {
	tests := []struct {
		name    string
		apiKey  string
		status  int
		wantErr bool
	}{
		{name: "success", apiKey: "test-key", status: http.StatusOK},
		{name: "unauthorized", apiKey: "bad-key", status: http.StatusUnauthorized, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.apiKey == "test-key" && r.Header.Get("x-api-key") != "test-key" {
					t.Errorf("missing x-api-key header")
				}
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"hi"}]}`))
			}))
			defer srv.Close()

			p := NewAnthropic(tt.apiKey, "claude-sonnet-4-6")
			p.endpoint = srv.URL
			p.client = srv.Client()

			err := p.Verify(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("Verify() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGemini_Verify(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("x-goog-api-key") != "test-key" {
			t.Errorf("missing x-goog-api-key header")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"models":[]}`))
	}))
	defer srv.Close()

	p := NewGemini("test-key", "gemini-2.5-flash")
	p.endpoint = srv.URL
	p.client = srv.Client()

	if err := p.Verify(context.Background()); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

func TestCustom_Verify(t *testing.T) {
	tests := []struct {
		name       string
		endpoint   string
		apiKey     string
		status     int
		wantErr    bool
		skipServer bool
	}{
		{name: "success", apiKey: "test-key", status: http.StatusOK},
		{name: "no endpoint", endpoint: "", apiKey: "key", wantErr: true, skipServer: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.skipServer {
				p := NewCustom("test", tt.endpoint, tt.apiKey, "model")
				err := p.Verify(context.Background())
				if (err != nil) != tt.wantErr {
					t.Errorf("Verify() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v1/models" {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(`{"data":[]}`))
			}))
			defer srv.Close()
			p := NewCustom("openrouter", srv.URL+"/v1/chat/completions", tt.apiKey, "openai/gpt-4.1")
			p.client = srv.Client()
			err := p.Verify(context.Background())
			if (err != nil) != tt.wantErr {
				t.Errorf("Verify() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestVerifyProvider(t *testing.T) {
	tests := []struct {
		name    string
		setup   func() LLMProvider
		wantErr bool
	}{
		{
			name: "non-verifier returns nil",
			setup: func() LLMProvider {
				return &mockProvider{name: "mock", available: true}
			},
		},
		{
			name: "verifier success",
			setup: func() LLMProvider {
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{"data":[]}`))
				}))
				t.Cleanup(srv.Close)
				p := NewOpenAI("test-key", "gpt-4o")
				p.endpoint = srv.URL + "/v1/chat/completions"
				p.client = srv.Client()
				return p
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := tt.setup()
			err := VerifyProvider(context.Background(), p)
			if (err != nil) != tt.wantErr {
				t.Errorf("VerifyProvider() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
